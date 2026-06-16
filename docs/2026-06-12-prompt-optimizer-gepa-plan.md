---
title: Prompt Optimizer (GEPA) — Plan
author: Bob <dutifulbob@gmail.com>
date: 2026-06-12
---

# Prompt Optimizer (GEPA) — Plan

This records the design used by the `prompt-optimizer/` tool that uses GEPA to
improve the Localpager classifier prompt. The implementation lives under
`prompt-optimizer/` and the reviewed prompt artifacts live under
`prompt-optimizer/results/`.

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

- Own `pyproject.toml` with a GEPA dependency plus an OpenAI-compatible client;
  virtualenv is gitignored.
- Touches neither `go.mod` nor `localpager-agent/package.json`.
- Excluded from `scripts/check.sh` and the Go/slophammer gates. Optional own
  gates (`ruff`, `pytest`) may be added later; none are required while it is
  experimental.
- Talks to Localpager only through the stable seam (the classifier command) and
  the shared files. No imports across the language boundary.

## What GEPA optimizes (the candidate)

DS4 compatibility mode seeds from the OpenClaw classification dataset prompt
`prompts/localpager-openclaw-routing-v9.1-monologue-cap.hbs` in
`dutifuldev/openclaw-classification-dataset` at revision
`8d1088276425ca72a5313c18cde4adef20ffe194`.

Evalstate v2 mode must use an overlay-only GEPA candidate. The v10 prompt is
not optimized directly. Instead, split it into:

- a fixed v10-based scaffold containing only stable task contract text:
  task framing, evalstate v2 label list, evalstate topic definitions, output
  schema, input placeholders, valid-label constraints, and max-3-label output
  constraints;
- one mutable routing-policy overlay placeholder, rendered into the scaffold
  at runtime.

The initial evalstate GEPA candidate should be a separate overlay seed file,
not the full v10 prompt. Use Shaun/evalstate's overlay shape:

```text
Decision Procedure
Cardinality Rules
Boundary Overlays
Suppression Rules
```

The overlay candidate may contain compact decision rules, tie-breakers,
centrality tests, false-positive suppression rules, and false-negative recovery
rules. It must not restate topic definitions, the allowed-topic enum, cue-word
lists, the deliverable test, the output schema, or the cardinality law. Those
belong to the fixed scaffold. If current v10 text mixes heuristic policy into
the fixed body, move that policy text into the overlay seed or delete it if it
duplicates evalstate's fixed definitions.

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
allowed to, and can never break the placeholder, label, or schema contract. The
template is assembled as: frozen scaffold + the current `routing_policy`
overlay + fixed target/context suffix.

Implementation target:

- Add a GEPA-specific scaffold file, for example
  `prompts/localpager-openclaw-routing-v10-overlay-scaffold.hbs`.
- Add a separate seed overlay file, for example
  `prompts/localpager-openclaw-routing-v10-overlay-seed.md`.
- Make evalstate mode load the scaffold as the prompt wrapper and the seed
  overlay as `seed_candidate["routing_policy"]`.
- Add tests proving that evalstate candidates do not include topic definitions,
  schema text, or the label enum, and that rendering reconstructs a full
  Localpager prompt with the overlay inserted.

## Evaluation harness (the transfer rule)

Candidates are scored through the same path production uses, so a gain in the
lab transfers to deployment:

- Render the candidate with the production renderer
  (`scripts/localpager-render-profile.mjs`) so placeholders and the
  taxonomy-derived schema enum match exactly.
- Run it through `localpager-classifier` → `localpager-agent` → Pi → the
  Qwen vLLM endpoint → `final_json`.

The runtime classifier harness is `localpager-agent`. The Python
`prompt-optimizer` harness is only a driver: it assembles candidate prompt files,
selects dataset rows, invokes `localpager-classifier` / `localpager-agent`, and
parses `final_json`. It must not replace or bypass `localpager-agent` for model
execution.

To keep many rollouts tractable:

- Pre-fetch and cache each dataset row's GitHub context once (pass
  `--github-context-file`), so rollouts never re-hit GitHub.
- Keep the model server warm.
- Run Qwen through vLLM at concurrency 4, matching the c4 serving profile's
  `--max-num-seqs 4`.
- Use `--thinking medium` for saved benchmark settings and future local
  classifier benchmark runs unless the experiment is explicitly marked as a
  different thinking-level comparison.
- Use `--max-tokens 8192` for Qwen classifier rollouts. This is the current
  recommended cap for avoiding `final_json` structural failures on long
  reasoning rows while keeping concurrency at 4.
