---
title: DS4 Dataset and Gemma Prompt Optimization
author: Bob <dutifulbob@gmail.com>
date: 2026-06-01
---

# DS4 Dataset and Gemma Prompt Optimization

This records the local experiment that produced the DS4-labeled OpenClaw
classification dataset and then used that dataset to improve Gemma 4 prompting.

It is intentionally about prompt and dataset work only. The production lesson
for Localpager is: keep the classifier schema strict, inject the full GitHub
context into the prompt, and fix model behavior through prompt/profile changes,
not through hidden topic rewriting logic.

## Artifact Map

Primary local dataset folder:

```text
/home/bob/oc/openclaw-classification-dataset
```

Canonical Hugging Face dataset:

```text
dutifuldev/openclaw-classification-dataset
```

URL:

```text
https://huggingface.co/datasets/dutifuldev/openclaw-classification-dataset
```

Important files in that folder:

- `seed.jsonl`: original 638-row curated dataset.
- `row.schema.json`: dataset row schema.
- `topic_keywords.json`: topic taxonomy and keyword hints.
- `github-interest-classifier.openai.schema.json`: OpenAI-compatible schema
  used by benchmark tooling.
- `generate_deepseek_localagent_dataset.mjs`: DS4 dataset generation script.
- `benchmark_model_comparison.mjs`: Gemma, DS4, and Codex comparison script.
- `benchmark_ds4_concurrency.mjs`: local DS4 server throughput probe.
- `prompt-snapshots/`: recovered and saved prompt snapshots.
- `manual-prompt-experiments/ds4-precision/`: prompt-only iterations for
  improving Gemma precision.
- `benchmark-runs/`: benchmark configs, summaries, and per-row results.

Hugging Face upload staging for `dutifuldev/openclaw-classification-dataset`:

```text
/home/bob/oc/openclaw-classification-dataset/hf-ds4-upload
```

That staging directory has these 742-line files:

- `codex-batch.jsonl`: original dataset generated with Codex in batched mode.
- `ds4.jsonl`: same rows plus `deepseek_localagent.output`.
- `ds4-outputs.jsonl`: raw DS4 per-row output records.
- `regression-set.json`, `row.schema.json`, `topic_keywords.json`.

Prompt snapshots:

- `prompt-snapshots/2026-05-29-classifier-prompt.md`
- `prompt-snapshots/2026-05-30-deepseek-localagent-generation-prompt.md`
- `prompt-snapshots/2026-05-30-deepseek-localagent-runtime-template.md`
- `prompt-snapshots/2026-05-30-deepseek-localagent-runtime-example-0001.md`
- `prompt-snapshots/2026-05-30-deepseek-localagent-runtime-prompt-0001.md`
- `prompt-snapshots/2026-05-30-deepseek-localagent-runtime-prompts.jsonl`
- `prompt-snapshots/localpager-openclaw-routing-v8-production.prompt.md`
- `scripts/generate_deepseek_localagent_dataset.mjs`

Remote prompt provenance:

- Original DS4 generation prompt:
  <https://huggingface.co/datasets/dutifuldev/openclaw-classification-dataset/blob/main/prompt-snapshots/2026-05-30-deepseek-localagent-generation-prompt.md>
- Representative original DS4 runtime prompt:
  <https://huggingface.co/datasets/dutifuldev/openclaw-classification-dataset/blob/main/prompt-snapshots/2026-05-30-deepseek-localagent-runtime-prompt-0001.md>
- DS4 runtime template rendered by the original generator from a placeholder seed row:
  <https://huggingface.co/datasets/dutifuldev/openclaw-classification-dataset/blob/main/prompt-snapshots/2026-05-30-deepseek-localagent-runtime-template.md>
- Rendered DS4 runtime prompt example:
  <https://huggingface.co/datasets/dutifuldev/openclaw-classification-dataset/blob/main/prompt-snapshots/2026-05-30-deepseek-localagent-runtime-example-0001.md>
- Rendered DS4 runtime prompts:
  <https://huggingface.co/datasets/dutifuldev/openclaw-classification-dataset/blob/main/prompt-snapshots/2026-05-30-deepseek-localagent-runtime-prompts.jsonl>
