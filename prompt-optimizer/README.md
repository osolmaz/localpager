# Localpager Prompt Optimizer

Offline tooling for improving the OpenClaw routing prompt with GEPA. The
optimizer uses evalstate's published OpenClaw Git-label splits, the repo-local
OpenClaw benchmark taxonomy, and a v10-based fixed scaffold with an overlay-only
routing-policy candidate.

This package does not run inside the Localpager worker. It is a lab tool whose
production output is a reviewed prompt file.

## Quick Check

From the repository root:

```sh
prompt-optimizer/scripts/check.sh
```

Or run the commands directly:

```sh
PYTHONPATH=prompt-optimizer/src python3 -m unittest discover -s prompt-optimizer/tests
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli summary
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli evaluate-seed --limit 1
```

The summary command expects the local dataset checkout and paths described in
`docs/2026-06-12-prompt-optimizer-gepa-plan.md`.

## Scope

The implementation covers:

- loading evalstate's `feedback300`, `pareto60`, and `bench78` split files
- validating labels against the OpenClaw taxonomy
- normalizing prompt templates into Localpager placeholders
- supporting the v10 scaffold plus overlay-only GEPA candidate boundary
- scoring predictions with the row-aware multilabel metric used for the
  evalstate runs
- evaluating candidates through a mockable GEPA adapter shape
- invoking the production `scripts/localpager-classifier` wrapper through a
  tested subprocess harness
- wiring `gepa.optimize` behind an explicit CLI command that writes run artifacts
- wrapping `codex exec -` as a GEPA reflection language model
- re-evaluating saved routing-policy candidates against explicit dataset slices
- continuing a GEPA run from a saved routing-policy candidate
- writing score and prompt-diff HTML reports for completed GEPA runs

The `evaluate-seed` command defaults to a no-model static harness. To run a
real one-row classifier smoke through the production wrapper, pass:

```sh
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli evaluate-seed \
  --harness localpager-agent \
  --model nvidia/Qwen3.6-35B-A3B-NVFP4 \
  --base-url http://127.0.0.1:8000/v1 \
  --concurrency 4 \
  --max-tokens 8192 \
  --limit 1
```

Default inputs:

- train/feedback pool:
  `/home/bob/repos/openclaw-git-labels/data/splits/feedback300.jsonl`
- GEPA Pareto validation set:
  `/home/bob/repos/openclaw-git-labels/data/splits/pareto60.jsonl`
- held-out reporting set:
  `/home/bob/repos/openclaw-git-labels/data/splits/bench78.jsonl`
- taxonomy: `examples/profiles/openclaw-routing-topics.json`
- fixed scaffold source:
  `examples/profiles/openclaw-routing.prompt.hbs`
- GEPA scaffold:
  `prompt-optimizer/prompts/localpager-openclaw-routing-v10-overlay-scaffold.hbs`
- seed overlay:
  `prompt-optimizer/prompts/localpager-openclaw-routing-v10-overlay-seed.md`
- live model:
  `nvidia/Qwen3.6-35B-A3B-NVFP4` on `http://127.0.0.1:8000/v1`
- live concurrency: `4`
- thinking: `medium`
- GEPA max output tokens: `8192`

Static smoke checks do not make model calls:

```sh
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli summary
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli evaluate-seed \
  --eval-split pareto \
  --limit 1
```

GEPA should not mutate the whole v10 prompt. The v10-based scaffold keeps the
task contract fixed: label list, topic definitions, schema, input format,
valid-label constraints, and max-3-label output constraints. GEPA only mutates
the routing-policy overlay inserted into that scaffold. The overlay should keep
the compact section shape:

```text
Decision Procedure
Cardinality Rules
Boundary Overlays
Suppression Rules
```

Overlay candidates may add decision rules, centrality tests, boundary
tie-breakers, and suppression rules. They must not restate topic definitions,
the allowed-topic enum, cue-word lists, the output schema, or the cardinality
law.

## Scoring

The optimizer uses the later row-aware scorer from the evalstate runs:

```text
score = 0.60 * row_jaccard
      + 0.20 * row_topic_f1
      + 0.20 * row_exact
      - policy_penalties
```

This keeps the score bounded from 0 to 1. False positives and false negatives
both reduce row Jaccard, row topic F1, and exact row match. Current policy
penalties cover duplicate predicted labels and more than 3 predicted labels.

Use `--eval-split`, `--offset`, and `--limit` to check explicit slices. Saved
routing-policy candidates can be scored without another GEPA run:

```sh
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli evaluate-candidate \
  --harness localpager-agent \
  --model nvidia/Qwen3.6-35B-A3B-NVFP4 \
  --base-url http://127.0.0.1:8000/v1 \
  --concurrency 4 \
  --max-tokens 8192 \
  --eval-split heldout \
  --routing-policy prompt-optimizer/results/2026-06-17-gepa-evalstate-qwen-overlay-best.routing_policy.md \
  --candidate-name gepa-evalstate-qwen-overlay \
  --limit 20
```

GEPA optimization is explicit because a live run can take a long time:

```sh
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli optimize \
  --max-metric-calls 20 \
  --row-limit 4 \
  --model nvidia/Qwen3.6-35B-A3B-NVFP4 \
  --base-url http://127.0.0.1:8000/v1 \
  --concurrency 4 \
  --thinking medium \
  --max-tokens 8192
```

To continue from a saved candidate instead of the seed overlay, pass
`--seed-routing-policy`:

```sh
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli optimize \
  --seed-routing-policy prompt-optimizer/results/2026-06-17-gepa-evalstate-qwen-overlay-best.routing_policy.md \
  --max-metric-calls 30 \
  --max-candidate-proposals 8 \
  --row-limit 12 \
  --reflection-minibatch-size 4 \
  --concurrency 4 \
  --model nvidia/Qwen3.6-35B-A3B-NVFP4 \
  --base-url http://127.0.0.1:8000/v1 \
  --thinking medium \
  --max-tokens 8192
```

Summarize a live or completed GEPA run without making model calls:

```sh
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli report-run \
  --run-dir prompt-optimizer/out/gepa-evalstate-qwen-overlay-c4-full-YYYYMMDDTHHMMSSZ
```

Write a self-contained score report with iteration and candidate-score charts:

```sh
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli plot-run \
  --run-dir prompt-optimizer/out/gepa-evalstate-qwen-overlay-c4-full-YYYYMMDDTHHMMSSZ
```

Write a self-contained prompt diff report for every saved candidate in a GEPA
run. The page has left/right dropdowns for comparing either the editable
routing policy or the assembled full prompt:

```sh
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli plot-prompt-diffs \
  --run-dir prompt-optimizer/out/gepa-evalstate-qwen-overlay-c4-full-YYYYMMDDTHHMMSSZ
```

Summarize a saved evaluation JSON without making model calls:

```sh
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli summarize-evaluation \
  --evaluation prompt-optimizer/out/validation-evalstate-qwen-overlay-YYYYMMDDTHHMMSSZ/heldout.json
```
