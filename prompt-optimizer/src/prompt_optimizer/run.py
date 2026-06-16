from __future__ import annotations

import hashlib
import json
from dataclasses import asdict, dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from prompt_optimizer.adapter import LocalpagerAdapter, ROUTING_POLICY_COMPONENT
from prompt_optimizer.dataset import (
    DEFAULT_DS4_PATH,
    DEFAULT_EVALSTATE_HELDOUT_PATH,
    DEFAULT_EVALSTATE_PARETO_PATH,
    DEFAULT_EVALSTATE_TRAIN_PATH,
    DEFAULT_FEEDBACK_MANIFEST_PATH,
    DEFAULT_TAXONOMY_PATH,
    DEFAULT_V2_TAXONOMY_PATH,
    FeedbackPoolRow,
    build_feedback_pool,
    build_evalstate_pool,
    load_evalstate_split,
    load_taxonomy,
)
from prompt_optimizer.harness import (
    DEFAULT_MAX_TOKENS,
    ClassifierHarness,
    LocalpagerAgentHarness,
    StaticClassifierHarness,
)
from prompt_optimizer.prompt import (
    DEFAULT_EVALSTATE_SEED_OVERLAY_PATH,
    DEFAULT_EVALSTATE_SEED_PROMPT_PATH,
    DEFAULT_SEED_PROMPT_PATH,
    PromptParts,
    load_overlay_seed_prompt,
    load_seed_prompt,
)
from prompt_optimizer.reflection import CodexReflectionLM

DEFAULT_MODEL = "nvidia/Qwen3.6-35B-A3B-NVFP4"
DEFAULT_BASE_URL = "http://127.0.0.1:8000/v1"
FALLBACK_MODEL = "gemma-e4b-reason-test"
DEFAULT_CONCURRENCY = 4


@dataclass(frozen=True)
class OptimizerInputs:
    rows: tuple[FeedbackPoolRow, ...]
    allowed_topics: frozenset[str]
    prompt_parts: PromptParts
    pareto_rows: tuple[FeedbackPoolRow, ...] = ()
    heldout_rows: tuple[FeedbackPoolRow, ...] = ()
    dataset_name: str = "ds4-gepa-good-60"


@dataclass(frozen=True)
class HarnessConfig:
    model: str = DEFAULT_MODEL
    concurrency: int = DEFAULT_CONCURRENCY
    max_tokens: int = DEFAULT_MAX_TOKENS
    timeout_ms: int = 900_000
    base_url: str | None = DEFAULT_BASE_URL
    context_window: int | None = None
    thinking: str = "medium"
    state_dir: Path | None = None


@dataclass(frozen=True)
class GEPARunConfig:
    output_dir: Path
    max_metric_calls: int
    max_candidate_proposals: int | None = None
    reflection_minibatch_size: int = 4
    seed: int = 0
    row_limit: int | None = None
    dataset_name: str | None = None
    seed_routing_policy: str | None = None
    harness: HarnessConfig = HarnessConfig()


def load_optimizer_inputs(
    *,
    ds4_path: Path = DEFAULT_DS4_PATH,
    feedback_manifest_path: Path = DEFAULT_FEEDBACK_MANIFEST_PATH,
    taxonomy_path: Path = DEFAULT_TAXONOMY_PATH,
    seed_prompt_path: Path = DEFAULT_SEED_PROMPT_PATH,
) -> OptimizerInputs:
    pool = build_feedback_pool(ds4_path, feedback_manifest_path, taxonomy_path)
    return OptimizerInputs(
        rows=pool.rows,
        allowed_topics=load_taxonomy(taxonomy_path),
        prompt_parts=load_seed_prompt(seed_prompt_path),
    )


def load_evalstate_optimizer_inputs(
    *,
    train_path: Path = DEFAULT_EVALSTATE_TRAIN_PATH,
    pareto_path: Path = DEFAULT_EVALSTATE_PARETO_PATH,
    heldout_path: Path = DEFAULT_EVALSTATE_HELDOUT_PATH,
    taxonomy_path: Path = DEFAULT_V2_TAXONOMY_PATH,
    seed_prompt_path: Path = DEFAULT_EVALSTATE_SEED_PROMPT_PATH,
    seed_overlay_path: Path = DEFAULT_EVALSTATE_SEED_OVERLAY_PATH,
) -> OptimizerInputs:
    allowed_topics = load_taxonomy(taxonomy_path)
    train_pool = build_evalstate_pool(train_path, taxonomy_path, split_name="feedback")
    return OptimizerInputs(
        rows=train_pool.rows,
        pareto_rows=load_evalstate_split(pareto_path, allowed_topics, split_name="pareto"),
        heldout_rows=load_evalstate_split(heldout_path, allowed_topics, split_name="heldout"),
        allowed_topics=allowed_topics,
        prompt_parts=load_overlay_seed_prompt(seed_prompt_path, seed_overlay_path),
        dataset_name="evalstate-openclaw-git-labels",
    )