- If Qwen rollout speed blocks iteration, use `gemma-e4b-reason-test` for
  smoke/debug runs and explicitly marked exploratory optimizer iterations.
  E4B scores are not transfer evidence: any candidate selected or filtered with
  E4B must be re-evaluated on Qwen before promotion, validation, or final
  reporting.
- Add the one production-side seam below so rows stream through a warm process
  instead of spawning Pi per row.

## Scoring and ASI

- Optimization metric (μ): Shaun/evalstate's later row-aware multilabel routing
  score against the gold labels:

  ```text
  score = 0.60 * row_jaccard
        + 0.20 * row_topic_f1
        + 0.20 * row_exact
        - policy_penalties
  ```

  `row_jaccard` is `TP / (TP + FP + FN)` for the row's topic set.
  `row_topic_f1` is the row-level topic F1. `row_exact` is 1 only when the
  predicted set exactly matches the gold set. `policy_penalties` apply when a
  candidate breaks the label policy, currently duplicate predicted labels and
  more than 3 predicted labels. This scorer is balanced rather than
  recall-leaning: false positives and false negatives both reduce Jaccard, F1,
  and exact match. Report topic micro-F1, per-topic precision, and per-topic
  recall alongside this objective, but select candidates by the weighted score.
- The metric must be game-resistant: proposing random extra labels must never
  help a candidate. A predicted label only improves the score when it is present
  in the row's gold label set. Every extra allowed label is a false positive,
  reduces row Jaccard and row F1, prevents exact match, and should be surfaced in
  ASI feedback as label spam. Invalid labels are schema failures, not partial
  credit.
- Feedback / ASI (μ_f): the per-row, per-topic mistakes turned into short
  natural-language notes, e.g. "item 412: predicted `config`; gold has none — an
  option was added but config behavior is not the subject." This is the same
  signal that drove the manual prompt gains, produced automatically.

## Models

- Task / evaluator LM: `nvidia/Qwen3.6-35B-A3B-NVFP4` served through vLLM at
  `http://127.0.0.1:8000/v1`, using concurrency 4. Non-negotiable for transfer;
  the prompt is tuned to that served model's quirks.
- Fast fallback LM: `gemma-e4b-reason-test`, for smoke/debug runs or
  budget-limited exploration only. Log fallback use clearly and never compare E4B
  scores directly against Qwen scores.
- Reflection / proposer LM: Codex. Prefer a direct non-interactive Codex CLI
  bridge (`codex exec -`) wrapped as GEPA's `reflection_lm`; pass the reflection
  prompt on stdin and capture the final response from stdout or
  `--output-last-message`. Use `--output-schema` when the reflection response
  needs machine-readable fields. A Pi-backed Codex invocation is an acceptable
  fallback only if the direct CLI bridge does not fit GEPA's adapter contract.
  Keep Codex auth outside the repo, using the operator's existing Codex CLI
  authentication/config.

## Data and reproducibility

- Three-way split of the curated dataset: a feedback/minibatch pool, a
  `D_pareto` validation set for candidate selection, and a held-out test set the
  optimizer never sees (report final transfer on it).
- Treat `ds4.jsonl` as the authoritative golden set. The gold labels for scoring
  are exactly the `topics_of_interest` values recorded on each row in
  `ds4.jsonl`; the optimizer must not reinterpret, relabel, broaden, or
  adjudicate them during a run. Loader normalization is limited to deterministic
  parsing, de-duplication, ordering for stable output, and validation that labels
  are in the configured taxonomy.
- Pin sampling: temperature 0, fixed seed, matching the production worker.
- Log the dataset revision, seed prompt path, taxonomy path, loaded model
  identifier, concurrency, every candidate, its scores, and the chosen prompt to
  a run directory so a run is reproducible from saved inputs.

### Evalstate v2 split setup

For the next GEPA run, use evalstate's published OpenClaw Git-label dataset
with the v2 topic taxonomy and a v10-based overlay scaffold:

- Feedback/minibatch pool: `feedback300.jsonl`.
- Pareto validation set for GEPA candidate selection: `pareto60.jsonl`.
- Held-out reporting set that GEPA must not optimize against: `bench78.jsonl`.
- Local split checkout:
  `/home/bob/repos/openclaw-git-labels/data/splits/`.
- Local taxonomy:
  `examples/profiles/openclaw-routing-topics.v2.json`.
- Fixed scaffold source:
  `/home/bob/oc/openclaw-classification-dataset/prompts/localpager-openclaw-routing-v10-production.hbs`.
- Planned GEPA scaffold artifact:
  `/home/bob/oc/openclaw-classification-dataset/prompts/localpager-openclaw-routing-v10-overlay-scaffold.hbs`.
