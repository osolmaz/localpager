from __future__ import annotations

from dataclasses import dataclass
from typing import Protocol

from prompt_optimizer.dataset import FeedbackPoolRow


@dataclass(frozen=True)
class ClassifierOutput:
    topics_of_interest: tuple[str, ...]
    description: str
    caveats: tuple[str, ...] = ()
    error: str | None = None


class ClassifierHarness(Protocol):
    """Classifier runtime used by LocalpagerAdapter.

    Production implementations must call `localpager-agent`; tests can provide a
    deterministic fake with the same method shape.
    """

    def classify(self, row: FeedbackPoolRow, prompt_text: str) -> ClassifierOutput:
        ...


@dataclass(frozen=True)
class StaticClassifierHarness:
    predictions: dict[str, tuple[str, ...]]

    def classify(self, row: FeedbackPoolRow, prompt_text: str) -> ClassifierOutput:
        del prompt_text
        return ClassifierOutput(
            topics_of_interest=self.predictions.get(row.ds4.id, ()),
            description="static test prediction",
        )