- DS4 generator script that produced the rendered prompts:
  <https://huggingface.co/datasets/dutifuldev/openclaw-classification-dataset/blob/main/scripts/generate_deepseek_localagent_dataset.mjs>
- Gemma v8 prompt experiment:
  <https://huggingface.co/datasets/dutifuldev/openclaw-classification-dataset/blob/main/prompt-experiments/ds4-precision/routing-intent-v8-fp-table.md>
- Final Localpager/Gemma production prompt:
  <https://huggingface.co/datasets/dutifuldev/openclaw-classification-dataset/blob/main/prompt-snapshots/localpager-openclaw-routing-v8-production.prompt.md>
- Full-dataset prompt comparison:
  <https://huggingface.co/datasets/dutifuldev/openclaw-classification-dataset/blob/main/prompt-experiments/ds4-precision/full-638-20260601-093700/comparison-note.md>

## Dataset Lineage

The dataset had two layers.

First, there was an original 638-row dataset produced with Codex in batched
mode. That became `seed.jsonl` locally and `codex-batch.jsonl` in the upload
staging directory.

Second, every row was reclassified through a fully local DS4 run. Those DS4
outputs were not used to delete the original labels. They were attached as a new
field:

```json
{
  "deepseek_localagent": {
    "model": "deepseek-v4-pro",
    "base_url": "http://127.0.0.1:8000/v1",
    "generated_at": "...",
    "prompt_chars": 12345,
    "elapsed_seconds": 12.3,
    "session_dir": "...",
    "error": null,
    "output": {
      "topics_of_interest": ["..."],
      "interest": "i2",
      "confidence": 0.73,
      "description": "...",
      "caveats": []
    }
  }
}
```

The older dataset schema still had `interest` and `confidence`. Localpager later
removed those from its runtime schema. For Localpager, the useful part of the
DS4 output is the topic set and the short explanation/caveats; `interest` and
`confidence` should not come back.

The DS4 upload naming was deliberately generic:

- original batched dataset: `codex-batch.jsonl`
- DS4-augmented dataset: `ds4.jsonl`
- raw DS4 outputs: `ds4-outputs.jsonl`

## Local DS4 Generation Run

The DS4 run was fully local. It used a local OpenAI-compatible DS4 endpoint,
then called it through localagent so schema-constrained final output would be
captured with session transcripts.

Recorded config:

```json
{
  "input_path": "/home/bob/oc/openclaw-classification-dataset/seed.jsonl",
  "output_dir": "/home/bob/oc/openclaw-classification-dataset/deepseek-localagent",
  "schema_path": "/home/bob/clawd-notifier-impl/schemas/github-interest-classifier.schema.json",
  "policy_path": "/home/bob/clawd-notifier-impl/skills/openclaw-maintainer/github-classifier-policy.md",
  "topic_keywords_path": "/home/bob/oc/openclaw-classification-dataset/topic_keywords.json",
  "localagent_command": "/home/bob/.nvm/versions/node/v22.22.0/bin/localagent",
  "base_url": "http://127.0.0.1:8000/v1",
  "model": "deepseek-v4-pro",
  "context_window": 32768,
  "max_tokens": 768,
  "timeout_ms": 1200000,
  "prompt_limits": {
    "body_chars": 2500,
    "comments_chars": 1500,
    "diff_chars": 5000,
    "changed_files_chars": 2000,
    "topic_keyword_limit": 3
  }
}
```

Final progress:

```json
{
  "startedAt": "2026-05-29T01:40:37.742Z",
  "updatedAt": "2026-05-29T13:51:48.011Z",
  "totalRows": 638,
  "selectedRows": 638,
  "completedRows": 638,
  "errorRows": 0,
  "done": true,
  "model": "deepseek-v4-pro",
  "baseUrl": "http://127.0.0.1:8000/v1"
}
```

The effective localagent command shape was:

```bash
/home/bob/.nvm/versions/node/v22.22.0/bin/localagent \
  --base-url http://127.0.0.1:8000/v1 \
  --model deepseek-v4-pro \
  --max-tokens 768 \
  --timeout-ms 5000 \
  --context-window 32768 \
  --final-schema /home/bob/clawd-notifier-impl/schemas/github-interest-classifier.schema.json \
  --session-dir <output-dir>/sessions/<row-index>-<row-id> \
  -p <rendered prompt>
```

