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
localpager reposhell       compatibility wrapper for the standalone reposhell CLI
localpager-enqueue-github  enqueue GitHub issues and PRs once
localpager-ingest-json     ingest one normalized item from JSON
localpager-watch           poll source adapters and enqueue items
localpager-worker          run classifier jobs and send notifications
reposhell                  standalone read-only repository shell service, exec, and shell
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
  "classifier": {
    "schema": "~/.config/localpager/classification.schema.json",
    "prompt_template": "~/.config/localpager/classifier.prompt.md",
    "topic_taxonomy": "~/.config/localpager/topics.json",
    "tools": ["bash", "final_json"],
    "reposhell_default_repo": "localpager",
    "reposhell_visible_repos": ["localpager"],
    "context": {
      "github": {
        "include_body": true,
        "include_labels": true,
        "include_comments": true,
        "include_changed_files": true,
        "include_diff": true
      }
    }
  },
  "worker": {
    "agent_base_url": "http://127.0.0.1:1234/v1",
    "agent_context_window": 8192,
    "agent_max_tokens": 768,
    "agent_timeout_ms": 5000,
    "model_unavailable_retry_delay": "5m",
    "notify_topics_any": ["local_models", "open_weight_models"]
  }
}
```

When `classifier.tools` includes `bash`, run `reposhell serve` with matching
`reposhell` config. The model sees a familiar bash-shaped tool, but Localpager
enforces a read-only command allowlist against pinned repository snapshots. See
[Reposhell](reposhell/README.md) for standalone setup, config, direct
commands, allowed command shapes, service mode, and troubleshooting.

If `notify_topics_any` is set, at least one classifier topic must match before a
notification is created.

Classifier prompts, topic taxonomies, and schemas should be configured as a
deployment profile. See [Classifier Profiles](docs/2026-06-01-classifier-profiles.md).
When `classifier.topic_taxonomy` is set, `localpager-classifier` generates a
runtime schema that rejects topics outside that taxonomy.

For OpenClaw maintainer routing, use
`examples/profiles/openclaw-routing-v8.prompt.md` with the OpenClaw topic
keyword taxonomy in `examples/profiles/openclaw-routing-topics.json`. That
profile encodes the DS4/Gemma prompt work: notification routing, title-first
centrality, one-topic default, a second-topic gate, and false-positive
suppression for broad topics such as `local_model_providers`, `reliability`,
`api_surface`, `tool_calling`, and `config`.

Before the classifier runs, Localpager renders GitHub context into the prompt:
stored title/body/labels plus optional comments, changed files, and selected PR
diff. Prompt templates can include that block with `__GITHUB_CONTEXT__`.
For the local DS4 dataset and Gemma 4 prompt-optimization history that informed
this design, see [DS4 Dataset and Gemma Prompt Optimization](docs/2026-06-01-ds4-dataset-gemma-prompt-optimization.md).
To test a prompt profile on a small live GitHub sample, use
`scripts/localpager-experiment.mjs`; see
[Classifier Experiment Runner](docs/2026-06-01-classifier-experiment-runner.md)
and [Classifier Benchmark Metrics](docs/2026-06-01-classifier-benchmark-metrics.md).
Machine-specific runtime values, such as the loaded model context window,
parallelism, context truncation budget, and DS4/LM Studio exclusivity rules,
belong in a deployment setup document. See
[Onur's Isengard Setup](docs/2026-06-02-onur-isengard-localpager-setup.md) for
the current OpenClaw/Gemma deployment.

If the classifier writes lines like these to stderr, Localpager stores them with
the result:

```text
prompt: /path/to/prompt.md
session: /path/to/session-dir
```

Transient local model failures, such as the LM Studio endpoint being down, are
requeued with `model_unavailable_retry_delay` and do not burn classifier
attempts. To manually requeue matching dead jobs:

```bash
localpager requeue-jobs \
  --config ~/.config/localpager/config.json \
  --status dead \
  --last-error-contains "fetch failed"
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
LOCALPAGER_AGENT_CONTEXT_WINDOW
LOCALPAGER_AGENT_MAX_TOKENS
LOCALPAGER_AGENT_TIMEOUT_MS
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
writes `localpager-worker.service`, `localpager-reposhell.service`,
`localpager-watch.service`, `localpager-enqueue-github.service`, and
`localpager-enqueue-github.timer`.

```bash
make install
localpager install-service --config ~/.config/localpager/config.json --work-dir "$PWD"
systemctl --user daemon-reload
systemctl --user enable --now localpager-reposhell.service localpager-worker.service localpager-enqueue-github.timer
```

That setup runs the worker continuously and enqueues GitHub issues and pull
requests on the timer. Enable `localpager-watch.service` instead if you want
continuous source polling. If your classifier profile does not expose `bash`,
you can leave `localpager-reposhell.service` disabled.

Check state:

```bash
localpager status --config ~/.config/localpager/config.json
localpager test-discord --config ~/.config/localpager/config.json
reposhell status --config ~/.config/localpager/config.json
```

## Runtime State

The default SQLite state path is:

```text
~/.local/state/localpager/localpager.sqlite
```

Localpager creates and updates SQLite tables from its GORM models at startup.

Override it with `--db`.
