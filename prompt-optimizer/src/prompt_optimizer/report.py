from __future__ import annotations

import json
import re
from pathlib import Path
from typing import Any


_NUMBER = r"([0-9]+(?:\.[0-9]+)?)"


def summarize_gepa_run(run_dir: Path) -> dict[str, Any]:
    """Summarize a GEPA run directory while it is running or after completion."""

    run_log_text = _read_text(run_dir / "run_log.txt")
    result = _read_json(run_dir / "gepa-result.json")
    summary = _read_json(run_dir / "summary.json")
    return {
        "run_dir": str(run_dir),
        "run_log": summarize_run_log(run_log_text),
        "result": summarize_gepa_result(result),
        "summary": summary,
    }


def summarize_run_log(text: str) -> dict[str, Any]:
    base_score: float | None = None
    selected_events: list[dict[str, Any]] = []
    proposal_events: list[dict[str, Any]] = []
    better_valset_scores: list[dict[str, Any]] = []

    pending_proposal_iteration: int | None = None
    for line in text.splitlines():
        if match := re.search(r"Iteration 0: Base program full valset score: " + _NUMBER, line):
            base_score = float(match.group(1))
            continue
        if match := re.search(r"Iteration ([0-9]+): Selected program ([0-9]+) score: " + _NUMBER, line):
            selected_events.append(
                {
                    "iteration": int(match.group(1)),
                    "candidate_idx": int(match.group(2)),
                    "score": float(match.group(3)),
                }
            )
            continue
        if match := re.search(r"Iteration ([0-9]+): Proposed new text for ", line):
            pending_proposal_iteration = int(match.group(1))
            continue
        if match := re.search(
            r"Iteration ([0-9]+): New subsample score "
            + _NUMBER
            + r" is (not better|better) than old score "
            + _NUMBER,
            line,
        ):
            iteration = int(match.group(1))
            new_score = float(match.group(2))
            decision = match.group(3)
            old_score = float(match.group(4))
            proposal_events.append(
                {
                    "iteration": iteration,
                    "old_subsample_sum": old_score,
                    "new_subsample_sum": new_score,
                    "delta": new_score - old_score,
                    "accepted_for_full_eval": decision == "better",
                    "has_proposed_text": pending_proposal_iteration == iteration,
                }
            )
            if pending_proposal_iteration == iteration:
                pending_proposal_iteration = None
            continue
        if match := re.search(r"Iteration ([0-9]+): Found a better program on the valset with score " + _NUMBER, line):
            better_valset_scores.append({"iteration": int(match.group(1)), "score": float(match.group(2))})

    return {
        "base_score": base_score,
        "selected_iterations": len(selected_events),
        "proposal_attempts": len(proposal_events),
        "proposal_texts_started": len(re.findall(r"Iteration [0-9]+: Proposed new text for ", text)),
        "accepted_full_eval_candidates": sum(1 for event in proposal_events if event["accepted_for_full_eval"]),
        "rejected_candidates": sum(1 for event in proposal_events if not event["accepted_for_full_eval"]),
        "better_valset_events": better_valset_scores,
        "selected_events": selected_events,
        "proposal_events": proposal_events,
        "line_count": len(text.splitlines()),
        "byte_count": len(text.encode("utf-8")),
    }


def summarize_gepa_result(result: Any) -> dict[str, Any] | None:
    if not isinstance(result, dict):
        return None
    scores = result.get("val_aggregate_scores")
    if not isinstance(scores, list):
        scores = []
    return {
        "best_idx": result.get("best_idx"),
        "total_metric_calls": result.get("total_metric_calls"),
        "num_candidates": len(result.get("candidates") or []),
        "num_full_val_evals": result.get("num_full_val_evals"),
        "val_aggregate_scores": scores,
    }


def _read_text(path: Path) -> str:
    if not path.exists():
        return ""
    return path.read_text(encoding="utf-8", errors="replace")


def _read_json(path: Path) -> Any:
    if not path.exists() or path.stat().st_size == 0:
        return None
    return json.loads(path.read_text(encoding="utf-8"))
