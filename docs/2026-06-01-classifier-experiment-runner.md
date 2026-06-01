---
title: Classifier Experiment Runner
author: Bob <dutifulbob@gmail.com>
date: 2026-06-01
---

# Classifier Experiment Runner

Localpager includes a dependency-free experiment runner for checking a prompt
profile against live GitHub issues and pull requests.

The runner is intentionally generic. It takes a repo, a prompt template, a topic
taxonomy, and a schema. It fetches a small GitHub sample, renders the same kind
of context Localpager sends to the classifier, runs a reference model and a
target model, validates the outputs, then writes a benchmark bundle.

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

Use a real local OpenAI-compatible endpoint by setting the model side:

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

A one-row LM Studio smoke test was also run against the local Gemma endpoint:

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

That produced a valid target output for one live PR. The comparison score was
not meaningful because the reference side was the mock classifier, but the model
call, structured JSON parse, runtime topic enum, and validator path all worked.
