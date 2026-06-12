from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from typing import Any

try:
    from gepa.core.adapter import EvaluationBatch
except ImportError:  # pragma: no cover - exercised only without the optional venv dependency.
    @dataclass
    class EvaluationBatch:  # type: ignore[no-redef]
        outputs: list[Any]
        scores: list[float]
        trajectories: list[Any] | None = None
        objective_scores: list[dict[str, float]] | None = None

from prompt_optimizer.dataset import FeedbackPoolRow
from prompt_optimizer.harness import ClassifierHarness, ClassifierOutput
from prompt_optimizer.metric import InvalidLabelError, RowScore, asi_notes, score_row
from prompt_optimizer.prompt import PromptParts

ROUTING_POLICY_COMPONENT = "routing_policy"


@dataclass(frozen=True)
class LocalpagerTrajectory:
    row_id: str
    target: str
    title: str
    gold_topics: tuple[str, ...]
    predicted_topics: tuple[str, ...]
    score: RowScore | None
    feedback: tuple[str, ...]
    error: str | None = None


class LocalpagerAdapter:
    """GEPA adapter that scores candidate routing policy text against DS4 rows."""

    propose_new_texts = None

    def __init__(
        self,
        *,
        prompt_parts: PromptParts,
        harness: ClassifierHarness,
        allowed_topics: frozenset[str],
    ) -> None:
        self.prompt_parts = prompt_parts
        self.harness = harness
        self.allowed_topics = allowed_topics

    def evaluate(
        self,
        batch: list[FeedbackPoolRow],
        candidate: dict[str, str],
        capture_traces: bool = False,
    ) -> EvaluationBatch:
        routing_policy = _candidate_routing_policy(candidate)
        prompt_text = self.prompt_parts.assemble(routing_policy)
        outputs: list[ClassifierOutput] = []
        scores: list[float] = []
        trajectories: list[LocalpagerTrajectory] | None = [] if capture_traces else None
        objective_scores: list[dict[str, float]] = []

        for row in batch:
            output = self.harness.classify(row, prompt_text)
            outputs.append(output)
            row_score: RowScore | None = None
            feedback: tuple[str, ...]
            error = output.error
            if error is None:
                try:
                    row_score = score_row(
                        row.ds4.topics_of_interest,
                        output.topics_of_interest,
                        self.allowed_topics,
                    )
                    score = row_score.score
                    feedback = asi_notes(row.ds4.id, row_score)
                except InvalidLabelError as exc:
                    error = str(exc)
                    score = 0.0
                    feedback = (f"{row.ds4.id}: classifier emitted invalid label: {exc}",)
            else:
                score = 0.0
                feedback = (f"{row.ds4.id}: classifier failed: {error}",)
            scores.append(score)
            objective_scores.append({"weighted_score": score})
            if trajectories is not None:
                trajectories.append(
                    LocalpagerTrajectory(
                        row_id=row.ds4.id,
                        target=row.ds4.url,
                        title=row.ds4.title,
                        gold_topics=row.ds4.topics_of_interest,
                        predicted_topics=output.topics_of_interest,
                        score=row_score,
                        feedback=feedback,
                        error=error,
                    )
                )
        return EvaluationBatch(
            outputs=outputs,
            scores=scores,
            trajectories=trajectories,
            objective_scores=objective_scores,
        )

    def make_reflective_dataset(
        self,
        candidate: dict[str, str],
        eval_batch: EvaluationBatch,
        components_to_update: list[str],
    ) -> Mapping[str, Sequence[Mapping[str, Any]]]:
        _candidate_routing_policy(candidate)
        if ROUTING_POLICY_COMPONENT not in components_to_update:
            return {}
        if eval_batch.trajectories is None:
            return {ROUTING_POLICY_COMPONENT: []}
        records: list[Mapping[str, Any]] = []
        for trajectory in eval_batch.trajectories:
            if not isinstance(trajectory, LocalpagerTrajectory):
                continue
            if not trajectory.feedback:
                continue
            records.append(
                {
                    "Inputs": {
                        "target": trajectory.target,
                        "title": trajectory.title,
                        "gold_topics": list(trajectory.gold_topics),
                    },
                    "Generated Outputs": {
                        "topics_of_interest": list(trajectory.predicted_topics),
                    },
                    "Feedback": "\n".join(trajectory.feedback),
                    "score": trajectory.score.score if trajectory.score is not None else 0.0,
                    "error": trajectory.error,
                }
            )
        return {ROUTING_POLICY_COMPONENT: records}


def _candidate_routing_policy(candidate: dict[str, str]) -> str:
    routing_policy = candidate.get(ROUTING_POLICY_COMPONENT)
    if not isinstance(routing_policy, str) or routing_policy.strip() == "":
        raise ValueError(f"candidate must include non-empty `{ROUTING_POLICY_COMPONENT}`")
    return routing_policy
