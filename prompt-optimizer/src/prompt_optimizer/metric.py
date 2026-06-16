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
    fbeta_beta: float = 0.5
    fbeta_weight: float = 0.55
    micro_f1_weight: float = 0.20
    micro_precision_weight: float = 0.15
    cardinality_closeness_weight: float = 0.07
    exact_match_weight: float = 0.03


@dataclass(frozen=True)
class RowScore:
    gold: tuple[str, ...]
    predicted: tuple[str, ...]
    true_positives: tuple[str, ...]
    false_positives: tuple[str, ...]
    false_negatives: tuple[str, ...]
    over_label_count: int
    precision: float
    recall: float
    f1: float
    fbeta: float
    cardinality_closeness: float
    exact_match: float
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
    micro_fbeta: float
    cardinality_closeness: float
    exact_match: float
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
    precision = _label_precision(len(true_positives), len(false_positives), len(gold_topics))
    recall = _label_recall(len(true_positives), len(false_negatives))
    f1 = _f1(precision, recall)
    fbeta = _fbeta(precision, recall, config.fbeta_beta)
    cardinality_closeness = _cardinality_closeness(len(predicted_topics), len(gold_topics))
    exact_match = 1.0 if predicted_set == gold_set else 0.0
    score = _weighted_score(
        fbeta=fbeta,
        micro_f1=f1,
        micro_precision=precision,
        cardinality_closeness=cardinality_closeness,
        exact_match=exact_match,
        config=config,
    )
    return RowScore(
        gold=gold_topics,
        predicted=predicted_topics,
        true_positives=true_positives,
        false_positives=false_positives,
        false_negatives=false_negatives,
        over_label_count=over_label_count,
        precision=precision,
        recall=recall,
        f1=f1,
        fbeta=fbeta,
        cardinality_closeness=cardinality_closeness,
        exact_match=exact_match,
        loss=1.0 - score,
        score=score,
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
    if not rows:
        return BatchScore(
            rows=(),
            loss=1.0,
            score=0.0,
            micro_precision=0.0,
            micro_recall=0.0,
            micro_f1=0.0,
            micro_fbeta=0.0,
            cardinality_closeness=0.0,
            exact_match=0.0,
            per_topic={},
        )
    total_tp = sum(len(row.true_positives) for row in rows)
    total_fp = sum(len(row.false_positives) for row in rows)
    total_fn = sum(len(row.false_negatives) for row in rows)
    total_gold = sum(len(row.gold) for row in rows)
    precision = _label_precision(total_tp, total_fp, total_gold)
    recall = _label_recall(total_tp, total_fn)
    micro_f1 = _f1(precision, recall)
    micro_fbeta = _fbeta(precision, recall, config.fbeta_beta)
    cardinality_closeness = sum(row.cardinality_closeness for row in rows) / len(rows)
    exact_match = sum(row.exact_match for row in rows) / len(rows)
    score = _weighted_score(
        fbeta=micro_fbeta,
        micro_f1=micro_f1,
        micro_precision=precision,
        cardinality_closeness=cardinality_closeness,
        exact_match=exact_match,
        config=config,
    )
    per_topic = _per_topic_stats(rows)
    return BatchScore(
        rows=rows,
        loss=1.0 - score,
        score=score,
        micro_precision=precision,
        micro_recall=recall,
        micro_f1=micro_f1,
        micro_fbeta=micro_fbeta,
        cardinality_closeness=cardinality_closeness,
        exact_match=exact_match,
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


def _label_precision(true_positives: int, false_positives: int, gold_count: int) -> float:
    denominator = true_positives + false_positives
    if denominator:
        return true_positives / denominator
    return 1.0 if gold_count == 0 else 0.0


def _label_recall(true_positives: int, false_negatives: int) -> float:
    denominator = true_positives + false_negatives
    return true_positives / denominator if denominator else 1.0


def _f1(precision: float, recall: float) -> float:
    return 2 * precision * recall / (precision + recall) if precision + recall else 0.0


def _fbeta(precision: float, recall: float, beta: float) -> float:
    if precision + recall == 0:
        return 0.0
    beta_squared = beta * beta
    return (1 + beta_squared) * precision * recall / ((beta_squared * precision) + recall)


def _cardinality_closeness(predicted_count: int, gold_count: int) -> float:
    denominator = max(predicted_count, gold_count, 1)
    return 1.0 - (abs(predicted_count - gold_count) / denominator)


def _weighted_score(
    *,
    fbeta: float,
    micro_f1: float,
    micro_precision: float,
    cardinality_closeness: float,
    exact_match: float,
    config: ScoringConfig,
) -> float:
    return (
        config.fbeta_weight * fbeta
        + config.micro_f1_weight * micro_f1
        + config.micro_precision_weight * micro_precision
        + config.cardinality_closeness_weight * cardinality_closeness
        + config.exact_match_weight * exact_match
    )


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