def seed_candidate(prompt_parts: PromptParts) -> dict[str, str]:
    return {ROUTING_POLICY_COMPONENT: prompt_parts.routing_policy}


def make_adapter(
    *,
    inputs: OptimizerInputs,
    harness: ClassifierHarness,
    concurrency: int = DEFAULT_CONCURRENCY,
) -> LocalpagerAdapter:
    return LocalpagerAdapter(
        prompt_parts=inputs.prompt_parts,
        harness=harness,
        allowed_topics=inputs.allowed_topics,
        concurrency=concurrency,
    )


def static_empty_harness() -> StaticClassifierHarness:
    return StaticClassifierHarness(predictions={})


def localpager_agent_harness(config: HarnessConfig) -> LocalpagerAgentHarness:
    return LocalpagerAgentHarness(
        model=config.model,
        base_url=config.base_url,
        context_window=config.context_window,
        thinking=config.thinking,
        max_tokens=config.max_tokens,
        timeout_ms=config.timeout_ms,
        state_dir=config.state_dir,
    )


def evaluate_seed(
    *,
    inputs: OptimizerInputs,
    harness: ClassifierHarness,
    concurrency: int = DEFAULT_CONCURRENCY,
    limit: int | None = None,
    offset: int = 0,
    capture_traces: bool = True,
) -> dict[str, Any]:
    rows = _selected_rows(inputs.rows, limit, offset)
    adapter = make_adapter(inputs=inputs, harness=harness, concurrency=concurrency)
    batch = adapter.evaluate(list(rows), seed_candidate(inputs.prompt_parts), capture_traces=capture_traces)
    return evaluation_report(
        rows,
        batch,
        routing_policy=inputs.prompt_parts.routing_policy,
        candidate_name="seed",
    )


def evaluate_routing_policy(
    *,
    inputs: OptimizerInputs,
    routing_policy: str,
    harness: ClassifierHarness,
    concurrency: int = DEFAULT_CONCURRENCY,
    limit: int | None = None,
    offset: int = 0,
    capture_traces: bool = True,
    candidate_name: str = "candidate",
) -> dict[str, Any]:
    rows = _selected_rows(inputs.rows, limit, offset)
    adapter = make_adapter(inputs=inputs, harness=harness, concurrency=concurrency)
    batch = adapter.evaluate(
        list(rows),
        {ROUTING_POLICY_COMPONENT: routing_policy},
        capture_traces=capture_traces,
    )
    return evaluation_report(
        rows,
        batch,
        routing_policy=routing_policy,
        candidate_name=candidate_name,
    )


def run_gepa(
    *,
    inputs: OptimizerInputs,
    config: GEPARunConfig,
) -> dict[str, Any]:
    import gepa

    rows = list(_selected_rows(inputs.rows, config.row_limit, 0))
    val_rows = list(inputs.pareto_rows or rows)
    harness = localpager_agent_harness(config.harness)
    adapter = make_adapter(inputs=inputs, harness=harness, concurrency=config.harness.concurrency)
    initial_candidate = (
        {ROUTING_POLICY_COMPONENT: config.seed_routing_policy}
        if config.seed_routing_policy is not None
        else seed_candidate(inputs.prompt_parts)
    )
    config.output_dir.mkdir(parents=True, exist_ok=True)
    stop_callbacks = None
    if config.max_candidate_proposals is not None:
        from gepa.utils import MaxCandidateProposalsStopper

        stop_callbacks = [MaxCandidateProposalsStopper(config.max_candidate_proposals)]
    result = gepa.optimize(
        seed_candidate=initial_candidate,
        trainset=rows,
        valset=val_rows,
        adapter=adapter,
        reflection_lm=CodexReflectionLM(),
        max_metric_calls=config.max_metric_calls,
        stop_callbacks=stop_callbacks,
        reflection_minibatch_size=config.reflection_minibatch_size,
        run_dir=str(config.output_dir),
        seed=config.seed,
        display_progress_bar=False,
    )
    return write_result_artifacts(result, inputs.prompt_parts, config.output_dir, config)


