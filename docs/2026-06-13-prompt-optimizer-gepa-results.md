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
| first 12B GEPA candidate | 13-18 | 12B | 0.5833 | fill-in comparison |
| second 12B GEPA candidate | 13-18 | 12B | 0.5417 | held-out gain |
| v9.1 seed | 19-30 | 12B | 0.3946 | larger untouched validation slice |
| first 12B GEPA candidate | 19-30 | 12B | 0.4552 | validation gain; zero structural failures |
| second 12B GEPA candidate | 19-30 | 12B | 0.4134 | smaller gain; more over-labeling |
| v9.1 seed | 31-60 | 12B | 0.4056 | remaining held-out half |
| first 12B GEPA candidate | 31-60 | 12B | 0.4687 | validation gain; zero structural failures |

Combined external 12B scores:

- First 12B GEPA candidate, rows 1-12: `0.5375` vs seed `0.5024`, delta
  `+0.0351`.
- Second 12B GEPA candidate, rows 1-18: `0.5244` vs seed `0.5016`, delta
  `+0.0228`.
- First 12B GEPA candidate, rows 1-30: `0.5137` vs seed `0.4588`, delta
  `+0.0549`; 11 wins, 12 ties, 7 losses; 12 false positives, 26 false
  negatives, 1 over-labeling event, and 0 structural failures.
- Second 12B GEPA candidate, rows 1-30: `0.4800` vs seed `0.4588`, delta
  `+0.0212`; 12 wins, 8 ties, 10 losses; 16 false positives, 17 false
  negatives, 6 over-labeling events, and 2 structural failures.
- The v9.1 seed on rows 1-30 had 15 false positives, 24 false negatives, 2
  over-labeling events, and 2 structural failures.
- First 12B GEPA candidate, rows 1-60: `0.4912` vs seed `0.4322`, delta
  `+0.0590`; 21 wins, 26 ties, 13 losses; 23 false positives, 61 false
  negatives, 1 over-labeling event, and 0 structural failures.
- The v9.1 seed on rows 1-60 had 29 false positives, 60 false negatives, 2
  over-labeling events, and 3 structural failures.

Micro metrics over rows 1-60:

| candidate | precision | recall | F1 |
| --- | ---: | ---: | ---: |
| v9.1 seed | 0.7411 | 0.5804 | 0.6510 |
| first 12B GEPA candidate | 0.7965 | 0.5960 | 0.6818 |

## Current Best Candidate

The tracked assembled prompt is:

```text
prompt-optimizer/results/2026-06-13-gepa-12b-six-best.prompt.md
```

The editable routing-policy block is:

```text
prompt-optimizer/results/2026-06-13-gepa-12b-six-best.routing_policy.md
```

Candidate SHA-256:

```text
f4b161bb9bbaf366f1d4f1841243d73544bbd3c553ca6be5eb2818e757007187
```

The main improvements are explicit rules for:

- `model_serving` vs `telemetry_usage`.
- policy/config/security/MCP conformance cases.
- tighter cardinality guidance and keyword suppression.
- avoiding random extra labels unless they route to a central maintainer bucket.

## Scrutiny

This is progress, not final evidence. The later continuation candidate looked
better inside its GEPA run, but external validation exposed more over-labeling
and two structural failures over the rows 1-30 comparison. The earlier first-12B
candidate is now the better current artifact because it has the larger external
mean-score gain, fewer false positives than v9.1, fewer over-labeling events,
and zero structural failures on the checked rows.

The first candidate still misses one more gold label than v9.1 on rows 1-60, so
it may be slightly conservative. That tradeoff held on rows 31-60: the candidate
kept lower false positives, slightly improved recall, and eliminated structural
failures across the full 60-row set. The remaining risk is qualitative: some
losses are large, especially ACP/session/gateway cases where the candidate drops
a central secondary label. A human should review those losses before replacing a
deployed OpenClaw prompt template.