- Planned seed overlay artifact:
  `/home/bob/oc/openclaw-classification-dataset/prompts/localpager-openclaw-routing-v10-overlay-seed.md`.

The production v10 prompt is the source for the fixed contract, not the GEPA
candidate. The optimizer must only propose replacements for the overlay seed.

The evalstate rows already include the saved `target`, `github_context`, and
`expected_topics`. For this dataset, the gold labels are exactly
`expected_topics` from the split rows, validated against the v2 taxonomy. The
optimizer must pass the saved `github_context` into `localpager-agent`; it must
not refetch GitHub context or silently fall back to DS4 row fields.

The GEPA CLI mode for this setup is `--dataset evalstate`. It trains on
`feedback300`, uses `pareto60` as GEPA's `valset`, and keeps `bench78` available
for explicit held-out evaluation/reporting after the optimizer run.

Static preflight commands:

```sh
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli summary --dataset evalstate
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli evaluate-seed \
  --dataset evalstate \
  --eval-split pareto \
  --limit 1
```

Run command shape, when ready:

```sh
PYTHONPATH=prompt-optimizer/src python3 -m prompt_optimizer.cli optimize \
  --dataset evalstate \
  --max-metric-calls 480 \
  --max-candidate-proposals 16 \
  --reflection-minibatch-size 4 \
  --concurrency 4 \
  --model nvidia/Qwen3.6-35B-A3B-NVFP4 \
  --base-url http://127.0.0.1:8000/v1 \
  --thinking medium \
  --max-tokens 8192
```

Do not start this run until the code and static smoke checks have passed.

### Initial feedback/minibatch pool

Use Shaun's 60-row `gepa-good-60` set as the initial GEPA feedback/minibatch
pool. The row selection comes from
`/home/bob/scratch/shaun-openclaw-data-rows/gepa-good-60.rows.jsonl`, and the
hydrated local input is
`/home/bob/scratch/shaun-openclaw-data-rows/gepa-good-60.hydrated.ds4-input.jsonl`.

This set is for optimizer feedback, not final validation. It contains 32
stratified rows, 19 confusion rows, and 9 random rows. Use only the row
identities/order from Shaun's manifest. Do not score against the manifest's
`teacher_topics` or `expected_topics`; those are broader teacher labels. Score
against the canonical `ds4.jsonl` `topics_of_interest` labels below.