The generator's outer process timeout was 1200000 ms per row. The localagent
CLI probe timeout stayed at the default 5000 ms.

Resume command recorded by the generator:

```bash
node /home/bob/oc/openclaw-classification-dataset/generate_deepseek_localagent_dataset.mjs
```

Retry command:

```bash
node /home/bob/oc/openclaw-classification-dataset/generate_deepseek_localagent_dataset.mjs --retry-errors
```

## DS4 Prompt Shape

The DS4 generation prompt was rendered per row. It had five major parts:

1. Classifier instruction: classify one OpenClaw GitHub issue or PR.
2. Output discipline: call `final_json` exactly once, with no prose.
3. Allowed enum lists: `topics_of_interest` and the older `interest` enum.
4. Topic keyword hints: up to three hint phrases per topic.
5. Dataset-aligned labeling policy plus injected GitHub context.

The static header started with these ideas:

```text
Classify this OpenClaw GitHub issue or pull request against Onur's current
topics of interest.

Do not write prose, analysis, markdown, or JSON text in the assistant response.
Submit the answer by calling final_json exactly once.

Use the injected GitHub context first. It was collected before this model run
from GitHub, and may be truncated to keep the prompt small.
```

The row context shape was:

````text
GitHub item:
- Repository: <repo>
- Type: <pull_request|issue>
- Number: <number>
- URL: <url>
- Title: <title>
- State: <state>
- Author: <author>
- Labels: <labels>
- Changed file count available to wrapper: <count>
- Changed files: <changed files>
- Context caveats: <caveats>

Body:
```markdown
<body>
```

Comments/context:
```markdown
<comments>
```

Diff/context:
```diff
<selected diff>
```
````

The prompt renderer truncated large fields:

- body: 2500 chars
- comments: 1500 chars
- changed files: 2000 chars
- selected diff: 5000 chars
- topic keyword hints: 3 examples per topic

Diff selection kept file headers, hunk headers, and lines containing relevant
terms like local model, LM Studio, vLLM, Ollama, llama.cpp, Gemma, classifier,
MCP, ACP, ACPX, Codex, Hugging Face, model serving, open weight, and self-hosted.

The renderer also neutralized control-like tags in source content before prompt
injection. It changed tags such as `<system` and `</system` so issue text could
not act like a hidden instruction.

Prompt recovery details:

- 638 runtime prompts were recovered from saved localagent session transcripts.
- runtime prompt JSONL SHA-256:
  `7dadd6a48b26dddc806eb544711b8f4c552054c814b2ac6107e93d53055d8ff4`
- first sample prompt SHA-256:
  `4d0e9dbc8754c5c9824b481146c059338bdaaf789f65eb622ffe87931bab20df`

## Benchmark Setup

The benchmark script compared local Gemma, local DS4, and optionally Codex on
the same rendered classifier prompt.

Default model endpoints:

```text
gemma: gemma-4-e4b-it at http://127.0.0.1:1234/v1
ds4: deepseek-v4-pro at http://127.0.0.1:8000/v1
codex: codex CLI with model gpt-5.5
```

Default benchmark settings:

- sample: `regression`
- limit: 80, but the regression manifest had 30 rows
- max tokens: 768
- temperature: 0
- concurrency: 1
- topic keyword hint limit: 3
- tool/function name: `final_json`

The benchmark measured:

- exact topic-set match
- micro precision
- micro recall
- micro F1
- topic false positives and false negatives
- per-row latency
- completion tokens per second
- largest misses
- weakest topics

For DS4-as-ground-truth scoring, the benchmark used:

```bash
node /home/bob/oc/openclaw-classification-dataset/benchmark_model_comparison.mjs \
  --dataset-file /home/bob/oc/openclaw-classification-dataset/hf-ds4-upload/ds4.jsonl \
  --expected-source ds4 \
  --models gemma \
  --sample regression
```

## DS4 Server Throughput Probe

The DS4 concurrency benchmark showed that independent requests were effectively
serialized through one graph worker. Higher client concurrency mostly increased
queueing latency.

```text
model: deepseek-v4-pro
base URL: http://127.0.0.1:8000/v1
max tokens: 768
```

