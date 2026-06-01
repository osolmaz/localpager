# Localpager

Localpager is a local-first notifier for GitHub issues and pull requests.

It watches repo activity, queues new items, runs a classifier command, stores the
structured result, and sends matching notifications to Discord.

This repository is an early extraction of a working notifier prototype. The
current code is still intentionally small: it provides the worker, queue,
gitcrawl source adapter, classifier command boundary, SQLite state, and Discord
delivery. The next step is a config-driven `localpager` CLI and direct GitHub API
source support.

## Current Commands

```text
cmd/notifier-enqueue-github  enqueue GitHub issues and PRs from a gitcrawl DB
cmd/notifier-ingest-json     ingest one normalized item from JSON
cmd/notifier-watch           poll source adapters and enqueue items
cmd/notifier-worker          run classifier jobs and send notifications
```

## Classifier Contract

The worker runs an external classifier command. The command receives one target
argument, usually a GitHub URL or `owner/repo#number`, and must print one JSON
object to stdout:

```json
{
  "interest": "high",
  "confidence": 0.92,
  "topics_of_interest": ["bug", "release"],
  "description": "Why this item matters.",
  "caveats": []
}
```

The worker stores the full JSON. It treats empty, `none`, `low`, `irrelevant`,
`false`, or `i0` interest values as non-notifying. Other interest values create a
notification.

If the classifier writes lines like these to stderr, Localpager stores them with
the result:

```text
prompt: /path/to/prompt.md
session: /path/to/session.jsonl
```

## Local Run

```bash
go test ./...
go run ./cmd/notifier-worker --once --dry-run-discord
```

To send Discord messages, pass a channel ID and set a bot token:

```bash
export DISCORD_BOT_TOKEN="<discord bot token>"
go run ./cmd/notifier-worker \
  --send-discord \
  --discord-channel-id "$DISCORD_CHANNEL_ID" \
  --classifier-command ./scripts/your-classifier
```

Do not commit tokens, `.env` files, SQLite databases, classifier sessions, or
prompt logs.

## gitcrawl Source

The first source adapter reads from a local gitcrawl SQLite database:

```bash
go run ./cmd/notifier-enqueue-github \
  --repo owner/repo \
  --type both \
  --gitcrawl-db ~/.config/gitcrawl/gitcrawl.db
```

Direct GitHub API polling and webhook support should be added before treating
Localpager as broadly usable outside machines that already run gitcrawl.

## Runtime State

The default SQLite state path is:

```text
~/.local/state/localpager/notifier.sqlite
```

Override it with `--db`.