| # | target | gold `topics_of_interest` from `ds4.jsonl` |
| ---: | --- | --- |
| 1 | PR #48940 | `acp`, `gateway`, `agent_runtime` |
| 2 | PR #80783 | `mcp_tooling`, `config`, `security` |
| 3 | PR #42027 | `exec_tools`, `browser_automation`, `cron_automation` |
| 4 | PR #77748 | `codex`, `chat_integrations` |
| 5 | issue #79897 | `model_serving` |
| 6 | issue #40332 | `acp`, `approvals`, `acpx` |
| 7 | PR #63007 | `gateway`, `sessions` |
| 8 | PR #80255 | `memory`, `reliability` |
| 9 | PR #84670 | `gateway`, `api_surface`, `ui_tui` |
| 10 | PR #46552 | `queueing`, `docs` |
| 11 | PR #62428 | `exec_tools`, `sandboxing`, `approvals` |
| 12 | issue #82507 | `acpx`, `codex`, `skills_plugins` |
| 13 | PR #80479 | `self_hosted_inference`, `memory` |
| 14 | issue #90146 | `local_model_providers`, `reliability` |
| 15 | PR #51849 | `docs` |
| 16 | PR #68725 | `open_weight_models`, `local_model_providers` |
| 17 | issue #84297 | `notifications`, `chat_integrations` |
| 18 | PR #77827 | `model_serving`, `local_models` |
| 19 | PR #81957 | `security` |
| 20 | issue #39248 | `coding_agents`, `sandboxing`, `agent_runtime` |
| 21 | PR #47083 | `sessions`, `telemetry_usage` |
| 22 | PR #70882 | `mcp_tooling`, `tool_calling` |
| 23 | PR #63826 | `security`, `hooks`, `skills_plugins` |
| 24 | issue #81249 | `local_models`, `self_hosted_inference` |
| 25 | issue #70529 | `browser_automation`, `packaging_deployment` |
| 26 | issue #87277 | `local_model_providers`, `model_serving` |
| 27 | issue #64199 | `acp`, `sessions` |
| 28 | PR #84752 | `reliability`, `auth_identity`, `sessions` |
| 29 | issue #84583 | `cron_automation`, `sessions`, `reliability` |
| 30 | issue #67244 | `acpx`, `acp` |
| 31 | issue #71216 | `config`, `sandboxing`, `gateway` |
| 32 | issue #84477 | `sessions`, `agent_runtime`, `reliability` |
| 33 | PR #65242 | `acp`, `coding_agents`, `reliability` |
| 34 | issue #73910 | `codex`, `acp`, `acpx`, `auth_identity` |
| 35 | PR #80008 | `acp`, `coding_agents` |
| 36 | PR #43765 | `reliability`, `exec_tools`, `cron_automation` |
| 37 | issue #60979 | `acp`, `chat_integrations`, `sessions` |
| 38 | issue #83863 | `acp`, `codex`, `agent_runtime` |
| 39 | issue #84715 | `codex`, `packaging_deployment` |
| 40 | issue #84757 | `sessions`, `chat_integrations`, `reliability` |
| 41 | PR #56442 | `acp`, `sessions`, `agent_runtime` |
| 42 | issue #78528 | `security`, `exec_tools`, `skills_plugins` |
| 43 | issue #84789 | `memory`, `sessions` |
| 44 | PR #84763 | `acpx`, `acp`, `security` |
| 45 | PR #65364 | `auth_identity`, `api_surface` |
| 46 | PR #52747 | `acp`, `sessions`, `reliability` |
| 47 | issue #10467 | `queueing`, `sessions`, `coding_agents` |
| 48 | PR #43246 | `tool_calling`, `security` |
| 49 | issue #59878 | `sessions`, `reliability` |
| 50 | issue #51667 | `model_serving`, `security`, `config` |
| 51 | issue #44202 | `local_models`, `memory`, `self_hosted_inference` |
| 52 | issue #48580 | `acpx`, `codex`, `sessions` |
| 53 | issue #74305 | `acpx`, `codex` |
| 54 | PR #45393 | `tool_calling`, `coding_agents`, `reliability` |
| 55 | issue #84771 | `reliability`, `sessions` |
| 56 | PR #44379 | `coding_agents`, `memory`, `hooks`, `reliability` |
| 57 | issue #84746 | `reliability`, `sessions` |
| 58 | issue #68187 | `mcp_tooling`, `sessions`, `gateway` |
| 59 | issue #52249 | `acp`, `sessions`, `reliability` |
| 60 | PR #69256 | `cron_automation`, `sessions`, `reliability` |

## Package layout