| concurrency | requests | errors | wall s | avg latency s | p95 latency s | prompt tok/s | completion tok/s |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 1 | 0 | 29.539 | 29.539 | 29.539 | 167.44 | 7.719 |
| 2 | 2 | 0 | 59.767 | 44.966 | 59.766 | 143.407 | 8.433 |
| 3 | 3 | 0 | 80.344 | 53.720 | 80.342 | 131.099 | 8.551 |

The practical rule was to use concurrency 1 for repeatable scoring.

## Initial Regression Benchmark

The initial model comparison used the original 638-row dataset and the 30-row
regression sample.

Run directory:

```text
/home/bob/oc/openclaw-classification-dataset/benchmark-runs/2026-05-28T08-40-57-156Z
```

| model | errors | exact | precision | recall | F1 | FP | FN | rows/min | avg latency s | p95 latency s | completion tok/s |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| codex | 0 | 0.000 | 0.606 | 0.701 | 0.651 | 61 | 40 | 5.864 | 10.231 | 16.195 | 0 |
| ds4 | 0 | 0.033 | 0.629 | 0.418 | 0.502 | 33 | 78 | 2.233 | 26.871 | 30.533 | 7.878 |
| gemma | 1 | 0.000 | 0.615 | 0.418 | 0.498 | 35 | 78 | 9.030 | 6.644 | 7.807 | 19.224 |

Gemma had one schema failure in that run:

```text
openclaw-openclaw-77748: invalid topic: docker
```

The main early failure mode was low exact match with many missed topics. This
was partly because the original labels were broad and sometimes multi-topic.

## Gemma Scored Against DS4 Labels

After the DS4-labeled dataset existed, Gemma was scored against DS4 outputs as
the expected topic set.

Run directory:

```text
/home/bob/oc/openclaw-classification-dataset/benchmark-runs/2026-05-29T14-41-50-480Z
```

Config highlights:

```json
{
  "dataset_file": "/home/bob/oc/openclaw-classification-dataset/hf-ds4-upload/ds4.jsonl",
  "expected_source": "ds4",
  "row_count_total": 638,
  "row_count_evaluated": 30,
  "sample": "regression",
  "models": [
    {
      "key": "gemma",
      "model": "gemma-4-e4b-it",
      "baseUrl": "http://127.0.0.1:1234/v1"
    }
  ],
  "max_tokens": 768,
  "temperature": 0
}
```

Result:

| model | errors | exact | precision | recall | F1 | FP | FN |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| gemma vs DS4 | 1 | 0.033 | 0.407 | 0.712 | 0.517 | 54 | 15 |

This changed the diagnosis. Against DS4 labels, Gemma recalled many DS4 topics
but over-labeled badly. The problem was no longer mainly "Gemma misses topics";
it was "Gemma adds unnecessary topics."

Worst false-positive topics in that run:

| topic | true positives | false positives | false negatives | precision | recall |
| --- | ---: | ---: | ---: | ---: | ---: |
| config | 2 | 11 | 0 | 0.154 | 1.000 |
| tool_calling | 0 | 9 | 0 | 0.000 | 0.000 |
| reliability | 6 | 6 | 1 | 0.500 | 0.857 |
| api_surface | 0 | 6 | 0 | 0.000 | 0.000 |
| local_model_providers | 0 | 3 | 0 | 0.000 | 0.000 |
| self_hosted_inference | 3 | 3 | 0 | 0.500 | 1.000 |

Plain diagnosis: Gemma was treating implementation details, changed files, test
files, config-looking snippets, and broad component words as labels. It needed
stronger instructions to label only the central routing reason.

## Prompt-Only Optimization Rules

The optimization pass was deliberately prompt-only.

Allowed:

- change prompt wording
- change topic boundary descriptions
- change examples and tie-breakers
- change the ordering and strength of decision checks
- keep the JSON schema strict

Not allowed:

- deterministic topic rewriting
- adding code that deletes "bad" labels after model output
- row-specific if/then rules
- letting the model invent topics outside the schema
- replacing topic labels with another hidden scoring field

This is why the manual prompt experiment files are useful: they show the prompt
pressure that improved Gemma without adding custom classifier logic.

## Prompt Iteration Timeline

All prompt-only candidates are in:

