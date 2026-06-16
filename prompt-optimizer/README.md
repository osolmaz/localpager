# Localpager Prompt Optimizer

Offline tooling for improving the OpenClaw routing prompt with GEPA. The
optimizer reads the canonical DS4 dataset, starts from the v9.1 seed prompt,
and scores candidates against Localpager routing labels.

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

The summary command expects the local dataset checkout and Shaun feedback set
paths described in `docs/2026-06-12-prompt-optimizer-gepa-plan.md`.

## Scope

The implementation covers:

- loading canonical `ds4.jsonl`
- loading Shaun's `gepa-good-60` row identities
- validating labels against the OpenClaw taxonomy
- normalizing the v9.1 prompt template into Localpager placeholders
- extracting the editable `routing_policy` block
- scoring predictions with a precision-weighted multilabel metric
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
  --model gemma-12b-q4km-reason \
  --max-tokens 4096 \
  --limit 1
```

## Scoring

The optimizer uses an evalstate-shaped scorer with precision-weighted terms:

```text
score = 0.55 * Fβ(β=0.5)
      + 0.20 * topic_micro_f1
      + 0.15 * topic_micro_precision
      + 0.07 * cardinality_closeness
      + 0.03 * exact_match
```

This keeps the score bounded from 0 to 1. It is intentionally stricter about
false positives than false negatives: `Fβ(β=0.5)` and the explicit precision
term make random extra labels hurt, while `cardinality_closeness` penalizes
label-count drift.

Use `--offset` with `--limit` to check held-out slices of Shaun's ordered
60-row set. Saved routing-policy candidates can be scored without another GEPA
run:

```sh
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli evaluate-candidate \
  --harness localpager-agent \
  --model gemma-12b-q4km-reason \
  --max-tokens 4096 \
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
  --max-metric-calls 20 \
  --row-limit 4 \
  --model gemma-12b-q4km-reason \
  --max-tokens 4096
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
  --concurrency 2 \
  --model gemma-12b-q4km-reason \
  --max-tokens 4096
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