```
prompt-optimizer/
  pyproject.toml                 # depends on gepa + an OpenAI-compatible client
  README.md                      # what it does, how to run
  src/prompt_optimizer/
    harness.py                   # Python driver around localpager-agent, parse final_json
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
  prompt from frozen scaffold + `candidate["routing_policy"]`, hand it to the
  Python driver, run classification through `localpager-agent`, parse
  `final_json`, score vs DS4. Returns outputs, scores, and (when
  `capture_traces`) the wrong-topic details.
- `make_reflective_dataset(candidate, eval_batch, components_to_update)` → turn
  the captured mistakes into the ASI notes above, keyed by `routing_policy`.

`run.py` calls `gepa.optimize(seed_candidate, trainset, valset, adapter,
reflection_lm, max_metric_calls=...)` and writes the winning `routing_policy`
plus a report to `out/`.

The first reflection bridge should be `CodexReflectionLM`, a thin subprocess
wrapper around `codex exec -`. It must run with no repository edits, log the
exact `codex --version`, selected model/profile/config knobs, command argv, and
reflection prompt digest for reproducibility, and fail closed if Codex returns
non-JSON when a schema is required.

## The one production change

Add a batch / RPC classify mode to `localpager-agent`, the runtime classifier
harness, so the adapter can stream many rows through one warm agent process
instead of spawning Pi per row. This is the only change outside
`prompt-optimizer/`, it keeps evaluation faithful, and it is independently
useful.

## Milestones

1. Scaffold `prompt-optimizer/` (pyproject, package skeleton, README).
2. `dataset.py`: load DS4 rows or evalstate split rows, cache prompt inputs and
   saved GitHub contexts, three-way split.
3. `harness.py` + `metric.py`: classify one row through `localpager-agent` and
   score it vs DS4 using the weighted FP/FN/over-labeling objective, with a mock
   model path for fast tests.
4. `reflection.py`: implement and smoke-test `CodexReflectionLM` over
   `codex exec -`.
5. `adapter.py`: implement `evaluate` and `make_reflective_dataset`.
6. `run.py`: wire `gepa.optimize`; do a tiny smoke run on a handful of rows.
7. Add the batch-classify seam to `localpager-agent`; switch the Python driver
   to it.
8. Full run; review the winning prompt; commit it as the new
   `classifier.prompt_template`.

## 8-hour progress target

Within the first 8 hours of implementation, "good progress" means a committed,
tested first slice exists even if no full GEPA optimization run has completed:

- `prompt-optimizer/` is scaffolded with `pyproject.toml`, package modules,
  README, and local test command.
- `dataset.py` loads canonical `ds4.jsonl`, loads Shaun's `gepa-good-60` row
  identities, validates all 60 rows against the configured taxonomy, and reports
  the feedback-pool composition.
- Prompt handling loads the correct seed prompt for the selected dataset,
  normalizes the Handlebars variables to Localpager placeholders, extracts the
  editable `routing_policy`, and preserves the frozen scaffold in tests.
- `metric.py` implements the row-aware score with tests that prove random extra
  labels hurt and invalid labels fail instead of earning partial credit.
- A small CLI or test fixture can print a deterministic summary of the chosen
  dataset, prompt seed, and scoring config.

Stretch goal for the same time box: classify one Shaun row through
`localpager-agent` with a mock or live model path and parse `final_json`. Do not
block the required first slice on a slow live rollout.

## Successful run targets

These are the targets for calling a GEPA run successful. They are not minimum
smoke-run thresholds.

GEPA run:

- `proposal_attempts >= 16`.
- `distinct_candidates >= 8` including baseline.
- `accepted_full_eval_candidates >= 6`.
- `max_metric_calls >= 480`.
- `row_limit >= 30` if runtime allows. If runtime forces `row_limit = 18`,
  require stronger external validation before promotion.
- `concurrency = 4` for Qwen/vLLM runs.
- `thinking = medium` for saved benchmark settings and future local classifier
  benchmark runs.
- `max_tokens = 8192` for Qwen classifier rollouts.
- No OOM, model-server instability, hidden retry storm, or untracked stale
  classifier processes competing for the loaded model.

External 60-row validation against v9.1 on the DS4 gold labels:

- Mean weighted score: `>= 0.54`.
- Absolute improvement over v9.1: `>= +0.10`.
- Micro-F1: `>= 0.70`.
- Precision: `>= 0.80`.
- Recall: `>= 0.61`.
- False positives: `<= 20`.
- False negatives: `<= 58`.
- Over-label events: `0` or `1`.
- Structural failures: `0`.
- Exact matches: `>= 15`.

Anti-hacking success checks:

- Mean predicted labels per row must not increase by more than `+0.10`.
- The prompt must not gain score by proposing broad or random extra labels.
- Any score gain with a false-positive regression is not a successful run.
- The winning prompt must beat both v9.1 and the earlier GEPA-six prompt on the
  same 60-row validation set.

Live smoke note from 2026-06-13: row `openclaw-openclaw-48940` now runs through
the production `scripts/localpager-classifier` -> `localpager-agent` path. With
12B and `--max-tokens 512`, the model exhausted the output cap during reasoning
and never called `final_json`; this should be treated as a structural failure,
not a classifier score. With E4B and the same row/cap, `final_json` parsed and
scored `0.25` (`acp`, `gateway`, `sessions` vs DS4 gold `acp`, `gateway`,
`agent_runtime`). With 12B and `--max-tokens 1536`, `final_json` parsed and
scored `0.5` (`acp`, `gateway` vs the same DS4 gold). Later 60-row validation
showed `1536` can still produce `final_json was not called` failures on harder
rows, so the current recommended rollout cap remains `--max-tokens 8192`.
Optimizer live defaults now use Qwen/vLLM, concurrency 4, `--thinking medium`,
and `--max-tokens 8192` unless a run is explicitly marked as E4B smoke/debug or
a thinking-level comparison.

Live GEPA note from 2026-06-13: the best current 12B candidate is saved at
`prompt-optimizer/results/2026-06-13-gepa-12b-six-best.routing_policy.md`.
Across all 60 rows of Shaun's ordered set, it improves the external 12B mean
from `0.4322` to `0.4912`, improves micro-F1 from `0.6510` to `0.6818`,
reduces false positives from `29` to `23`, reduces over-labeling events from
`2` to `1`, and eliminates three structural `final_json` failures. It increases
false negatives from `60` to `61`, so it is more precise but still needs human
review before replacing a deployed OpenClaw prompt template.

## Open questions

- Exact rollout budget vs. wall-clock threshold for switching exploratory runs
  from Qwen/vLLM to `gemma-e4b-reason-test`.
- Whether to ever optimize more than the prompt (schema or topic list); if so,
  the library's `optimize_anything` / adapter framing is the fit, and the
  candidate grows beyond `routing_policy`.
