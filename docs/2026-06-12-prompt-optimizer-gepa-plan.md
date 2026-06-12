---
title: Prompt Optimizer (GEPA) — Plan
author: Bob <dutifulbob@gmail.com>
date: 2026-06-12
---

# Prompt Optimizer (GEPA) — Plan

This plans a `prompt-optimizer/` tool that uses GEPA to improve the Localpager
classifier prompt. It is a plan only; no code is committed yet.

## Goal

Automatically improve the OpenClaw topic-classifier prompt so the local model
labels GitHub items more accurately, without hand-tuning each rule and without
adding any post-output topic-rewriting logic.

## Core principle

GEPA is an offline lab. The only thing it touches in production is the prompt
file. It reads the current prompt, evolves it against a labelled dataset using
the real classifier harness for scoring, and emits an improved prompt file that
a human reviews and commits. No optimizer code runs in the Go worker or the TS
agent, and nothing in the notify path changes.

This matches Localpager's existing contract: the classifier prompt is already a
deployment-owned artifact (`classifier.prompt_template`). GEPA just produces a
better version of that artifact.

## Why a Python tool in this repo

- The maintained GEPA implementation is the Python `gepa` package. We use it
  directly and write only the glue; we do not reimplement the algorithm.
- The repo is already polyglot (`localpager-agent/` is a Node subproject), so a
  Python subproject is the same pattern, isolated in its own directory.
- Co-location is a real win: the tool reads the repo's schema and taxonomy, uses
  the selected dataset prompt source, drives the repo's classifier harness, and
  emits a new prompt file — all in one repo and one review.

## Location and isolation

The tool lives at `prompt-optimizer/` and is isolated from the Go build and CI:

- Own `pyproject.toml` with a pinned `gepa` dependency plus an OpenAI-compatible
  client; virtualenv is gitignored.
- Touches neither `go.mod` nor `localpager-agent/package.json`.
- Excluded from `scripts/check.sh` and the Go/slophammer gates. Optional own
  gates (`ruff`, `pytest`) may be added later; none are required while it is
  experimental.
- Talks to Localpager only through the stable seam (the classifier command) and
  the shared files. No imports across the language boundary.

## What GEPA optimizes (the candidate)

Seed the run from the OpenClaw classification dataset prompt
`prompts/localpager-openclaw-routing-v9.1-monologue-cap.hbs` in
`dutifuldev/openclaw-classification-dataset` at revision
`8d1088276425ca72a5313c18cde4adef20ffe194`.

The candidate is the editable routing-policy block of that v9.1 prompt, not the
whole template. The scaffold stays frozen: output-shape rules, repository-read
instructions, the v9.1 inner-monologue cap, topic placeholders, and target /
GitHub-context placeholders.

The optimizer should normalize the dataset Handlebars variables
(`{{target}}`, `{{{github_context}}}`, `{{{allowed_topics_json}}}`,
`{{{topic_descriptions}}}`) to the Localpager renderer placeholders
(`__TARGET__`, `__GITHUB_CONTEXT__`, `__ALLOWED_TOPICS_JSON__`,
`__TOPIC_DESCRIPTIONS__`) before assembly, so candidates still run through the
same profile renderer as production.

```python
seed_candidate = {"routing_policy": "<the decision rules + cue tables>"}
```

This is GEPA's "module" notion. It guarantees GEPA can only rewrite prose it is
allowed to, and can never break the placeholder or schema contract. The template
is assembled as: frozen scaffold + the current `routing_policy` block.

## Evaluation harness (the transfer rule)

Candidates are scored through the same path production uses, so a gain in the
lab transfers to deployment:

- Render the candidate with the production renderer
  (`scripts/localpager-render-profile.mjs`) so placeholders and the
  taxonomy-derived schema enum match exactly.
- Run it through `localpager-classifier` → `localpager-agent` → Pi → the
  already-loaded local 12B model → `final_json`.

To keep many rollouts tractable:

- Pre-fetch and cache each dataset row's GitHub context once (pass
  `--github-context-file`), so rollouts never re-hit GitHub.
- Keep the model server warm.
- Run at concurrency 2: at most two classifier calls in flight against the local
  12B model.
- Add the one production-side seam below so rows stream through a warm process
  instead of spawning Pi per row.

## Scoring and ASI

