from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from prompt_optimizer.adapter import ROUTING_POLICY_COMPONENT
from prompt_optimizer.dataset import DS4Row, FeedbackManifestRow, FeedbackPoolRow
from prompt_optimizer.harness import StaticClassifierHarness
from prompt_optimizer.prompt import normalize_template_variables, split_seed_prompt
from prompt_optimizer.run import (
    GEPARunConfig,
    HarnessConfig,
    OptimizerInputs,
    evaluate_seed,
    seed_candidate,
    write_result_artifacts,
)


class RunTest(unittest.TestCase):
    def test_evaluate_seed_reports_scores(self) -> None:
        inputs = _inputs()

        report = evaluate_seed(
            inputs=inputs,
            harness=StaticClassifierHarness({"row-1": ("acp",)}),
            concurrency=2,
        )

        self.assertEqual(report["rows"], 1)
        self.assertEqual(report["mean_score"], 1.0)
        self.assertEqual(report["row_reports"][0]["gold_topics"], ["acp"])
        self.assertEqual(report["row_reports"][0]["predicted_topics"], ["acp"])

    def test_seed_candidate_uses_routing_policy_component(self) -> None:
        inputs = _inputs()

        candidate = seed_candidate(inputs.prompt_parts)

        self.assertEqual(set(candidate), {ROUTING_POLICY_COMPONENT})
        self.assertIn("## Goal", candidate[ROUTING_POLICY_COMPONENT])

    def test_write_result_artifacts_writes_best_prompt(self) -> None:
        inputs = _inputs()
        with tempfile.TemporaryDirectory() as tmp:
            output_dir = Path(tmp)
            summary = write_result_artifacts(
                _FakeResult(),
                inputs.prompt_parts,
                output_dir,
                GEPARunConfig(
                    output_dir=output_dir,
                    max_metric_calls=2,
                    harness=HarnessConfig(model="gemma-test", concurrency=2),
                ),
            )

            self.assertEqual(summary["best_score"], 0.75)
            self.assertTrue((output_dir / "best.prompt.md").exists())
            self.assertTrue((output_dir / "best.routing_policy.md").exists())
            saved_summary = json.loads((output_dir / "summary.json").read_text(encoding="utf-8"))
            self.assertEqual(saved_summary["config"]["harness"]["model"], "gemma-test")
            self.assertIn("Better policy", (output_dir / "best.prompt.md").read_text(encoding="utf-8"))


def _inputs() -> OptimizerInputs:
    prompt_parts = split_seed_prompt(
        normalize_template_variables(
            """{{{allowed_topics_json}}}
{{{topic_descriptions}}}

## Goal
Old policy

## Target
{{target}}

## GitHub Context
{{{github_context}}}
"""
        )
    )
    return OptimizerInputs(
        rows=(_pool_row(),),
        allowed_topics=frozenset({"acp"}),
        prompt_parts=prompt_parts,
    )


def _pool_row() -> FeedbackPoolRow:
    ds4 = DS4Row(
        id="row-1",
        repo="openclaw/openclaw",
        item_type="github_pr",
        number=1,
        url="https://github.com/openclaw/openclaw/pull/1",
        title="ACP runtime",
        topics_of_interest=("acp",),
        raw={},
    )
    manifest = FeedbackManifestRow(
        id=ds4.id,
        source_set="gepa-good-60",
        audit_bucket="stratified",
        repo=ds4.repo,
        item_type=ds4.item_type,
        number=ds4.number,
        url=ds4.url,
        title=ds4.title,
        ds4_topics=ds4.topics_of_interest,
        raw={},
    )
    return FeedbackPoolRow(manifest=manifest, ds4=ds4)


class _FakeResult:
    best_candidate = {ROUTING_POLICY_COMPONENT: "## Goal\nBetter policy\n"}
    best_idx = 0
    val_aggregate_scores = [0.75]
    total_metric_calls = 2
    num_full_val_evals = 1
    num_candidates = 1

    def to_dict(self) -> dict[str, object]:
        return {"fake": True}


if __name__ == "__main__":
    unittest.main()
