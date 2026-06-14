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

## 2026-06-14 Proper GEPA Continuation

The completed continuation run is:

```text
prompt-optimizer/out/gepa-12b-row30-prop20-continuation-20260614T021448Z
```

It was seeded from the previous proper best routing policy and run with 12B,
row limit 30, reflection minibatch 4, concurrency 2, max metric calls 720, and
`max_candidate_proposals=20`. The higher proposal cap was intentional: GEPA's
stopper count did not map one-to-one to the logged `proposal_attempts`, and two
earlier continuations stopped at 15 logged attempts.

Run gates:

| gate | target | observed | result |
| --- | ---: | ---: | --- |
| proposal attempts | >= 16 | 20 | pass |
| accepted full-eval candidates | >= 6 | 18 | pass |
| distinct candidates including seed | >= 8 | 19 | pass |
| metric calls | >= 480 | 730 | pass |
| row limit | >= 30 if runtime allows | 30 | pass |
| 12B concurrency | 2 | 2 | pass |
| OOM / retry storm | none | none observed | pass |

The best internal 30-row validation score was `0.7404` at candidate index 2.
The tracked artifacts are:

```text
prompt-optimizer/results/2026-06-14-gepa-12b-prop20-best.prompt.md
prompt-optimizer/results/2026-06-14-gepa-12b-prop20-best.routing_policy.md
```

Routing-policy SHA-256:

```text
5ff17ce59da5ff6c98c7241f10a12e7ebb908b415c51a6aad631ad9d83a686e5
```

External 60-row validation artifact:

```text
prompt-optimizer/out/validation-12b-row30-prop16-best-20260614T081931Z/gepa-12b-row30-prop16-best-limit60.json
```

External 60-row metrics:

| metric | target | observed | result |
| --- | ---: | ---: | --- |
| mean weighted score | >= 0.5400 | 0.5936 | pass |
| score delta vs v9.1 | >= +0.1000 | +0.1613 | pass |
| micro-F1 | >= 0.7000 | 0.7566 | pass |
| precision | >= 0.8000 | 0.8707 | pass |
| recall | >= 0.6100 | 0.6689 | pass |
| false positives | <= 20 | 15 | pass |
| false negatives | <= 58 | 50 | pass |
| over-label events | 0 or 1 | 2 | fail |
| structural failures | 0 | 4 | fail |
| exact matches | >= 15 | 24 | pass |
| mean predicted labels delta vs v9.1 | <= +0.10 | +0.067 | pass |

Comparison against earlier 60-row references:

| candidate | mean score | precision | recall | F1 | FP | FN | over-label events | structural failures | exact |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| v9.1 seed | 0.4322 | 0.7411 | 0.5804 | 0.6510 | 29 | 60 | 2 | 3 | 10 |
| first 12B GEPA candidate | 0.4912 | 0.7965 | 0.5960 | 0.6818 | 23 | 61 | 1 | 0 | 12 |
| previous proper best | 0.5355 | 0.7778 | 0.6712 | 0.7206 | 28 | 48 | 5 | 2 | 21 |
| 2026-06-14 prop20 best | 0.5936 | 0.8707 | 0.6689 | 0.7566 | 15 | 50 | 2 | 4 | 24 |

Scrutiny:

- This is the strongest aggregate result so far and clearly beats v9.1,
  GEPA-six, and the previous proper best on mean score, precision, F1, false
  positives, and exact matches.
- It is not a clean success by the plan because structural failures regressed
  to 4 and over-label events are 2 rather than 0 or 1.
- The mean predicted labels per row is `1.9333`, up from v9.1's `1.8667` on the
  same 60-row split artifacts. The +0.067 increase stays below the anti-hacking
  cardinality guardrail.
- The structural failures are all `final_json was not called`; transcript
  inspection showed the model exhausting the 1536-token output budget in
  reasoning before tool-calling on at least one failing row.
- A manual tail-only `final_json` guard was tested on the two small windows that
  contain the four observed structural failures. It fixed some rows but moved a
  structural miss to another row and worsened the row-47/50 window score, so it
  was not promoted.

## Current Best Candidate

The tracked assembled prompt is:

```text
prompt-optimizer/results/2026-06-14-gepa-12b-prop20-best.prompt.md
```

The editable routing-policy block is:

```text
prompt-optimizer/results/2026-06-14-gepa-12b-prop20-best.routing_policy.md
```

Candidate SHA-256:

```text
5ff17ce59da5ff6c98c7241f10a12e7ebb908b415c51a6aad631ad9d83a686e5
```

The main improvements are explicit rules for:

- `model_serving` vs `telemetry_usage`.
- policy/config/security/MCP conformance cases.
- tighter cardinality guidance and keyword suppression.
- avoiding random extra labels unless they route to a central maintainer bucket.

## Scrutiny

This is the current best aggregate artifact, but not a deployment-ready win by
the plan's strict gates. It materially improves the classifier's score and false
positive profile, while keeping cardinality under control, but it still has
format-following failures under the 1536-token classifier budget.

The next useful experiment should target structural reliability directly rather
than chasing more topic rules: either shorten the routing policy, alter the
runtime wrapper to suppress long reasoning more aggressively, or run a GEPA
variant whose feedback set includes the observed final_json failures and whose
scoring function makes structural failure a hard loss.
