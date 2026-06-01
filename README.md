# Localpager

Localpager is a local-first triage and paging tool for GitHub issues and pull requests.

It watches repo activity, queues new items, runs a classifier command, stores the
structured result, and sends matching notifications to Discord.

This repository is an early extraction of a working triage prototype. The
current code is intentionally small: it provides the worker, queue, GitHub and
gitcrawl source adapters, classifier command boundary, SQLite state, Discord
delivery, a bundled local model runner in `localpager-agent/`, and a small
`localpager` operations CLI.

## Current Commands

```text
localpager                 validate config, show status, install services, test Discord
localpager-enqueue-github  enqueue GitHub issues and PRs once
localpager-ingest-json     ingest one normalized item from JSON
localpager-watch           poll source adapters and enqueue items
localpager-worker          run classifier jobs and send notifications
```

## Classifier Contract

The worker runs a classifier command. By default it uses:

```text
./scripts/localpager-classifier
```

That wrapper calls `localpager-agent`, which points Pi at a local
OpenAI-compatible model endpoint and forces the final answer through
`schemas/classification.schema.json`.

The classifier command receives one target argument, usually a GitHub URL or
`owner/repo#number`, and must print one JSON object to stdout:

```json
{
  "topics_of_interest": ["bug", "release"],
  "description": "Why this item matters.",
  "caveats": []
}
```

Notification policy is deployment config, not classifier logic:

```json
{
  "worker": {
    "notify_topics_any": ["local_models", "open_weight_models"]
  }
}
```

If `notify_topics_any` is set, at least one classifier topic must match before a
notification is created.

If the classifier writes lines like these to stderr, Localpager stores them with
the result:

```text
prompt: /path/to/prompt.md
session: /path/to/session.jsonl
```

## Local Run

```bash
npm install --prefix localpager-agent
make test
make install
localpager validate --config examples/config.example.json
localpager-worker --config examples/config.example.json --once --dry-run-discord
```

Check the bundled agent:

```bash
npm --prefix localpager-agent run localpager-agent -- --status
```

To send Discord messages, pass a channel ID and set a bot token:

```bash
export DISCORD_BOT_TOKEN="<discord bot token>"
localpager-worker \
  --config examples/config.example.json \
  --discord-channel-id "$DISCORD_CHANNEL_ID" \
  --send-discord
```

Do not commit tokens, `.env` files, SQLite databases, classifier sessions, or
prompt logs.

## Localpager Agent

`localpager-agent/` is the absorbed local model runner formerly used as an
external command. It defaults to LM Studio's OpenAI-compatible endpoint:

```text
http://127.0.0.1:1234/v1
```

The main environment variables are:

```text
LOCALPAGER_AGENT_BASE_URL
LOCALPAGER_AGENT_MODEL
LOCALPAGER_AGENT_STATE_DIR
LOCALPAGER_AGENT_SESSION_DIR
LOCALPAGER_AGENT_PI_CMD
LOCALPAGER_AGENT_FINAL_SCHEMA
```

## Sources

The default public source is the GitHub API:

```bash
export GITHUB_TOKEN="<github token>"
localpager-watch --config examples/config.example.json --source github --once
```

The gitcrawl source remains available for machines that already maintain a
gitcrawl SQLite database:

```bash
localpager-watch --config examples/config.example.json --source gitcrawl --once
```

`localpager-watch` can run continuously under systemd. `localpager-enqueue-github`
is the one-shot equivalent.

## Services

Install compiled binaries and write user systemd units. The service installer
writes `localpager-worker.service`, `localpager-watch.service`,
`localpager-enqueue-github.service`, and `localpager-enqueue-github.timer`.

```bash
make install
localpager install-service --config ~/.config/localpager/config.json --work-dir "$PWD"
systemctl --user daemon-reload
systemctl --user enable --now localpager-worker.service localpager-enqueue-github.timer
```

That setup runs the worker continuously and enqueues GitHub issues and pull
requests on the timer. Enable `localpager-watch.service` instead if you want
continuous source polling.

Check state:

```bash
localpager status --config ~/.config/localpager/config.json
localpager test-discord --config ~/.config/localpager/config.json
```

## Runtime State

The default SQLite state path is:

```text
~/.local/state/localpager/localpager.sqlite
```

Override it with `--db`.
