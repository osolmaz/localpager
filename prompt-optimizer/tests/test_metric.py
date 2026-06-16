from __future__ import annotations

import unittest

from prompt_optimizer.metric import InvalidLabelError, asi_notes, score_batch, score_row


class MetricTest(unittest.TestCase):
    def test_random_extra_label_hurts(self) -> None:
        allowed = {"acp", "gateway", "reliability"}
        exact = score_row(["acp"], ["acp"], allowed)
        extra = score_row(["acp"], ["acp", "gateway"], allowed)

        self.assertEqual(exact.loss, 0.0)
        self.assertGreater(extra.loss, exact.loss)
        self.assertLess(extra.score, exact.score)
        self.assertAlmostEqual(extra.precision, 0.5)
        self.assertAlmostEqual(extra.cardinality_closeness, 0.5)
        self.assertEqual(extra.false_positives, ("gateway",))
        self.assertEqual(extra.over_label_count, 1)

    def test_false_positive_costs_more_than_false_negative(self) -> None:
        allowed = {"acp", "gateway"}
        false_positive = score_row(["acp"], ["acp", "gateway"], allowed)
        false_negative = score_row(["acp", "gateway"], ["acp"], allowed)

        self.assertGreater(false_positive.loss, false_negative.loss)
        self.assertLess(false_positive.score, false_negative.score)

    def test_invalid_predicted_label_fails(self) -> None:
        with self.assertRaisesRegex(InvalidLabelError, "not in taxonomy"):
            score_row(["acp"], ["acp", "random_label"], {"acp"})

    def test_batch_reports_micro_f1(self) -> None:
        batch = score_batch(
            gold_rows=[["acp", "gateway"], ["reliability"]],
            predicted_rows=[["acp"], ["reliability", "gateway"]],
            allowed_topics={"acp", "gateway", "reliability"},
        )

        self.assertEqual(batch.rows[0].false_negatives, ("gateway",))
        self.assertEqual(batch.rows[1].false_positives, ("gateway",))
        self.assertAlmostEqual(batch.micro_precision, 2 / 3)
        self.assertAlmostEqual(batch.micro_recall, 2 / 3)
        self.assertAlmostEqual(batch.micro_f1, 2 / 3)
        self.assertAlmostEqual(batch.micro_fbeta, 2 / 3)

    def test_empty_gold_and_empty_prediction_is_perfect(self) -> None:
        row = score_row([], [], {"acp"})

        self.assertEqual(row.score, 1.0)
        self.assertEqual(row.loss, 0.0)
        self.assertEqual(row.exact_match, 1.0)
        self.assertEqual(row.cardinality_closeness, 1.0)

    def test_asi_notes_surface_label_spam(self) -> None:
        row = score_row(["acp"], ["acp", "gateway"], {"acp", "gateway"})

        self.assertEqual(
            asi_notes("row-1", row),
            ("row-1: predicted `gateway` but gold does not include it; treat as label spam unless central.",),
        )


if __name__ == "__main__":
    unittest.main()
