from __future__ import annotations

from collections import defaultdict
from dataclasses import dataclass
from typing import Iterable


class MetricError(ValueError):
    """Raised when metric inputs violate the scoring contract."""


class InvalidLabelError(MetricError):
    """Raised when a prediction or gold row contains a topic outside the taxonomy."""


@dataclass(frozen=True)
class ScoringConfig:
    false_positive_weight: float = 2.0
    false_negative_weight: float = 1.0
    over_label_weight: float = 0.5


@dataclass(frozen=True)
class RowScore:
    gold: tuple[str, ...]
    predicted: tuple[str, ...]
    true_positives: tuple[str, ...]
    false_positives: tuple[str, ...]
    false_negatives: tuple[str, ...]
    over_label_count: int
    loss: float
    score: float


@dataclass(frozen=True)
class TopicStats:
    true_positives: int = 0
    false_positives: int = 0
    false_negatives: int = 0

    @property
    def precision(self) -> float:
        denominator = self.true_positives + self.false_positives
        return self.true_positives / denominator if denominator else 0.0

    @property
    def recall(self) -> float:
        denominator = self.true_positives + self.false_negatives
        return self.true_positives / denominator if denominator else 0.0


@dataclass(frozen=True)
class BatchScore:
    rows: tuple[RowScore, ...]
    loss: float
    score: float
    micro_precision: float
    micro_recall: float
    micro_f1: float
    per_topic: dict[str, TopicStats]


SCORING_CONFIG = ScoringConfig()


def score_row(
    gold: Iterable[str],
    predicted: Iterable[str],
    allowed_topics: Iterable[str],
    config: ScoringConfig = SCORING_CONFIG,
) -> RowScore:
    allowed = frozenset(allowed_topics)
    gold_topics = _normal_label_tuple(gold, allowed, "gold")
    predicted_topics = _normal_label_tuple(predicted, allowed, "predicted")

    gold_set = set(gold_topics)
    predicted_set = set(predicted_topics)
    true_positives = tuple(topic for topic in predicted_topics if topic in gold_set)
    false_positives = tuple(topic for topic in predicted_topics if topic not in gold_set)
    false_negatives = tuple(topic for topic in gold_topics if topic not in predicted_set)
    over_label_count = max(0, len(predicted_topics) - len(gold_topics))
    loss = (
        config.false_positive_weight * len(false_positives)
        + config.false_negative_weight * len(false_negatives)
        + config.over_label_weight * over_label_count
    )
    return RowScore(
        gold=gold_topics,
        predicted=predicted_topics,
        true_positives=true_positives,
        false_positives=false_positives,
        false_negatives=false_negatives,
        over_label_count=over_label_count,
        loss=loss,
        score=1.0 / (1.0 + loss),
    )


def score_batch(
    gold_rows: Iterable[Iterable[str]],
    predicted_rows: Iterable[Iterable[str]],
    allowed_topics: Iterable[str],
    config: ScoringConfig = SCORING_CONFIG,
) -> BatchScore:
    rows = tuple(
        score_row(gold, predicted, allowed_topics, config)
        for gold, predicted in zip(gold_rows, predicted_rows, strict=True)
    )
    total_loss = sum(row.loss for row in rows)
    total_tp = sum(len(row.true_positives) for row in rows)
    total_fp = sum(len(row.false_positives) for row in rows)
    total_fn = sum(len(row.false_negatives) for row in rows)
    precision = _ratio(total_tp, total_tp + total_fp)
    recall = _ratio(total_tp, total_tp + total_fn)
    per_topic = _per_topic_stats(rows)
    return BatchScore(
        rows=rows,
        loss=total_loss,
        score=1.0 / (1.0 + (total_loss / len(rows))) if rows else 0.0,
        micro_precision=precision,
        micro_recall=recall,
        micro_f1=_f1(precision, recall),
        per_topic=per_topic,
    )


def asi_notes(row_id: str, row_score: RowScore) -> tuple[str, ...]:
    notes: list[str] = []
    for topic in row_score.false_positives:
        notes.append(
            f"{row_id}: predicted `{topic}` but gold does not include it; treat as label spam unless central."
        )
    for topic in row_score.false_negatives:
        notes.append(f"{row_id}: missed gold topic `{topic}`.")
    return tuple(notes)


def _normal_label_tuple(labels: Iterable[str], allowed_topics: frozenset[str], context: str) -> tuple[str, ...]:
    normal: list[str] = []
    seen: set[str] = set()
    for label in labels:
        if not isinstance(label, str) or label == "":
            raise InvalidLabelError(f"{context} label must be a non-empty string")
        if label not in allowed_topics:
            raise InvalidLabelError(f"{context} label not in taxonomy: {label}")
        if label in seen:
            continue
        seen.add(label)
        normal.append(label)
    return tuple(normal)


def _ratio(numerator: int, denominator: int) -> float:
    return numerator / denominator if denominator else 0.0


def _f1(precision: float, recall: float) -> float:
    return 2 * precision * recall / (precision + recall) if precision + recall else 0.0


def _per_topic_stats(rows: tuple[RowScore, ...]) -> dict[str, TopicStats]:
    counts: dict[str, list[int]] = defaultdict(lambda: [0, 0, 0])
    for row in rows:
        for topic in row.true_positives:
            counts[topic][0] += 1
        for topic in row.false_positives:
            counts[topic][1] += 1
        for topic in row.false_negatives:
            counts[topic][2] += 1
    return {
        topic: TopicStats(
            true_positives=values[0],
            false_positives=values[1],
            false_negatives=values[2],
        )
        for topic, values in sorted(counts.items())
    }
