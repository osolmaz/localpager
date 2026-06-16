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
    DEFAULT_BASE_URL,
    DEFAULT_CONCURRENCY,
    DEFAULT_MAX_TOKENS,
    DEFAULT_MODEL,
    GEPARunConfig,
    HarnessConfig,
    OptimizerInputs,
    evaluate_routing_policy,
    evaluate_seed,
    seed_candidate,
    write_result_artifacts,
)


class RunTest(unittest.TestCase):
    def test_live_harness_defaults_use_qwen_c4_profile(self) -> None:
        config = HarnessConfig()

        self.assertEqual(DEFAULT_MODEL, "nvidia/Qwen3.6-35B-A3B-NVFP4")
        self.assertEqual(DEFAULT_BASE_URL, "http://127.0.0.1:8000/v1")
        self.assertEqual(DEFAULT_CONCURRENCY, 4)
        self.assertEqual(config.model, DEFAULT_MODEL)
        self.assertEqual(config.base_url, DEFAULT_BASE_URL)
        self.assertEqual(config.concurrency, 4)
        self.assertEqual(config.thinking, "medium")
        self.assertEqual(config.max_tokens, 8192)
        self.assertEqual(DEFAULT_MAX_TOKENS, 8192)

    def test_evaluate_seed_reports_scores(self) -> None:
        inputs = _inputs()

        report = evaluate_seed(
            inputs=inputs,
            harness=StaticClassifierHarness({"row-1": ("acp",)}),
            concurrency=2,
        )

        self.assertEqual(report["rows"], 1)
        self.assertEqual(report["candidate"], "seed")
        self.assertEqual(report["mean_score"], 1.0)
        self.assertIn("routing_policy_sha256", report)
        self.assertEqual(report["row_reports"][0]["gold_topics"], ["acp"])
        self.assertEqual(report["row_reports"][0]["predicted_topics"], ["acp"])
        self.assertEqual(report["row_reports"][0]["true_positives"], ["acp"])
        self.assertEqual(report["row_reports"][0]["false_positives"], [])
        self.assertEqual(report["row_reports"][0]["false_negatives"], [])
        self.assertEqual(report["row_reports"][0]["loss"], 0.0)

    def test_evaluate_routing_policy_reports_candidate_name(self) -> None:
        inputs = _inputs()

        report = evaluate_routing_policy(
            inputs=inputs,
            routing_policy="## Goal\nNew policy\n",
            harness=StaticClassifierHarness({"row-1": ("acp",)}),
            candidate_name="gepa-test",
        )

        self.assertEqual(report["candidate"], "gepa-test")
        self.assertEqual(report["mean_score"], 1.0)

    def test_evaluate_seed_supports_offset(self) -> None:
        prompt_parts = _prompt_parts()
        inputs = OptimizerInputs(
            rows=(_pool_row("row-1", "ACP runtime"), _pool_row("row-2", "ACP follow-up")),
            allowed_topics=frozenset({"acp"}),
            prompt_parts=prompt_parts,
        )

        report = evaluate_seed(
            inputs=inputs,
            harness=StaticClassifierHarness({"row-2": ("acp",)}),
            limit=1,
            offset=1,
        )

        self.assertEqual(report["rows"], 1)
        self.assertEqual(report["row_reports"][0]["id"], "row-2")

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
                    max_candidate_proposals=16,
                    seed_routing_policy="## Goal\nBetter policy\n",
                    harness=HarnessConfig(model="gemma-test", concurrency=2),
                ),
            )

            self.assertEqual(summary["best_score"], 0.75)
            self.assertTrue((output_dir / "best.prompt.md").exists())
            self.assertTrue((output_dir / "best.routing_policy.md").exists())
            saved_summary = json.loads((output_dir / "summary.json").read_text(encoding="utf-8"))
            self.assertEqual(saved_summary["config"]["harness"]["model"], "gemma-test")
            self.assertEqual(saved_summary["config"]["max_candidate_proposals"], 16)
            self.assertEqual(saved_summary["config"]["seed_routing_policy_chars"], 22)
            self.assertIsNotNone(saved_summary["config"]["seed_routing_policy_sha256"])
            self.assertIn("Better policy", (output_dir / "best.prompt.md").read_text(encoding="utf-8"))


def _inputs() -> OptimizerInputs:
    return OptimizerInputs(
        rows=(_pool_row(),),
        allowed_topics=frozenset({"acp"}),
        prompt_parts=_prompt_parts(),
    )


def _prompt_parts():
    return split_seed_prompt(
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


def _pool_row(row_id: str = "row-1", title: str = "ACP runtime") -> FeedbackPoolRow:
    ds4 = DS4Row(
        id=row_id,
        repo="openclaw/openclaw",
        item_type="github_pr",
        number=1,
        url="https://github.com/openclaw/openclaw/pull/1",
        title=title,
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
