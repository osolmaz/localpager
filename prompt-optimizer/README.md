# Localpager Prompt Optimizer

Offline tooling for improving the OpenClaw routing prompt with GEPA. The
optimizer can read either the original DS4/Shaun 60-row setup or evalstate's
published OpenClaw Git-label splits. DS4 mode starts from the v9.1 seed prompt.
Evalstate mode uses the v2 topic taxonomy with a v10-based fixed scaffold and
an overlay-only routing-policy candidate.

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
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli summary --dataset evalstate
```

The summary commands expect the local dataset checkouts and paths described in
`docs/2026-06-12-prompt-optimizer-gepa-plan.md`.

## Scope

The implementation covers:

- loading canonical `ds4.jsonl`
- loading Shaun's `gepa-good-60` row identities
- loading evalstate's `feedback300`, `pareto60`, and `bench78` split files
- validating labels against the OpenClaw taxonomy
- normalizing prompt templates into Localpager placeholders
- extracting the editable `routing_policy` block
- planning evalstate's v10 scaffold plus overlay-only GEPA candidate boundary
- scoring predictions with Shaun/evalstate's row-aware multilabel metric
- evaluating candidates through a mockable GEPA adapter shape
- invoking the production `scripts/localpager-classifier` wrapper through a
  tested subprocess harness
- wiring `gepa.optimize` behind an explicit CLI command that writes run artifacts
- wrapping `codex exec -` as a GEPA reflection language model
- re-evaluating saved routing-policy candidates against explicit dataset slices
- continuing a GEPA run from a saved routing-policy candidate

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

Evalstate mode uses these defaults:

- train/feedback pool:
  `/home/bob/repos/openclaw-git-labels/data/splits/feedback300.jsonl`
- GEPA Pareto validation set:
  `/home/bob/repos/openclaw-git-labels/data/splits/pareto60.jsonl`
- held-out reporting set:
  `/home/bob/repos/openclaw-git-labels/data/splits/bench78.jsonl`
- taxonomy: `examples/profiles/openclaw-routing-topics.v2.json`
- fixed scaffold source:
  `/home/bob/oc/openclaw-classification-dataset/prompts/localpager-openclaw-routing-v10-production.hbs`
- planned GEPA scaffold:
  `/home/bob/oc/openclaw-classification-dataset/prompts/localpager-openclaw-routing-v10-overlay-scaffold.hbs`
- planned seed overlay:
  `/home/bob/oc/openclaw-classification-dataset/prompts/localpager-openclaw-routing-v10-overlay-seed.md`
- live model:
  `nvidia/Qwen3.6-35B-A3B-NVFP4` on `http://127.0.0.1:8000/v1`
- live concurrency: `4`
- thinking: `medium`
- GEPA max output tokens: `8192`

Static smoke checks do not make model calls:

```sh
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli summary --dataset evalstate
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli evaluate-seed \
  --dataset evalstate \
  --eval-split pareto \
  --limit 1
```

Evalstate GEPA should not mutate the whole v10 prompt. The v10-based scaffold
keeps the task contract fixed: label list, topic definitions, schema, input
format, valid-label constraints, and max-3-label output constraints. GEPA only
mutates the routing-policy overlay inserted into that scaffold. The overlay
should keep Shaun/evalstate's compact section shape:

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

The optimizer uses Shaun/evalstate's later row-aware scorer:

```text
score = 0.60 * row_jaccard
      + 0.20 * row_topic_f1
      + 0.20 * row_exact
      - policy_penalties
```

This keeps the score bounded from 0 to 1. The scorer is balanced rather than
recall-leaning: false positives and false negatives both reduce row Jaccard,
row topic F1, and exact row match. Current policy penalties cover duplicate
predicted labels and more than 3 predicted labels.

Use `--offset` with `--limit` to check held-out slices of Shaun's ordered
60-row set. Saved routing-policy candidates can be scored without another GEPA
run:

```sh
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli evaluate-candidate \
  --harness localpager-agent \
  --model nvidia/Qwen3.6-35B-A3B-NVFP4 \
  --base-url http://127.0.0.1:8000/v1 \
  --concurrency 4 \
  --max-tokens 8192 \
  --routing-policy prompt-optimizer/results/2026-06-13-gepa-12b-six-best.routing_policy.md \
  --candidate-name gepa-12b-six-best \
  --limit 6 \
  --offset 12
```

The current best reviewed artifact from the 60-row validation sequence is:

```text
prompt-optimizer/results/2026-06-14-gepa-12b-prop20-cardinality-repair.prompt.md
```

Its editable routing-policy block is:

```text
prompt-optimizer/results/2026-06-14-gepa-12b-prop20-cardinality-repair.routing_policy.md
```

GEPA optimization is explicit because a live run can take a long time:

```sh
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli optimize \
  --dataset evalstate \
  --max-metric-calls 20 \
  --row-limit 4 \
  --model nvidia/Qwen3.6-35B-A3B-NVFP4 \
  --base-url http://127.0.0.1:8000/v1 \
  --concurrency 4 \
  --thinking medium \
  --max-tokens 8192
```

To continue from a saved candidate instead of the v9.1 seed prompt, pass
`--seed-routing-policy`:

```sh
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli optimize \
  --seed-routing-policy prompt-optimizer/results/2026-06-13-gepa-12b-six-best.routing_policy.md \
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
  --run-dir prompt-optimizer/out/gepa-12b-row30-prop16-from-proper-20260613T172903Z
```

Write a self-contained score report with iteration and candidate-score charts:

```sh
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli plot-run \
  --run-dir prompt-optimizer/out/gepa-12b-row30-prop16-from-proper-20260613T172903Z
```

Summarize a saved 60-row validation JSON without making model calls:

```sh
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli summarize-evaluation \
  --evaluation prompt-optimizer/out/validation-12b-row30-prop16-best-YYYYMMDDTHHMMSSZ/gepa-12b-row30-prop16-best-limit60.json
```
