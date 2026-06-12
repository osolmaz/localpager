---
title: Prompt Optimizer GEPA Results
author: Bob <dutifulbob@gmail.com>
date: 2026-06-13
---

# Prompt Optimizer GEPA Results

This records the first live GEPA attempts against the OpenClaw classifier prompt
v9.1 using Shaun's ordered `gepa-good-60` set. All gold labels are the canonical
`topics_of_interest` values from `ds4.jsonl`.

## Setup

- Task model: `gemma-12b-q4km-reason`, already loaded locally.
- Fallback/smoke model: `gemma-e4b-reason-test`.
- Reflection model: Codex through `codex exec -`.
- Runtime harness: `scripts/localpager-classifier` -> `localpager-agent`.
- Concurrency: 2 for 12B runs, 1 for the E4B smoke.
- Max output: 1536 tokens after a 512-token 12B smoke failed structurally.
- Metric: weighted multilabel score with false positives costlier than false
  negatives and an extra over-labeling penalty.

## Runs

| candidate | rows | model | mean score | note |
| --- | ---: | --- | ---: | --- |
| v9.1 seed | 1-6 | 12B | 0.5214 | baseline slice |
| E4B smoke candidate | 1-6 | 12B | 0.3607 | rejected; E4B win did not transfer |
| first 12B GEPA candidate | 1-6 | 12B | 0.5833 | train-slice gain |
| v9.1 seed | 7-12 | 12B | 0.4833 | held-out from first 12B run |
| first 12B GEPA candidate | 7-12 | 12B | 0.4917 | small held-out gain |
| second 12B GEPA candidate | 1-12 | 12B | 0.5158 | external re-eval; one structural failure |
| v9.1 seed | 13-18 | 12B | 0.5000 | held-out from second 12B run |
| second 12B GEPA candidate | 13-18 | 12B | 0.5417 | held-out gain |

Combined external 12B scores:

- First 12B GEPA candidate, rows 1-12: `0.5375` vs seed `0.5024`, delta
  `+0.0351`.
- Second 12B GEPA candidate, rows 1-18: `0.5244` vs seed `0.5016`, delta
  `+0.0228`.

## Current Best Candidate

The tracked candidate is:

```text
prompt-optimizer/results/2026-06-13-gepa-12b-twelve-best.routing_policy.md
```

Candidate SHA-256:

```text
6ab4227828618436d7f81662b5cc4993fb5b30557e3e56616801dbec6d2da34a
```

The main improvements are explicit rules for:

- `model_serving` vs `telemetry_usage`.
- `exec_tools`, `sandboxing`, and `approvals`.
- `acp`, `acpx`, and permission-mode labels.
- `config`, `security`, and `mcp_tooling` policy/conformance cases.

## Scrutiny

This is progress, not final evidence. The strongest concern is that the
second-generation candidate scored `0.6101` inside the GEPA run on rows 1-12,
but external re-evaluation on the same slice scored `0.5158` after one
`final_json` structural failure. That gap means the local 12B harness has enough
run-to-run variance that single-pass scores should not be treated as definitive.

The candidate is still worth keeping because it improved the rows 13-18 held-out
slice and did not rely on random extra labels. The next promotion gate should be
a repeated 12B evaluation on a larger held-out slice of the remaining 42 rows,
ideally with per-row win/tie/loss analysis against v9.1 and the first 12B
candidate.
