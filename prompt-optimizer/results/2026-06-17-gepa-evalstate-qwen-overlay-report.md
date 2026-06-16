# 2026-06-17 Evalstate Qwen Overlay GEPA Run

This run optimized only the `routing_policy` overlay for
`localpager-openclaw-routing-v10-overlay-scaffold.hbs`. The scaffold, evalstate
taxonomy, schema, and label set were not mutated.

## Run Settings

- Dataset: `evalstate/openclaw-git-labels`
- Train split: `feedback300.jsonl`
- GEPA validation split: `pareto60.jsonl`
- Heldout split: `bench78.jsonl`
- Runtime: `localpager-agent` via `scripts/localpager-classifier`
- Model: `nvidia/Qwen3.6-35B-A3B-NVFP4`
- Base URL: `http://127.0.0.1:8000/v1`
- Thinking: `medium`
- Max output tokens: `8192`
- Concurrency: `4`
- Reflection LM: `CodexReflectionLM`
- GEPA limits: `max_metric_calls=1440`, `max_candidate_proposals=32`
- Actual run: `32` selected iterations, `31` proposal texts, `20` full-val candidates, `1452` metric calls

GEPA exceeded `max_metric_calls` by 12 calls because the final accepted
candidate full evaluation finished in flight rather than stopping mid-candidate.
Iteration 29 selected a parent whose sampled rows were already perfect, so no
new candidate was proposed.

## Results

Pareto validation improved from `0.5742` to `0.6979`.

Heldout comparison uses the same 78-row `bench78` split, Qwen model, v2 labels,
medium thinking, max tokens 8192, and concurrency 4.

```text
v10 seed vs GEPA best (Change = GEPA best - v10 seed)

Metric                         v10 seed       GEPA best      Change
Mean score (higher)            .7686          .7882*          +2%
Precision (higher)             .8889*         .8731           -2%
Recall (higher)                .7778          .8125*          +3%
Micro F1 (higher)              .8296          .8417*          +1%
Exact matches (higher)         46             50*             +4
False positives (lower)        14*            17              +3
False negatives (lower)        32             27*             -5
Over-label total (lower)       4*             6               +2
Structural failures (lower)    0              0                0
Mean predicted labels          1.6154         1.7179          +.1026
```

The improvement is mainly recall: GEPA reduced false negatives by 5 and raised
exact matches by 4, while adding 3 false positives and 2 extra over-label
events. This is not a random-more-labels win: mean predicted labels rose by
only 0.10 labels per row, precision stayed high at `0.8731`, and exact matches
improved.

## Artifacts

- Best prompt: `2026-06-17-gepa-evalstate-qwen-overlay-best.prompt.md`
- Best routing policy overlay: `2026-06-17-gepa-evalstate-qwen-overlay-best.routing_policy.md`
- GEPA summary: `2026-06-17-gepa-evalstate-qwen-overlay-summary.json`
- Heldout GEPA-best summary: `2026-06-17-gepa-evalstate-qwen-overlay-heldout-best.summary.json`
- Heldout v10-seed summary: `2026-06-17-gepa-evalstate-qwen-overlay-heldout-seed.summary.json`

Untracked local HTML reports were also generated in the run directory:

- `score_report.html`
- `prompt-diffs/index.html`

## Scrutiny

- No manual prompt edits were made to the selected best prompt; it was copied
  from GEPA output.
- The optimized component stayed overlay-only. The full scaffold, taxonomy,
  schema, and label enum were not changed by GEPA.
- The best prompt/policy was searched for hard-coded row IDs, GitHub URLs, gold
  labels, and split names; none were found.
- The first heldout-best run had 47 contiguous `localpager-agent` subprocess
  exits after 31 successful rows. Those failed rows produced empty predictions
  and an invalid `0.3035` mean score. Rerunning rows 31-77 cleanly produced zero
  structural failures and mean `0.8043` on that slice. The reported heldout
  result combines successful rows 0-30 from the first run with the clean rerun
  rows 31-77.
- A small localpager-agent diagnostic fix now preserves bounded child stdout on
  nonzero structured-output exits, so future classifier failures include the
  backend/launcher error instead of only `classifier exit 1`.
- Residual risk: the Pareto gain is much larger than the heldout gain, so the
  prompt may be partly fit to `pareto60`. Heldout still improves, but the
  production decision should weigh the modest heldout gain against the slight
  precision and over-label regressions.
