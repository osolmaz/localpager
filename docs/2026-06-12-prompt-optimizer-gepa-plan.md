---
title: Prompt Optimizer (GEPA) Plan
author: Bob <dutifulbob@gmail.com>
date: 2026-06-12
---

# Prompt Optimizer (GEPA) Plan

This records the current design for `prompt-optimizer/`, the offline GEPA tool
used to improve the Localpager OpenClaw classifier prompt. The production
worker does not run GEPA. GEPA produces candidate prompt text, we evaluate it,
then a reviewed prompt artifact can be committed.

## Goal

Improve OpenClaw GitHub-item topic routing on the evalstate OpenClaw Git-label
dataset without adding post-output topic rewriting, hidden label repair, or
dataset-specific hacks.

## Current Dataset

The current optimizer uses evalstate's dataset checkout:

```text
/home/bob/repos/openclaw-git-labels/data/splits/feedback300.jsonl
/home/bob/repos/openclaw-git-labels/data/splits/pareto60.jsonl
/home/bob/repos/openclaw-git-labels/data/splits/bench78.jsonl
```

Use those split labels as the gold labels:

- train/feedback pool: `feedback300.jsonl`
- GEPA Pareto validation set: `pareto60.jsonl`
- held-out reporting set: `bench78.jsonl`
- gold field: `expected_topics`
- local taxonomy: `examples/profiles/openclaw-routing-topics.json`

The optimizer should not refetch GitHub context during scoring. Each row already
contains the saved `target`, `github_context`, `title`, and `expected_topics`
needed for reproducible evaluation.

## Prompt Boundary

GEPA must not mutate the whole v10 prompt. The current setup uses:

```text
examples/profiles/openclaw-routing.prompt.hbs
examples/profiles/openclaw-routing.schema.json
examples/profiles/openclaw-routing-topics.json
prompt-optimizer/prompts/localpager-openclaw-routing-v10-overlay-scaffold.hbs
prompt-optimizer/prompts/localpager-openclaw-routing-v10-overlay-seed.md
```

The fixed scaffold owns stable contract text:

- task framing
- evalstate label list
- evalstate topic definitions
- output schema and max-3-label contract
- input placeholders
- valid-label constraints

GEPA only mutates the `routing_policy` overlay inserted into that scaffold. The
seed overlay should keep this compact section shape:

```text
Decision Procedure
Cardinality Rules
Boundary Overlays
Suppression Rules
```

Overlay candidates may add decision rules, centrality tests, tie-breakers,
false-positive suppression rules, and false-negative recovery rules. They must
not restate or change the topic definitions, allowed-topic enum, output schema,
or cardinality law.

## Anti-Hacking Rules

The model must not be able to improve by proposing random extra labels.

The optimizer and scorer should make label spam unattractive:

- false positives and false negatives both reduce score
- exact row matches matter
- duplicate predicted labels are penalized
- more than three predicted labels is penalized
- candidates that rely on broad "include it just in case" rules should be
  rejected in review even if one slice improves

Do not add cardinality repair, label filtering, or taxonomy remapping outside
the prompt unless it is part of the production schema/renderer contract.

## Scoring

Use the later row-aware scorer used for the evalstate runs:

```text
score = 0.60 * row_jaccard
      + 0.20 * row_topic_f1
      + 0.20 * row_exact
      - policy_penalties
```

The score is bounded from 0 to 1 after clamping. Policy penalties currently
cover duplicate labels and over-cardinality predictions.

## Runtime Defaults

The default live harness is:

```text
model: nvidia/Qwen3.6-35B-A3B-NVFP4
base_url: http://127.0.0.1:8000/v1
concurrency: 4
thinking: medium
max_tokens: 8192
reflection_lm: CodexReflectionLM
classifier command: scripts/localpager-classifier
```

The harness writes each candidate prompt to a temp file and passes it through
`scripts/localpager-classifier`, which uses the same Localpager profile renderer
and `localpager-agent --final-schema` path as the production classifier.

## Serious Run Shape

A serious run should produce enough evidence to distinguish a real improvement
from a lucky slice:

- evaluate the seed on the relevant split before optimizing
- use all `feedback300` rows unless explicitly doing a smoke test
- validate candidates on `pareto60`
- evaluate the selected best prompt on `bench78`
- target at least 20 candidate proposals for a full attempt
- record all settings, run artifacts, per-row outputs, and score summaries
- generate score graphs and prompt-diff HTML reports
- inspect false positives, false negatives, label counts, and structural errors
  before claiming an improvement

The goal is not merely to run GEPA. The goal is a prompt that survives held-out
evaluation and manual scrutiny.

## Commands

Static summary and smoke checks:

```sh
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli summary
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli evaluate-seed \
  --eval-split pareto \
  --limit 1
```

Live seed check:

```sh
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli evaluate-seed \
  --harness localpager-agent \
  --model nvidia/Qwen3.6-35B-A3B-NVFP4 \
  --base-url http://127.0.0.1:8000/v1 \
  --concurrency 4 \
  --thinking medium \
  --max-tokens 8192 \
  --limit 5
```

Full GEPA attempt:

```sh
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli optimize \
  --max-metric-calls 120 \
  --max-candidate-proposals 20 \
  --reflection-minibatch-size 4 \
  --concurrency 4 \
  --model nvidia/Qwen3.6-35B-A3B-NVFP4 \
  --base-url http://127.0.0.1:8000/v1 \
  --thinking medium \
  --max-tokens 8192
```

Evaluate a saved candidate:

```sh
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli evaluate-candidate \
  --harness localpager-agent \
  --model nvidia/Qwen3.6-35B-A3B-NVFP4 \
  --base-url http://127.0.0.1:8000/v1 \
  --concurrency 4 \
  --thinking medium \
  --max-tokens 8192 \
  --eval-split heldout \
  --routing-policy prompt-optimizer/results/2026-06-17-gepa-evalstate-qwen-overlay-best.routing_policy.md \
  --candidate-name gepa-evalstate-qwen-overlay
```

Reports:

```sh
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli report-run \
  --run-dir prompt-optimizer/out/gepa-evalstate-qwen-overlay-c4-full-YYYYMMDDTHHMMSSZ

PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli plot-run \
  --run-dir prompt-optimizer/out/gepa-evalstate-qwen-overlay-c4-full-YYYYMMDDTHHMMSSZ

PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli plot-prompt-diffs \
  --run-dir prompt-optimizer/out/gepa-evalstate-qwen-overlay-c4-full-YYYYMMDDTHHMMSSZ
```

## Review Checklist

- The best candidate improves held-out score, not only train or Pareto score.
- Precision, recall, FP, FN, exact match, and mean labels are all reported.
- Any score gain is not explained by over-labeling.
- Structural failures are counted and investigated.
- Runtime settings are recorded: model, server, concurrency, thinking, max
  tokens, context window, prompt scaffold, overlay seed, and scorer version.
- The committed prompt remains faithful to the evalstate topic definitions and
  the production output schema.