```text
/home/bob/oc/openclaw-classification-dataset/manual-prompt-experiments/ds4-precision
```

The files are small policy fragments. They were tested by swapping them into
the classifier policy/prompt and rerunning representative rows or regression
checks. The useful progression was:

1. `anti-fp-examples.md`

   Added concrete negative examples for common false positives. Examples:
   session status count should route to `telemetry_usage`, base URL
   normalization should not become `local_model_providers`, and Telegram media
   directive bugs should not become `api_surface`.

2. `short-central.md`

   Compressed the rule into "central reason only." Most items should have one
   topic. A second topic is allowed only when the title or main body clearly has
   two independent central concerns. This was the simplest form of the final
   idea.

3. `strict-1-2.md`

   Started from the broader dataset policy and added a precision label budget:
   default to exactly one topic, use two only when both are independently
   central, and treat three or more as rare.

4. `removal-test.md`

   Added the most useful deletion test: for each candidate topic, ask whether
   removing it still leaves enough labels to explain the item. If yes, remove
   it. This directly targets Gemma's extra-label habit.

5. `broad-suppression.md`

   Added explicit high-risk false-positive topics for Gemma:
   `config`, `reliability`, `gateway`, `api_surface`, `tool_calling`,
   `agent_runtime`, `docs`, `tests_ci`, `security`, and
   `local_model_providers`. These require stronger central evidence than normal.

6. `combined-prune-broad.md`

   Combined the removal test with broad-topic suppression. This was useful
   because Gemma often followed one rule but ignored the other unless both were
   present close together.

7. `routing-intent.md`

   Reframed the whole task as notification routing, not code search. The model
   should pick the smallest set of topics needed to route the item to the right
   interest bucket, not describe every technical area touched.

8. `routing-intent-v2.md`

   Added positive routing cues so the model knew the intended destination for
   common phrases:
   `telemetry_usage` for counts/costs/metrics, `coding_agents` for subagent and
   harness behavior, `local_models` for local execution, `model_serving` for
   endpoint semantics, `chat_integrations` for named chat surfaces, and
   `ui_tui` for interface display/status behavior.

9. `routing-intent-v3.md`

   Added tie-breakers for nearby topics. This separated telemetry from UI or
   sessions, model serving from provider setup, chat integrations from generic
   notifications, and reliability from notification delivery.

10. `routing-intent-v4-title-first.md`

    Added title-first centrality. The title is the strongest routing evidence.
    Body, labels, files, comments, and diff can confirm the title or add one
    essential second topic, but should not broaden the label set.

11. `routing-intent-v4-second-label-gate.md`

    Added a gate for the second label: after choosing the best topic, add a
    second only if it changes who should be notified. This specifically fights
    "true but unnecessary" labels.

12. `routing-intent-v4-budgeted-cues.md`

    Rephrased the same idea as budgeted cues: normal budget is one topic; a
    second needs its own direct cue and must be needed for routing.

13. `routing-intent-v5-title-gate.md`

    Combined title-first centrality with the second-label gate. This became the
    strongest general shape: title decides the primary route; a second label
    needs explicit central evidence.

14. `routing-intent-v6-central-phrases.md`

    Added central phrase tie-breakers. Examples: counts and token/cost/status
    wording route to `telemetry_usage`; base URL normalization and endpoint
    lifecycle route to `model_serving`; execution-control behavior routes to
    `exec_tools`; thread/session isolation routes to `sessions`.

15. `routing-intent-v7-enum-boundaries.md`

    Added enum discipline after Gemma invented labels like `docker`. The model
    must output exact allowed enum strings only. If the closest word in the
    title is not an allowed enum, map it to the nearest allowed enum or omit it.

16. `routing-intent-v8-fp-table.md`

    Added the final false-positive suppression table. This table calls out the
    most common bad substitutions:

    - do not use `local_model_providers` for base URL normalization, endpoint
      lifecycle, streaming, usage chunks, or vLLM/TGI/LocalAI serving behavior
    - do not use `notifications` for named Discord/Telegram/Slack behavior or
      generic recovery correctness
    - do not use `tool_calling` for TTS tags/options, browser screenshot/vision,
      generic tool output, or config-like options
    - do not use `api_surface` for parse helpers, CLI edge-case tests, token
      parsing, status/footer display, internal command behavior, or local model
      compatibility
    - do not use `config` merely because a feature adds an option

