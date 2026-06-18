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
    row_jaccard_weight: float = 0.60
    row_topic_f1_weight: float = 0.20
    row_exact_weight: float = 0.20
    max_predicted_labels: int = 3
    over_cardinality_penalty: float = 0.10
    duplicate_label_penalty: float = 0.05


@dataclass(frozen=True)
class RowScore:
    gold: tuple[str, ...]
    predicted: tuple[str, ...]
    true_positives: tuple[str, ...]
    false_positives: tuple[str, ...]
    false_negatives: tuple[str, ...]
    over_label_count: int
    duplicate_label_count: int
    over_cardinality_count: int
    precision: float
    recall: float
    f1: float
    row_jaccard: float
    row_topic_f1: float
    row_exact: float
    policy_penalty: float
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
    avg_row_jaccard: float
    avg_row_topic_f1: float
    avg_row_exact: float
    avg_policy_penalty: float
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
    predicted_topics, duplicate_label_count = _normal_predicted_label_tuple(predicted, allowed)

    gold_set = set(gold_topics)
    predicted_set = set(predicted_topics)
    true_positives = tuple(topic for topic in predicted_topics if topic in gold_set)
    false_positives = tuple(topic for topic in predicted_topics if topic not in gold_set)
    false_negatives = tuple(topic for topic in gold_topics if topic not in predicted_set)
    over_label_count = max(0, len(predicted_topics) - len(gold_topics))
    over_cardinality_count = max(0, len(predicted_topics) - config.max_predicted_labels)
    precision = _label_precision(len(true_positives), len(false_positives), len(gold_topics))
    recall = _label_recall(len(true_positives), len(false_negatives))
    row_topic_f1 = _f1(precision, recall)
    row_jaccard = _jaccard(
        true_positives=len(true_positives),
        false_positives=len(false_positives),
        false_negatives=len(false_negatives),
    )
    row_exact = 1.0 if predicted_set == gold_set else 0.0
    policy_penalty = (
        config.over_cardinality_penalty * over_cardinality_count
        + config.duplicate_label_penalty * duplicate_label_count
    )
    score = _bounded_score(
        _weighted_score(
            row_jaccard=row_jaccard,
            row_topic_f1=row_topic_f1,
            row_exact=row_exact,
            policy_penalty=policy_penalty,
            config=config,
        )
    )
    return RowScore(
        gold=gold_topics,
        predicted=predicted_topics,
        true_positives=true_positives,
        false_positives=false_positives,
        false_negatives=false_negatives,
        over_label_count=over_label_count,
        duplicate_label_count=duplicate_label_count,
        over_cardinality_count=over_cardinality_count,
        precision=precision,
        recall=recall,
        f1=row_topic_f1,
        row_jaccard=row_jaccard,
        row_topic_f1=row_topic_f1,
        row_exact=row_exact,
        policy_penalty=policy_penalty,
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
            avg_row_jaccard=0.0,
            avg_row_topic_f1=0.0,
            avg_row_exact=0.0,
            avg_policy_penalty=0.0,
            per_topic={},
        )
    total_tp = sum(len(row.true_positives) for row in rows)
    total_fp = sum(len(row.false_positives) for row in rows)
    total_fn = sum(len(row.false_negatives) for row in rows)
    total_gold = sum(len(row.gold) for row in rows)
    precision = _label_precision(total_tp, total_fp, total_gold)
    recall = _label_recall(total_tp, total_fn)
    avg_row_jaccard = sum(row.row_jaccard for row in rows) / len(rows)
    avg_row_topic_f1 = sum(row.row_topic_f1 for row in rows) / len(rows)
    avg_row_exact = sum(row.row_exact for row in rows) / len(rows)
    avg_policy_penalty = sum(row.policy_penalty for row in rows) / len(rows)
    score = _bounded_score(
        _weighted_score(
            row_jaccard=avg_row_jaccard,
            row_topic_f1=avg_row_topic_f1,
            row_exact=avg_row_exact,
            policy_penalty=avg_policy_penalty,
            config=config,
        )
    )
    per_topic = _per_topic_stats(rows)
    return BatchScore(
        rows=rows,
        loss=1.0 - score,
        score=score,
        micro_precision=precision,
        micro_recall=recall,
        micro_f1=_f1(precision, recall),
        avg_row_jaccard=avg_row_jaccard,
        avg_row_topic_f1=avg_row_topic_f1,
        avg_row_exact=avg_row_exact,
        avg_policy_penalty=avg_policy_penalty,
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
    if row_score.over_cardinality_count:
        notes.append(f"{row_id}: predicted more than 3 labels; apply the cardinality policy.")
    if row_score.duplicate_label_count:
        notes.append(f"{row_id}: emitted duplicate labels; output each topic at most once.")
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


def _normal_predicted_label_tuple(labels: Iterable[str], allowed_topics: frozenset[str]) -> tuple[tuple[str, ...], int]:
    normal: list[str] = []
    seen: set[str] = set()
    duplicate_count = 0
    for label in labels:
        if not isinstance(label, str) or label == "":
            raise InvalidLabelError("predicted label must be a non-empty string")
        if label not in allowed_topics:
            raise InvalidLabelError(f"predicted label not in taxonomy: {label}")
        if label in seen:
            duplicate_count += 1
            continue
        seen.add(label)
        normal.append(label)
    return tuple(normal), duplicate_count


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


def _jaccard(*, true_positives: int, false_positives: int, false_negatives: int) -> float:
    denominator = true_positives + false_positives + false_negatives
    return true_positives / denominator if denominator else 1.0


def _weighted_score(
    *,
    row_jaccard: float,
    row_topic_f1: float,
    row_exact: float,
    policy_penalty: float,
    config: ScoringConfig,
) -> float:
    return (
        config.row_jaccard_weight * row_jaccard
        + config.row_topic_f1_weight * row_topic_f1
        + config.row_exact_weight * row_exact
        - policy_penalty
    )


def _bounded_score(score: float) -> float:
    return max(0.0, min(1.0, score))


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
