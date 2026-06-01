---
title: Classifier Experiment Runner
author: Bob <dutifulbob@gmail.com>
date: 2026-06-01
---

# Classifier Experiment Runner

Localpager includes an experiment runner for checking a prompt profile against
live GitHub issues and pull requests.

The runner takes a repo, a prompt template, a topic taxonomy, and a schema. It
fetches a small GitHub sample, renders the same kind of context Localpager sends
to the classifier, runs a reference model and a target model through
`scripts/localpager-classifier`, validates the outputs, then writes a benchmark
bundle.

## Command

```bash
node scripts/localpager-experiment.mjs \
  --repo owner/repo \
  --limit 5 \
  --item-type both \
  --output-dir experiment-runs/example \
  --overwrite \
  --schema schemas/classification.schema.json \
  --prompt-template examples/profiles/repo-routing.prompt.md \
  --topic-taxonomy examples/profiles/repo-routing-topics.json \
  --reference-model mock \
  --target-model mock
```

Use a real local OpenAI-compatible endpoint by setting the model side. Non-mock
models are routed through the normal Localpager classifier wrapper and
`localpager-agent --final-schema`:

```bash
node scripts/localpager-experiment.mjs \
  --repo owner/repo \
  --limit 3 \
  --reference-model mock \
  --target-base-url http://127.0.0.1:1234/v1 \
  --target-model gemma-4-e4b-it
```

If `GITHUB_TOKEN` is set, the runner uses it. The token value is never written
to the output directory.

## Inputs

- `--schema`: base classifier schema. The default is
  `schemas/classification.schema.json`.
- `--prompt-template`: prompt template. The default is
  `examples/profiles/repo-routing.prompt.md`.
- `--topic-taxonomy`: allowed topic list. The default is
  `examples/profiles/repo-routing-topics.json`.
- `--classifier-command`: classifier wrapper to run for non-mock models. The
  default is `scripts/localpager-classifier`.
- `--repo`: GitHub repo in `owner/repo` form.
- `--limit`: total item count after sorting recent issues and PRs together.

The prompt template supports the same profile placeholders as
`localpager-render-profile.mjs`:

- `__TARGET__`
- `__GITHUB_CONTEXT__`
- `__ALLOWED_TOPICS_JSON__`
- `__TOPIC_TAXONOMY_JSON__`
- `__TOPIC_DESCRIPTIONS__`

The runner refuses to write into a non-empty output directory unless
`--overwrite` is passed. This keeps repeated runs from mixing old prompt files
with current results.

## Output Bundle

Each run writes:

- `config.json`: run configuration with credentials omitted.
- `schema.runtime.json`: schema after topic enum injection.
- `items.jsonl`: fetched item summaries.
- `reference-outputs.jsonl`: reference model outputs.
- `target-outputs.jsonl`: target model outputs.
- `per-row-results.jsonl`: row-level comparison records.
- `summary.md`: human-readable result summary.
- `contexts/`: rendered GitHub context blocks.
- `prompts/`: final prompts sent to the model.

`experiment-runs/` is ignored by git. Keep useful summaries by copying the
important numbers into a dated doc instead of committing raw run directories.

## Smoke Verification

The initial runner was checked with:

```bash
node --check scripts/localpager-experiment.mjs
node scripts/localpager-experiment.mjs \
  --repo openclaw/openclaw \
  --limit 2 \
  --item-type both \
  --output-dir /tmp/localpager-experiment-smoke \
  --overwrite \
  --reference-model mock \
  --target-model mock
```

That smoke test verifies GitHub fetch, issue/PR hydration, prompt rendering,
runtime schema generation, output validation, metric calculation, and artifact
writing without needing a local model server.

A one-row LM Studio smoke test can be run against the local Gemma endpoint:

```bash
node scripts/localpager-experiment.mjs \
  --repo openclaw/openclaw \
  --limit 1 \
  --item-type both \
  --output-dir /tmp/localpager-experiment-gemma-smoke \
  --overwrite \
  --reference-model mock \
  --target-base-url http://127.0.0.1:1234/v1 \
  --target-model gemma-4-e4b-it
```

The comparison score is not meaningful when the reference side is the mock
classifier, but the model call, final-schema parse, runtime topic enum, and
validator path are exercised.

## DS4 Final-Schema Verification

Do not keep DS4 and LM Studio loaded at the same time on the local machine. DS4
uses the large DeepSeek model on port `8000`; LM Studio commonly serves Gemma on
port `1234`. Before DS4 verification, stop LM Studio and verify that only DS4 is
resident:

```bash
lms server stop
nvidia-smi --query-compute-apps=pid,process_name,used_memory --format=csv,noheader
```

The experiment runner now uses `localpager-classifier` for non-mock models, so
DS4 verification goes through the same `localpager-agent` final-schema path as
the production classifier setup.

Verified command shape:

```bash
LOCALPAGER_AGENT_BASE_URL=http://127.0.0.1:8000/v1 \
node scripts/localpager-experiment.mjs \
  --repo openclaw/openclaw \
  --limit 1 \
  --item-type prs \
  --output-dir /tmp/localpager-experiment-ds4-smoke \
  --overwrite \
  --reference-model mock \
  --target-base-url http://127.0.0.1:8000/v1 \
  --target-model deepseek-v4-pro \
  --context-window 32768 \
  --max-tokens 768 \
  --timeout-ms 600000 \
  --schema schemas/classification.schema.json \
  --prompt-template examples/profiles/repo-routing.prompt.md \
  --topic-taxonomy examples/profiles/repo-routing-topics.json
```

Result:

```json
{
  "topics_of_interest": ["docs"],
  "description": "PR 88875 is a comment-only maintainability pass adding public/API docs and inline comments across markdown, shared helpers, channel, gateway, plugin SDK, CLI, and security-adjacent contracts.",
  "caveats": [
    "GitHub diff was unavailable because the PR exceeded GitHub's 300-file diff limit.",
    "The PR body reports local test results but lacks attached proof."
  ]
}
```

This verifies that Localpager can send a rendered GitHub context plus runtime
topic enum to DS4 and receive schema-valid final JSON without loading Gemma.
