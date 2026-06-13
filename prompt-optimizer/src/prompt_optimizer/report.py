from __future__ import annotations

import json
import re
from html import escape
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


def summarize_evaluation_file(path: Path) -> dict[str, Any]:
    """Summarize an evaluate-candidate JSON artifact without model calls."""

    return summarize_evaluation_report(_read_json(path), source_path=path)


def summarize_evaluation_report(report: Any, *, source_path: Path | None = None) -> dict[str, Any]:
    if not isinstance(report, dict):
        raise ValueError("evaluation report must be a JSON object")
    rows = report.get("row_reports")
    if not isinstance(rows, list):
        raise ValueError("evaluation report must include row_reports")

    total_tp = 0
    total_fp = 0
    total_fn = 0
    total_gold = 0
    total_predicted = 0
    exact_matches = 0
    over_label_events = 0
    over_label_total = 0
    structural_failures = 0
    per_topic: dict[str, dict[str, int]] = {}

    for row in rows:
        if not isinstance(row, dict):
            continue
        gold = _string_list(row.get("gold_topics"))
        predicted = _string_list(row.get("predicted_topics"))
        gold_set = set(gold)
        predicted_set = set(predicted)
        true_positives = predicted_set & gold_set
        false_positives = predicted_set - gold_set
        false_negatives = gold_set - predicted_set

        total_tp += len(true_positives)
        total_fp += len(false_positives)
        total_fn += len(false_negatives)
        total_gold += len(gold)
        total_predicted += len(predicted)
        exact_matches += int(false_positives == set() and false_negatives == set())
        over_label_count = max(0, len(predicted) - len(gold))
        over_label_events += int(over_label_count > 0)
        over_label_total += over_label_count
        structural_failures += int(row.get("error") is not None)

        for topic in true_positives:
            _topic_counts(per_topic, topic)["true_positives"] += 1
        for topic in false_positives:
            _topic_counts(per_topic, topic)["false_positives"] += 1
        for topic in false_negatives:
            _topic_counts(per_topic, topic)["false_negatives"] += 1

    precision = _ratio(total_tp, total_tp + total_fp)
    recall = _ratio(total_tp, total_tp + total_fn)
    row_count = len(rows)
    return {
        "source_path": str(source_path) if source_path is not None else None,
        "candidate": report.get("candidate"),
        "rows": row_count,
        "mean_score": report.get("mean_score"),
        "true_positives": total_tp,
        "false_positives": total_fp,
        "false_negatives": total_fn,
        "precision": precision,
        "recall": recall,
        "micro_f1": _f1(precision, recall),
        "exact_matches": exact_matches,
        "over_label_events": over_label_events,
        "over_label_total": over_label_total,
        "structural_failures": structural_failures,
        "mean_gold_labels": total_gold / row_count if row_count else 0.0,
        "mean_predicted_labels": total_predicted / row_count if row_count else 0.0,
        "per_topic": {
            topic: {
                **counts,
                "precision": _ratio(counts["true_positives"], counts["true_positives"] + counts["false_positives"]),
                "recall": _ratio(counts["true_positives"], counts["true_positives"] + counts["false_negatives"]),
            }
            for topic, counts in sorted(per_topic.items())
        },
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


def write_gepa_run_report(run_dir: Path, output_path: Path | None = None) -> Path:
    """Write a self-contained HTML report with GEPA iteration charts."""

    summary = summarize_gepa_run(run_dir)
    if output_path is None:
        output_path = run_dir / "score_report.html"
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(render_gepa_run_report(summary), encoding="utf-8")
    return output_path


def render_gepa_run_report(summary: dict[str, Any]) -> str:
    run_log = summary.get("run_log") or {}
    result = summary.get("result") or {}
    run_dir = str(summary.get("run_dir") or "")

    selected_events = _dict_list(run_log.get("selected_events"))
    proposal_events = _dict_list(run_log.get("proposal_events"))
    better_events = _dict_list(run_log.get("better_valset_events"))
    candidate_scores = _number_list(result.get("val_aggregate_scores"))

    base_score = _optional_float(run_log.get("base_score"))
    selected_points = [
        (int(event["iteration"]), float(event["score"]))
        for event in selected_events
        if isinstance(event.get("iteration"), int) and isinstance(event.get("score"), (int, float))
    ]
    best_points: list[tuple[int, float]] = []
    if base_score is not None:
        best_points.append((0, base_score))
    best_so_far = base_score
    for event in better_events:
        iteration = event.get("iteration")
        score = event.get("score")
        if not isinstance(iteration, int) or not isinstance(score, (int, float)):
            continue
        best_so_far = max(float(score), best_so_far if best_so_far is not None else float(score))
        best_points.append((iteration, best_so_far))

    proposal_delta_points = [
        (int(event["iteration"]), float(event["delta"]))
        for event in proposal_events
        if isinstance(event.get("iteration"), int) and isinstance(event.get("delta"), (int, float))
    ]

    return "\n".join(
        [
            "<!doctype html>",
            '<html lang="en">',
            "<head>",
            '<meta charset="utf-8">',
            '<meta name="viewport" content="width=device-width, initial-scale=1">',
            f"<title>{escape(Path(run_dir).name)} GEPA score report</title>",
            "<style>",
            _REPORT_CSS,
            "</style>",
            "</head>",
            "<body>",
            "<main>",
            f"<h1>{escape(Path(run_dir).name)}</h1>",
            f"<p class=\"muted\">Run directory: <code>{escape(run_dir)}</code></p>",
            _summary_cards(run_log, result),
            _section(
                "Validation Score Over Iterations",
                _line_chart(
                    [
                        ("selected candidate", selected_points, "#2563eb"),
                        ("best so far", best_points, "#16a34a"),
                    ],
                    y_min=0.0,
                    y_max=1.0,
                    empty_message="No iteration scores found yet.",
                ),
            ),
            _section(
                "Proposal Subsample Delta",
                _bar_chart(
                    proposal_delta_points,
                    positive_color="#16a34a",
                    negative_color="#dc2626",
                    empty_message="No proposal deltas found yet.",
                ),
            ),
            _section(
                "Final Candidate Scores",
                _bar_chart(
                    list(enumerate(candidate_scores)),
                    positive_color="#2563eb",
                    negative_color="#dc2626",
                    empty_message="Final GEPA result is not available yet.",
                    y_min=0.0,
                    y_max=1.0,
                    x_prefix="candidate ",
                ),
            ),
            _proposal_table(proposal_events),
            "</main>",
            "</body>",
            "</html>",
            "",
        ]
    )


def _read_text(path: Path) -> str:
    if not path.exists():
        return ""
    return path.read_text(encoding="utf-8", errors="replace")


def _read_json(path: Path) -> Any:
    if not path.exists() or path.stat().st_size == 0:
        return None
    return json.loads(path.read_text(encoding="utf-8"))


def _dict_list(value: Any) -> list[dict[str, Any]]:
    if not isinstance(value, list):
        return []
    return [item for item in value if isinstance(item, dict)]


def _number_list(value: Any) -> list[float]:
    if not isinstance(value, list):
        return []
    return [float(item) for item in value if isinstance(item, (int, float))]


def _string_list(value: Any) -> list[str]:
    if not isinstance(value, list):
        return []
    return [item for item in value if isinstance(item, str)]


def _topic_counts(per_topic: dict[str, dict[str, int]], topic: str) -> dict[str, int]:
    return per_topic.setdefault(
        topic,
        {
            "true_positives": 0,
            "false_positives": 0,
            "false_negatives": 0,
        },
    )


def _ratio(numerator: int, denominator: int) -> float:
    return numerator / denominator if denominator else 0.0


def _f1(precision: float, recall: float) -> float:
    return 2 * precision * recall / (precision + recall) if precision + recall else 0.0


def _optional_float(value: Any) -> float | None:
    if not isinstance(value, (int, float)):
        return None
    return float(value)


def _summary_cards(run_log: dict[str, Any], result: dict[str, Any]) -> str:
    cards = [
        ("Base score", _format_value(run_log.get("base_score"))),
        ("Proposal attempts", _format_value(run_log.get("proposal_attempts"))),
        ("Accepted full evals", _format_value(run_log.get("accepted_full_eval_candidates"))),
        ("Rejected proposals", _format_value(run_log.get("rejected_candidates"))),
        ("Candidates", _format_value(result.get("num_candidates"))),
        ("Best candidate", _format_value(result.get("best_idx"))),
        ("Metric calls", _format_value(result.get("total_metric_calls"))),
    ]
    body = "\n".join(
        f'<div class="card"><div class="label">{escape(label)}</div><div class="value">{escape(value)}</div></div>'
        for label, value in cards
    )
    return f'<section class="cards">{body}</section>'


def _section(title: str, body: str) -> str:
    return f'<section class="panel"><h2>{escape(title)}</h2>{body}</section>'


def _line_chart(
    series: list[tuple[str, list[tuple[int, float]], str]],
    *,
    y_min: float,
    y_max: float,
    empty_message: str,
) -> str:
    width = 900
    height = 320
    pad_left = 56
    pad_right = 24
    pad_top = 24
    pad_bottom = 44
    all_points = [point for _, points, _ in series for point in points]
    if not all_points:
        return f'<p class="empty">{escape(empty_message)}</p>'
    x_values = [x for x, _ in all_points]
    x_min = min(x_values)
    x_max = max(max(x_values), x_min + 1)

    def sx(x: int) -> float:
        return pad_left + (x - x_min) / (x_max - x_min) * (width - pad_left - pad_right)

    def sy(y: float) -> float:
        clipped = max(y_min, min(y_max, y))
        return pad_top + (y_max - clipped) / (y_max - y_min) * (height - pad_top - pad_bottom)

    chart_parts = _axis_svg(width, height, pad_left, pad_right, pad_top, pad_bottom, y_min, y_max)
    for label, points, color in series:
        if not points:
            continue
        ordered = sorted(points)
        path_points = " ".join(f"{sx(x):.1f},{sy(y):.1f}" for x, y in ordered)
        chart_parts.append(
            f'<polyline points="{path_points}" fill="none" stroke="{escape(color)}" stroke-width="3" '
            'stroke-linecap="round" stroke-linejoin="round" />'
        )
        for x, y in ordered:
            chart_parts.append(
                f'<circle cx="{sx(x):.1f}" cy="{sy(y):.1f}" r="4" fill="{escape(color)}">'
                f"<title>{escape(label)} iteration {x}: {_format_value(y)}</title></circle>"
            )
    legend = "".join(
        f'<span><i style="background:{escape(color)}"></i>{escape(label)}</span>' for label, _, color in series
    )
    return (
        f'<svg viewBox="0 0 {width} {height}" role="img">{_join(chart_parts)}</svg>'
        f'<div class="legend">{legend}</div>'
    )


def _bar_chart(
    points: list[tuple[int, float]],
    *,
    positive_color: str,
    negative_color: str,
    empty_message: str,
    y_min: float | None = None,
    y_max: float | None = None,
    x_prefix: str = "iteration ",
) -> str:
    width = 900
    height = 320
    pad_left = 56
    pad_right = 24
    pad_top = 24
    pad_bottom = 52
    if not points:
        return f'<p class="empty">{escape(empty_message)}</p>'
    values = [value for _, value in points]
    low = min(values + ([y_min] if y_min is not None else [0.0]))
    high = max(values + ([y_max] if y_max is not None else [0.0]))
    if y_min is not None:
        low = y_min
    if y_max is not None:
        high = y_max
    if low == high:
        high = low + 1.0
    x_count = max(len(points), 1)
    plot_width = width - pad_left - pad_right
    bar_slot = plot_width / x_count
    bar_width = min(42.0, max(12.0, bar_slot * 0.68))

    def sy(y: float) -> float:
        return pad_top + (high - y) / (high - low) * (height - pad_top - pad_bottom)

    baseline = sy(0.0 if low <= 0 <= high else low)
    chart_parts = _axis_svg(width, height, pad_left, pad_right, pad_top, pad_bottom, low, high)
    for index, (x_value, y_value) in enumerate(points):
        center = pad_left + index * bar_slot + bar_slot / 2
        top = min(sy(y_value), baseline)
        bar_height = abs(baseline - sy(y_value))
        color = positive_color if y_value >= 0 else negative_color
        chart_parts.append(
            f'<rect x="{center - bar_width / 2:.1f}" y="{top:.1f}" width="{bar_width:.1f}" '
            f'height="{max(bar_height, 1.0):.1f}" fill="{escape(color)}">'
            f"<title>{escape(x_prefix)}{x_value}: {_format_value(y_value)}</title></rect>"
        )
        chart_parts.append(
            f'<text x="{center:.1f}" y="{height - 24}" text-anchor="middle" class="tick">{escape(str(x_value))}</text>'
        )
    return f'<svg viewBox="0 0 {width} {height}" role="img">{_join(chart_parts)}</svg>'


def _axis_svg(
    width: int,
    height: int,
    pad_left: int,
    pad_right: int,
    pad_top: int,
    pad_bottom: int,
    y_min: float,
    y_max: float,
) -> list[str]:
    x_axis_y = height - pad_bottom
    x_axis_x2 = width - pad_right
    parts = [
        f'<line x1="{pad_left}" y1="{x_axis_y}" x2="{x_axis_x2}" y2="{x_axis_y}" class="axis" />',
        f'<line x1="{pad_left}" y1="{pad_top}" x2="{pad_left}" y2="{x_axis_y}" class="axis" />',
    ]
    steps = 4
    for index in range(steps + 1):
        value = y_min + (y_max - y_min) * index / steps
        y = pad_top + (y_max - value) / (y_max - y_min) * (height - pad_top - pad_bottom)
        parts.append(f'<line x1="{pad_left}" y1="{y:.1f}" x2="{width - pad_right}" y2="{y:.1f}" class="grid" />')
        parts.append(
            f'<text x="{pad_left - 10}" y="{y + 4:.1f}" text-anchor="end" class="tick">'
            f"{_format_value(value)}</text>"
        )
    return parts


def _proposal_table(proposal_events: list[dict[str, Any]]) -> str:
    if not proposal_events:
        return _section("Proposal Events", '<p class="empty">No proposal events found yet.</p>')
    rows = []
    for event in proposal_events:
        decision = "accepted" if event.get("accepted_for_full_eval") else "rejected"
        rows.append(
            "<tr>"
            f"<td>{escape(str(event.get('iteration', '')))}</td>"
            f"<td>{escape(_format_value(event.get('old_subsample_sum')))}</td>"
            f"<td>{escape(_format_value(event.get('new_subsample_sum')))}</td>"
            f"<td>{escape(_format_value(event.get('delta')))}</td>"
            f"<td>{escape(decision)}</td>"
            "</tr>"
        )
    table = (
        "<table><thead><tr><th>Iteration</th><th>Old subsample</th><th>New subsample</th>"
        "<th>Delta</th><th>Decision</th></tr></thead><tbody>"
        + "\n".join(rows)
        + "</tbody></table>"
    )
    return _section("Proposal Events", table)


def _format_value(value: Any) -> str:
    if value is None:
        return "n/a"
    if isinstance(value, float):
        return f"{value:.4f}"
    return str(value)


def _join(parts: list[str]) -> str:
    return "\n".join(parts)


_REPORT_CSS = """
:root {
  color-scheme: light;
  font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  color: #111827;
  background: #f8fafc;
}
body {
  margin: 0;
}
main {
  width: min(1120px, calc(100vw - 32px));
  margin: 0 auto;
  padding: 32px 0 48px;
}
h1 {
  margin: 0 0 8px;
  font-size: 28px;
  line-height: 1.2;
}
h2 {
  margin: 0 0 16px;
  font-size: 18px;
}
code {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}
.muted,
.empty,
.tick {
  color: #64748b;
}
.muted {
  margin: 0 0 24px;
}
.cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 12px;
  margin-bottom: 16px;
}
.card,
.panel {
  background: #ffffff;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.04);
}
.card {
  padding: 14px 16px;
}
.label {
  color: #64748b;
  font-size: 13px;
}
.value {
  margin-top: 6px;
  font-size: 24px;
  font-weight: 700;
}
.panel {
  margin-top: 16px;
  padding: 18px;
}
svg {
  width: 100%;
  height: auto;
  display: block;
}
.axis {
  stroke: #94a3b8;
  stroke-width: 1.4;
}
.grid {
  stroke: #e2e8f0;
  stroke-width: 1;
}
.tick {
  font-size: 12px;
}
.legend {
  display: flex;
  gap: 18px;
  margin-top: 10px;
  color: #334155;
  font-size: 13px;
}
.legend i {
  display: inline-block;
  width: 10px;
  height: 10px;
  border-radius: 999px;
  margin-right: 6px;
  vertical-align: -1px;
}
table {
  width: 100%;
  border-collapse: collapse;
  font-size: 14px;
}
th,
td {
  padding: 10px 8px;
  border-bottom: 1px solid #e5e7eb;
  text-align: left;
}
th {
  color: #475569;
  font-weight: 700;
}
""".strip()
