# Localpager Prompt Optimizer

Experimental offline tooling for improving the OpenClaw routing prompt with
GEPA. The optimizer reads the canonical DS4 dataset, starts from the v9.1 seed
prompt, and scores candidates against Localpager routing labels.

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

## Current Scope

The first implementation slice covers:

- loading canonical `ds4.jsonl`
- loading Shaun's `gepa-good-60` row identities
- validating labels against the OpenClaw taxonomy
- normalizing the v9.1 prompt template into Localpager placeholders
- extracting the editable `routing_policy` block
- scoring predictions with a false-positive-heavy multilabel metric
- evaluating candidates through a mockable GEPA adapter shape
- invoking the production `scripts/localpager-classifier` wrapper through a
  tested subprocess harness
- wiring `gepa.optimize` behind an explicit CLI command that writes run artifacts
- wrapping `codex exec -` as a GEPA reflection language model

The `evaluate-seed` command defaults to a no-model static harness. To run a
real one-row classifier smoke through the production wrapper, pass:

```sh
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli evaluate-seed \
  --harness localpager-agent \
  --model gemma-12b-q4km-reason \
  --limit 1
```

GEPA optimization is explicit because a live run can take a long time:

```sh
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli optimize \
  --max-metric-calls 20 \
  --row-limit 4 \
  --model gemma-12b-q4km-reason
```
