---
title: Read-Only Repo Shell Plan
author: Bob <dutifulbob@gmail.com>
date: 2026-06-03
---

# Read-Only Repo Shell Plan

Localpager's background classifier should not get an unrestricted shell.
It should still give the model a familiar `bash` tool because models are good at
using `rg`, `find`, `sed`, and `cat` to inspect repositories.

The goal is a read-only repository interface that looks like bash to the model
but is enforced by Localpager.

## Desired Shape

```text
localpager reposhell service
  - owns repo mirrors
  - keeps them fresh
  - creates pinned read-only snapshots
  - enforces command policy

localpager-agent Pi extension
  - registers a tool named bash
  - sends the command string to reposhell
  - returns stdout/stderr-shaped output

classifier run
  - sees bash and final_json
  - starts in the default repo cwd
  - can only read configured repo snapshots
```

The production classifier should use an explicit tool allowlist:

```text
bash,final_json
```

If reposhell is disabled, the classifier should use:

```text
final_json
```

## Configuration

The reposhell should support multiple repositories, but each classifier profile
must choose which repos are visible.

```json
{
  "reposhell": {
    "enabled": true,
    "root": "~/.local/state/localpager/reposhell",
    "socket": "~/.local/state/localpager/reposhell.sock",
    "repos": [
      {
        "id": "openclaw",
        "remote": "https://github.com/openclaw/openclaw.git",
        "default_ref": "origin/main",
        "refresh_interval": "24h"
      },
      {
        "id": "clawhub",
        "remote": "https://github.com/openclaw/clawhub.git",
        "default_ref": "origin/main",
        "refresh_interval": "24h"
      }
    ],
    "command_timeout": "2s",
    "max_output_bytes": 65536
  },
  "classifier": {
    "reposhell_default_repo": "openclaw",
    "reposhell_visible_repos": ["openclaw"],
    "tools": ["bash", "final_json"]
  }
}
```

The service may sync many repos. A classifier run should only see the configured
visible repos for that profile.
The default refresh interval should be one day unless a repository overrides it.

## Run Binding

Each classifier run should request a pinned view:

```json
{
  "default_repo": "openclaw",
  "visible_repos": ["openclaw", "clawhub"]
}
```

The reposhell returns a run binding:

```json
{
  "run_id": "classifier-20260603-001",
  "cwd": "/repo/openclaw",
  "snapshots": {
    "openclaw": "abc123",
    "clawhub": "def456"
  }
}
```

Relative paths resolve against the default repo cwd. Cross-repo reads require an
explicit virtual path such as `/repo/clawhub`.

## Virtual Filesystem

The model should see a simple virtual layout:

```text
/repo/openclaw
/repo/clawhub
```

There is no real `/repo` mount. The reposhell maps those virtual roots to
pinned snapshot directories under its state root.

The default cwd should be the repo being classified:

```bash
pwd
# /repo/openclaw

rg "tool_calling" src docs
sed -n '1,120p' src/agents/tools/example.ts
```

## Allowed Bash Surface

The model-facing tool should be named `bash` and accept a command string:

```json
{
  "command": "rg \"tool_calling\" src docs"
}
```

The implementation must not execute `/bin/bash -c`. It should parse the command
and run only approved read-only commands with validated arguments.

Initial allowed commands:

```text
pwd
ls
find
rg
grep
sed -n
cat
head
git status --short
git show --name-only
git grep
```

The command surface should be intentionally boring. Add new commands only when a
classifier use case needs them.

## Rejected Shell Features

Reject these before execution:

```text
multiple commands
pipelines
redirects
background jobs
command substitution
env var expansion
env assignments
aliases
functions
globs unless Localpager implements expansion itself
absolute paths outside virtual /repo roots
.. escapes
symlink escapes
network tools
interpreters
package managers
write commands
```

Examples that must fail:

```bash
rg foo . | head
cat package.json > /tmp/out
find . -type f -exec cat {} \;
python -c 'print(1)'
git clean -fdx
rm -rf .
```

## Enforcement

Use Go for the reposhell and command runner.

The command parser should reject complex shell syntax before validation. Prefer
`mvdan.cc/sh/v3/syntax` if the dependency is acceptable, because it can parse
shell syntax without executing it. The runner should accept only a single simple
command node.

Execution rules:

