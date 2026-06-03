# Reposhell

Reposhell is a read-only, bash-shaped interface for repository snapshots.

It is meant for coding agents and classifiers that need to inspect repo files
without receiving an unrestricted shell. The model can ask for familiar
commands such as `rg`, `grep`, `cat`, `sed -n`, `git grep`, and `git ls-files`;
reposhell parses that command, rejects unsafe shell syntax, maps `/repo/<id>`
paths to configured snapshots, and runs only an allowlisted read-only command.

## Install

From this repository:

```bash
make install
```

That installs `reposhell` under `~/.local/bin` by default. To build only the
binary without installing:

```bash
go build -o bin/reposhell ./cmd/reposhell
```

## Configuration

The standalone CLI reads this file by default:

```text
~/.config/reposhell/config.json
```

Create a config like this:

```json
{
  "root": "~/.local/state/reposhell",
  "socket": "~/.local/state/reposhell/reposhell.sock",
  "default_repo": "project",
  "visible_repos": ["project"],
  "refresh_interval": "24h",
  "command_timeout": "2s",
  "max_output_bytes": 65536,
  "snapshot_retain": 5,
  "repos": [
    {
      "id": "project",
      "remote": "https://github.com/example/project.git",
      "default_ref": "origin/main",
      "refresh_interval": "24h"
    }
  ]
}
```

You can also use a local checkout as the remote:

```json
{
  "id": "project",
  "remote": "file:///home/alice/repos/project/.git",
  "default_ref": "main"
}
```

Config fields:

- `root`: where reposhell stores its own mirrors and snapshots.
- `socket`: Unix socket used by `reposhell serve`.
- `default_repo`: repo used for relative paths and `pwd`.
- `visible_repos`: repo ids available to a run.
- `refresh_interval`: default mirror fetch interval. The default is `24h`.
- `command_timeout`: maximum command runtime. The default is `2s`.
- `max_output_bytes`: stdout and stderr cap. The default is `65536`.
- `snapshot_retain`: completed snapshots to retain per repo, in addition to
  snapshots currently bound to active service runs. The default is `5`.
- `repos[].id`: stable repo id used in `/repo/<id>` paths.
- `repos[].remote`: Git remote URL or `file://` URL.
- `repos[].default_ref`: ref to snapshot. The default is `origin/main`.
- `repos[].refresh_interval`: optional per-repo refresh interval.

Reposhell stores snapshots under its own `root`; it does not read directly from
your working checkout after the snapshot is made. Mirrors are fetched when a run
binds to a repo and the configured refresh interval has elapsed. In service
mode, completed snapshots beyond the `snapshot_retain` count are
garbage-collected after binds, but snapshots currently attached to active runs
are kept.

## Direct Use

Run one command:

```bash
reposhell exec \
  --config ~/.config/reposhell/config.json \
  --repo project \
  --visible-repo project \
  --command 'rg -n reposhell README.md'
```

Search recursively:

```bash
reposhell exec \
  --config ~/.config/reposhell/config.json \
  --repo project \
  --visible-repo project \
  --command 'rg -n -i "api client" .'
```

Use recursive `grep` when you want POSIX grep behavior:

```bash
reposhell exec \
  --config ~/.config/reposhell/config.json \
  --repo project \
  --visible-repo project \
  --command 'grep -R -n -i "api client" .'
```

List files:

```bash
reposhell exec \
  --config ~/.config/reposhell/config.json \
  --repo project \
  --visible-repo project \
  --command 'git ls-files src'
```

Open an interactive read-only prompt:

```bash
reposhell shell \
  --config ~/.config/reposhell/config.json \
  --repo project \
  --visible-repo project
```

Inside `reposhell shell`, type one command per line. Use `help` to print the
allowed command shapes, and `exit` or `quit` to leave.

## Service Mode

Start the Unix-socket service:

```bash
reposhell serve --config ~/.config/reposhell/config.json
```

Check the service:

```bash
reposhell status --config ~/.config/reposhell/config.json
```

`reposhell serve` exposes a small local HTTP API over the configured Unix
socket. Agent integrations use that API to bind a run to a pinned set of
snapshots, then execute read-only commands against that binding.

## Paths

Commands start in the configured default repo:

```text
/repo/project
```

Relative paths are resolved inside that repo. You can address another visible
repo with an absolute virtual path:

```bash
reposhell exec \
  --config ~/.config/reposhell/config.json \
  --repo project \
  --visible-repo project \
  --visible-repo docs \
  --command 'rg -n "authentication" /repo/docs'
```

Real absolute paths outside `/repo/...` are rejected.

## Allowed Commands

Reposhell accepts one simple command at a time. Pipes, redirects, command
substitution, environment assignments, background jobs, and shell control
operators are rejected before execution.

Allowed command families:

- `pwd`
- `ls` with `-a`, `-l`, `-la`, `-al`, or `-1`
- `find` with paths plus `-maxdepth`, `-type f|d`, `-name`, or `-iname`
- `rg` with search flags, recursive search, `--files`, and `-g` globs
- `grep` with explicit file or directory paths; use `grep -R` for recursion
- `cat`
- `head` and `tail`, optionally with `-n`
- `wc -l`
- `sed -n START,ENDp file`
- `git status --short`
- `git show --name-only`
- `git grep`
- `git ls-files`

Examples:

```bash
pwd
ls -la
find . -maxdepth 2 -type f -name "*.go"
rg -n -i "reposhell" .
rg --files -g "*.ts"
grep -R -n -i "api client" .
cat README.md
sed -n 1,80p README.md
head -n 40 README.md
tail -n 40 README.md
wc -l README.md docs/architecture.md
git status --short
git show --name-only
git grep -n reposhell
git ls-files src
```

These are intentionally not allowed:

```bash
cd /tmp
rg reposhell | head
cat README.md > /tmp/out
$(cat README.md)
find . -type f -exec cat {} \;
python script.py
```

## Troubleshooting

If `reposhell status` says `connection refused`, the service is not running or
the socket path points at an old socket:

```bash
reposhell serve --config ~/.config/reposhell/config.json
```

If a command is rejected with `policy_error`, simplify it to one allowlisted
read-only command. For example, use this:

```bash
rg -n -i "api client" .
```

instead of this:

```bash
grep "api client"
```

`grep` requires an explicit path, and recursive grep requires `-R`:

```bash
grep -R -n -i "api client" .
```

If the snapshot is stale, reduce `refresh_interval` or remove the mirror under
`root/mirrors/<repo>.git` and bind again. If snapshot storage grows too large,
lower `snapshot_retain`; active service runs keep their pinned snapshots even
when they are older than the retention window.