- Optimization metric (μ): a weighted multilabel routing score computed from
  false positives, false negatives, and over-labeling against the DS4 labels.
  Initial row loss:

  ```text
  2.0 * false_positives + 1.0 * false_negatives + 0.5 * max(0, predicted_count - gold_count)
  ```

  False positives are intentionally worse than false negatives, and over-labeling
  gets an extra penalty beyond the individual false-positive labels. Report topic
  micro-F1, per-topic precision, and per-topic recall alongside this objective,
  but select candidates by the weighted score.
- Feedback / ASI (μ_f): the per-row, per-topic mistakes turned into short
  natural-language notes, e.g. "item 412: predicted `config`; gold has none — an
  option was added but config behavior is not the subject." This is the same
  signal that drove the manual prompt gains, produced automatically.

## Models

- Task / evaluator LM: the already-loaded local 12B model we actually serve.
  Non-negotiable for transfer; the prompt is tuned to that loaded model's quirks.
- Reflection / proposer LM: a stronger API model (e.g. Codex / GPT-5.x), passed
  as `reflection_lm`.

## Data and reproducibility

- Three-way split of the curated dataset: a feedback/minibatch pool, a
  `D_pareto` validation set for candidate selection, and a held-out test set the
  optimizer never sees (report final transfer on it).
- Pin sampling: temperature 0, fixed seed, matching the production worker.
- Log the dataset revision, v9.1 seed prompt path, loaded 12B model identifier,
  concurrency, every candidate, its scores, and the chosen prompt to a run
  directory so a run is reproducible from saved inputs.

## Package layout

```
prompt-optimizer/
  pyproject.toml                 # depends on gepa + an OpenAI-compatible client
  README.md                      # what it does, how to run
  src/prompt_optimizer/
    harness.py                   # render candidate + run classifier, parse final_json
    metric.py                    # weighted score, topic F1, ASI text from FP/FN
    adapter.py                   # LocalpagerAdapter(gepa GEPAAdapter)
    dataset.py                   # load DS4 rows, cache GitHub contexts, split
    run.py                       # gepa.optimize(...)
  out/                           # run reports + candidate prompts (gitignored; keep winners)
```

## GEPA integration

`adapter.py` implements the GEPA adapter protocol (verify exact signatures
against the installed `gepa` version):

- `evaluate(batch, candidate, capture_traces)` → for each row: assemble the
  prompt from frozen scaffold + `candidate["routing_policy"]`, run it through the
  harness, parse `final_json`, score vs DS4. Returns outputs, scores, and (when
  `capture_traces`) the wrong-topic details.
- `make_reflective_dataset(candidate, eval_batch, components_to_update)` → turn
  the captured mistakes into the ASI notes above, keyed by `routing_policy`.

`run.py` calls `gepa.optimize(seed_candidate, trainset, valset, adapter,
reflection_lm, max_metric_calls=...)` and writes the winning `routing_policy`
plus a report to `out/`.

## The one production change

Add a batch / RPC classify mode to `localpager-agent` so the adapter can stream
many rows through one warm agent process instead of spawning Pi per row. This is
the only change outside `prompt-optimizer/`, it keeps evaluation faithful, and
it is independently useful.

## Milestones

1. Scaffold `prompt-optimizer/` (pyproject, package skeleton, README).
2. `dataset.py`: load DS4 rows, cache the v9.1 seed prompt and GitHub contexts,
   three-way split.
3. `harness.py` + `metric.py`: classify one row through the real harness and
   score it vs DS4 using the weighted FP/FN/over-labeling objective, with a mock
   model path for fast tests.
4. `adapter.py`: implement `evaluate` and `make_reflective_dataset`.
5. `run.py`: wire `gepa.optimize`; do a tiny smoke run on a handful of rows.
6. Add the batch-classify seam to `localpager-agent`; switch the harness to it.
7. Full run; review the winning prompt; commit it as the new
   `classifier.prompt_template`.

## Open questions

- Reflection model and its access (API key handling, kept out of the repo).
- Rollout budget vs. wall-clock on the already-loaded local 12B model with
  concurrency fixed at 2.
- Whether to ever optimize more than the prompt (schema or topic list); if so,
  the library's `optimize_anything` / adapter framing is the fit, and the
  candidate grows beyond `routing_policy`.
