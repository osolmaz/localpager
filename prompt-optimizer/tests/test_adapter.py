from __future__ import annotations

import unittest

from prompt_optimizer.adapter import LocalpagerAdapter, ROUTING_POLICY_COMPONENT
from prompt_optimizer.dataset import DS4Row, FeedbackManifestRow, FeedbackPoolRow
from prompt_optimizer.harness import StaticClassifierHarness
from prompt_optimizer.prompt import normalize_template_variables, split_seed_prompt


class AdapterTest(unittest.TestCase):
    def test_evaluate_scores_outputs_and_builds_reflective_dataset(self) -> None:
        row = _pool_row("row-1", ["acp"], "A gateway false positive")
        prompt_parts = split_seed_prompt(
            normalize_template_variables(
                """Allowed
{{{allowed_topics_json}}}
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
        adapter = LocalpagerAdapter(
            prompt_parts=prompt_parts,
            harness=StaticClassifierHarness({"row-1": ("acp", "gateway")}),
            allowed_topics=frozenset({"acp", "gateway"}),
        )

        batch = adapter.evaluate(
            [row],
            {ROUTING_POLICY_COMPONENT: "## Goal\nNew policy\n"},
            capture_traces=True,
        )

        self.assertEqual(len(batch.outputs), 1)
        self.assertLess(batch.scores[0], 1.0)
        self.assertEqual(batch.outputs[0].topics_of_interest, ("acp", "gateway"))
        reflective = adapter.make_reflective_dataset(
            {ROUTING_POLICY_COMPONENT: "## Goal\nNew policy\n"},
            batch,
            [ROUTING_POLICY_COMPONENT],
        )
        records = list(reflective[ROUTING_POLICY_COMPONENT])
        self.assertEqual(len(records), 1)
        self.assertIn("label spam", records[0]["Feedback"])
        self.assertEqual(records[0]["Inputs"]["gold_topics"], ["acp"])

    def test_invalid_prediction_gets_zero_score_without_raising(self) -> None:
        row = _pool_row("row-1", ["acp"], "Invalid label")
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
        adapter = LocalpagerAdapter(
            prompt_parts=prompt_parts,
            harness=StaticClassifierHarness({"row-1": ("not_allowed",)}),
            allowed_topics=frozenset({"acp"}),
        )

        batch = adapter.evaluate(
            [row],
            {ROUTING_POLICY_COMPONENT: "## Goal\nNew policy\n"},
            capture_traces=True,
        )

        self.assertEqual(batch.scores, [0.0])
        assert batch.trajectories is not None
        self.assertIn("invalid label", batch.trajectories[0].feedback[0])


def _pool_row(row_id: str, topics: list[str], title: str) -> FeedbackPoolRow:
    ds4 = DS4Row(
        id=row_id,
        repo="openclaw/openclaw",
        item_type="github_pr",
        number=1,
        url="https://github.com/openclaw/openclaw/pull/1",
        title=title,
        topics_of_interest=tuple(topics),
        raw={},
    )
    manifest = FeedbackManifestRow(
        id=row_id,
        source_set="gepa-good-60",
        audit_bucket="stratified",
        repo=ds4.repo,
        item_type=ds4.item_type,
        number=ds4.number,
        url=ds4.url,
        title=title,
        ds4_topics=tuple(topics),
        raw={},
    )
    return FeedbackPoolRow(manifest=manifest, ds4=ds4)


if __name__ == "__main__":
    unittest.main()