## What Changed From Initial to Final Prompt

The initial production-style prompt was mostly taxonomy and evidence based:

- classify the GitHub item
- use injected context first
- choose up to five allowed topics
- prefer concrete evidence
- avoid broad OpenClaw words
- use topic-specific boundaries for local models, ACPX, ACP, MCP, Codex, and
  similar topics

That was good enough to stop many invented labels, but not enough to stop
Gemma from over-labeling. Gemma still treated implementation details as
secondary labels.

The final useful prompt direction added four stronger ideas:

1. The classifier is routing a notification, not indexing code.
2. One topic is the default; a second topic must change routing.
3. The title and first clear problem statement are stronger than files, tests,
   labels, or implementation details.
4. High-risk broad topics need explicit central evidence.

In plain language: label the reason the maintainer should care, not everything
the PR touched.

## Current Localpager Translation

Localpager should carry these lessons forward in a deployment profile:

- exact prompt template is configurable with `classifier.prompt_template`
- exact schema is configurable with `classifier.schema`
- exact topic list is configurable with `classifier.topic_taxonomy`
- GitHub title/body/labels/comments/changed files/diff are injected into
  `__GITHUB_CONTEXT__`
- the runtime schema rejects topics outside the configured taxonomy
- Localpager core does not hardcode OpenClaw labels
- Localpager core does not rewrite model topics after classification

The old dataset prompt included `interest` and `confidence`; Localpager should
not. The current runtime fields are:

```json
{
  "topics_of_interest": ["local_models"],
  "description": "One concise evidence-backed sentence.",
  "caveats": []
}
```

## Localpager DS4 Verification

The Localpager translation was checked against DS4 without loading LM Studio at
the same time. The machine guardrail is: stop LM Studio/Gemma first, then verify
that only `ds4-server` remains in GPU compute apps.

For Onur's current `isengard` Localpager/Gemma setup, including the exact LM
Studio context, parallelism, GitHub context budget, and customization steps, see
[Onur Isengard Localpager Setup](2026-06-02-onur-isengard-localpager-setup.md).

`scripts/localpager-experiment.mjs` now uses the same classifier setup as
Localpager for non-mock models. For DS4, that means the experiment runner and
production classifier both go through `localpager-classifier` and
`localpager-agent` final-schema output:

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

A one-PR smoke run against `openclaw/openclaw#88875` returned valid JSON with
`topics_of_interest`, `description`, and `caveats`, proving the Localpager
profile can run through DS4 with injected GitHub context and a strict runtime
topic enum.

## Reproduction Checklist

To reproduce this style of local dataset creation for another repo:

1. Build or collect a seed JSONL dataset with full GitHub context:
   title, body, labels, comments, changed files, and diff.
2. Define a strict JSON schema with an enum for `topics_of_interest`.
3. Define topic keyword hints, but keep them as hints only.
4. Run the stronger local model first to create a model-labeled reference set.
5. Save raw outputs separately from the original rows.
6. Preserve the original rows and add generated output under a new nested field.
7. Snapshot the exact generation prompt and runtime prompts.
8. Evaluate the target local model against both the original labels and the
   generated reference labels.
9. Diagnose false positives and false negatives by topic.
10. Improve the prompt only. Do not add post-output topic cleanup logic.
11. Keep prompt candidates as separate files with clear names.
12. Promote only the rules that improve general behavior, not row-specific
   memorization.

## What To Preserve

The useful parts to preserve in Localpager documentation and future scripts are:

- the full local-only generation config
- the exact prompt snapshot
- the recovered runtime prompts
- the 638-row completion and 0-error status
- the DS4 output JSONL separate from the DS4-augmented dataset
- benchmark configs and summaries
- per-row largest misses
- prompt candidate files and the reasoning behind each
- the final failure-mode diagnosis: Gemma tends to over-label broad or
  implementation-adjacent topics unless the prompt forces central routing
  labels

The most important operational rule is simple: any future classifier quality
work should be reproducible from saved dataset rows, saved prompt text, saved
schema, saved topic taxonomy, and saved benchmark output.