def write_result_artifacts(
    result: Any,
    prompt_parts: PromptParts,
    output_dir: Path,
    config: GEPARunConfig,
) -> dict[str, Any]:
    output_dir.mkdir(parents=True, exist_ok=True)
    best_candidate = result.best_candidate
    if not isinstance(best_candidate, dict):
        raise TypeError("GEPA best_candidate must be a dict for routing_policy optimization")
    best_routing_policy = best_candidate[ROUTING_POLICY_COMPONENT]
    summary = {
        "created_at": datetime.now(timezone.utc).isoformat(),
        "best_idx": result.best_idx,
        "best_score": result.val_aggregate_scores[result.best_idx],
        "num_candidates": result.num_candidates,
        "total_metric_calls": result.total_metric_calls,
        "num_full_val_evals": result.num_full_val_evals,
        "dataset_name": config.dataset_name,
        "config": _jsonable_config(config),
        "best_prompt_path": str(output_dir / "best.prompt.md"),
        "best_routing_policy_path": str(output_dir / "best.routing_policy.md"),
        "result_path": str(output_dir / "gepa-result.json"),
    }
    (output_dir / "best.routing_policy.md").write_text(best_routing_policy, encoding="utf-8")
    (output_dir / "best.prompt.md").write_text(prompt_parts.assemble(best_routing_policy), encoding="utf-8")
    (output_dir / "summary.json").write_text(json.dumps(summary, indent=2, sort_keys=True), encoding="utf-8")
    (output_dir / "gepa-result.json").write_text(
        json.dumps(result.to_dict(), default=str, indent=2, sort_keys=True),
        encoding="utf-8",
    )
    return summary


def evaluation_report(
    rows: tuple[FeedbackPoolRow, ...],
    batch: Any,
    *,
    routing_policy: str,
    candidate_name: str,
) -> dict[str, Any]:
    row_reports = []
    trajectories = batch.trajectories or []
    for index, (row, output, score) in enumerate(zip(rows, batch.outputs, batch.scores, strict=True)):
        report = {
            "id": row.ds4.id,
            "target": row.ds4.target or row.ds4.url,
            "title": row.ds4.title,
            "gold_topics": list(row.ds4.topics_of_interest),
            "predicted_topics": list(output.topics_of_interest),
            "score": score,
            "error": output.error,
        }
        if index < len(trajectories):
            row_score = getattr(trajectories[index], "score", None)
            if row_score is not None:
                report.update(
                    {
                        "true_positives": list(row_score.true_positives),
                        "false_positives": list(row_score.false_positives),
                        "false_negatives": list(row_score.false_negatives),
                        "over_label_count": row_score.over_label_count,
                        "duplicate_label_count": row_score.duplicate_label_count,
                        "over_cardinality_count": row_score.over_cardinality_count,
                        "precision": row_score.precision,
                        "recall": row_score.recall,
                        "f1": row_score.f1,
                        "row_jaccard": row_score.row_jaccard,
                        "row_topic_f1": row_score.row_topic_f1,
                        "row_exact": row_score.row_exact,
                        "policy_penalty": row_score.policy_penalty,
                        "loss": row_score.loss,
                    }
                )
        row_reports.append(report)
    mean_score = sum(batch.scores) / len(batch.scores) if batch.scores else 0.0
    return {
        "candidate": candidate_name,
        "routing_policy_sha256": _sha256(routing_policy),
        "rows": len(row_reports),
        "mean_score": mean_score,
        "scores": list(batch.scores),
        "row_reports": row_reports,
    }


def default_output_dir(root: Path = Path("prompt-optimizer/out")) -> Path:
    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    return root / f"gepa-{stamp}"


def _selected_rows(rows: tuple[FeedbackPoolRow, ...], limit: int | None, offset: int) -> tuple[FeedbackPoolRow, ...]:
    if offset < 0:
        raise ValueError("offset must be non-negative")
    rows = rows[offset:]
    if limit is None:
        return rows
    if limit < 1:
        raise ValueError("limit must be at least 1")
    return rows[:limit]


def _jsonable_config(config: GEPARunConfig) -> dict[str, Any]:
    payload = asdict(config)
    payload["output_dir"] = str(config.output_dir)
    seed_routing_policy = payload.pop("seed_routing_policy")
    payload["seed_routing_policy_sha256"] = (
        _sha256(seed_routing_policy) if seed_routing_policy is not None else None
    )
    payload["seed_routing_policy_chars"] = (
        len(seed_routing_policy) if seed_routing_policy is not None else None
    )
    harness = payload["harness"]
    if harness["state_dir"] is not None:
        harness["state_dir"] = str(harness["state_dir"])
    return payload


def _sha256(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()