- Use `exec.CommandContext`, never `sh -c`.
- Set `cmd.Dir` to the pinned snapshot for the current virtual cwd.
- Use a minimal environment such as `PATH=/usr/bin:/bin` and `LC_ALL=C`.
- Kill the process group on timeout.
- Cap stdout and stderr separately.
- Return truncation markers when output is capped.
- Do not pass stdin.

Path rules:

- Resolve virtual paths before execution.
- Resolve symlinks with `filepath.EvalSymlinks`.
- Reject any resolved path outside its pinned snapshot root.
- Treat missing paths as normal command errors, not policy errors.

## Service API

Expose a Unix socket API under Localpager state:

```text
~/.local/state/localpager/reposhell.sock
```

Minimum endpoints:

```text
CreateRunBinding(default_repo, visible_repos) -> run_id, cwd, snapshots
Exec(run_id, command) -> stdout, stderr, exit_code, policy_error, truncated
Refresh(repo_id) -> status
Status() -> repo sync state and active snapshot commits
```

The Pi extension should only call `CreateRunBinding` once per classifier run and
then call `Exec` for each `bash` tool invocation.

## Sync Model

The service owns mirrors and snapshots:

```text
reposhell/
  mirrors/<repo-id>.git
  worktrees/<repo-id>/<commit-sha>/
  snapshots/<repo-id>/<commit-sha> -> worktree path
```

Refresh loop:

1. Clone mirror if missing.
2. Fetch `default_ref` with `--prune`.
3. Resolve the commit SHA.
4. Ensure a clean read-only worktree for that SHA.
5. Garbage-collect old snapshots not used by active runs.

Classifier runs must use pinned snapshots. A refresh must not change what an
active run sees.

## Localpager-Agent Integration

Add classifier tool configuration:

```json
{
  "classifier": {
    "tools": ["bash", "final_json"],
    "reposhell_default_repo": "openclaw",
    "reposhell_visible_repos": ["openclaw"]
  }
}
```

`scripts/localpager-classifier` should pass an explicit tool allowlist to
`localpager-agent`.

When reposhell is enabled:

```text
--tools bash,final_json
--extension <reposhell-bash-extension>
```

When reposhell is disabled:

```text
--tools final_json
```

This removes dependence on Pi's default tool behavior.

Before enabling `bash` in production, add an automated check that the exposed
`bash` tool is the Localpager extension and not an unrestricted Pi built-in.

## Observability

Store enough metadata with each classifier result to audit what the model saw:

```json
{
  "reposhell": {
    "default_repo": "openclaw",
    "visible_repos": ["openclaw"],
    "snapshots": {
      "openclaw": "abc123"
    },
    "tool_calls": 4,
    "policy_denials": 0,
    "truncated_outputs": 1
  }
}
```

Do not store full command outputs in the database by default. They can contain
large source snippets. Store them in classifier session artifacts if needed.

## Implementation Checklist

- [x] Add `reposhell` config structs.
- [x] Add `classifier.tools`, `classifier.reposhell_default_repo`, and
  `classifier.reposhell_visible_repos`.
- [x] Add `localpager reposhell serve`.
- [x] Add mirror clone/fetch and snapshot pinning.
- [x] Add the command parser and policy validator.
- [x] Add read-only command execution with timeouts and output caps.
- [x] Add Unix socket API.
- [x] Add a Pi extension that registers tool name `bash`.
- [x] Make `localpager-agent` support explicit `--tools final_json` in schema
  mode.
- [x] Make `scripts/localpager-classifier` pass explicit tools.
- [x] Add tests that unrestricted shell commands are rejected.
- [x] Add tests that path traversal and symlink escapes are rejected.
- [x] Add tests that a classifier run starts in the default repo cwd.
- [x] Add tests for multiple visible repos and explicit `/repo/<id>` paths.
- [x] Add service docs and an example config.
- [x] Reject `--tools bash` unless the Localpager reposhell extension is
  attached.
- [ ] Persist reposhell audit metadata on classifier results.
- [ ] Add snapshot garbage collection.

## Open Questions

- Should `git show` allow file content for a specific commit, or only
  `--name-only` metadata at first?
- Should `rg` allow multiline and PCRE flags, or keep to the default engine?
- Should snapshots be filesystem read-only with permissions, or is policy-level
  read-only plus no write commands enough?
- How many old snapshots should the service retain?
- Should the reposhell support private repo credentials, or only already
  accessible HTTPS remotes for now?
