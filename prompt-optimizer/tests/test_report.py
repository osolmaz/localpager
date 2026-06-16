from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from prompt_optimizer.report import (
    render_gepa_run_report,
    summarize_evaluation_report,
    summarize_gepa_run,
    summarize_run_log,
    write_gepa_run_report,
)


class ReportTest(unittest.TestCase):
    def test_summarize_run_log_counts_proposals(self) -> None:
        summary = summarize_run_log(
            "\n".join(
                [
                    "Iteration 0: Base program full valset score: 0.5 over 30 / 30 examples",
                    "Iteration 1: Selected program 0 score: 0.5",
                    "Iteration 1: Proposed new text for routing_policy: ...",
                    "Iteration 1: New subsample score 1.0 is not better than old score 2.0, skipping",
                    "Iteration 2: Selected program 0 score: 0.5",
                    "Iteration 2: Proposed new text for routing_policy: ...",
                    "Iteration 2: New subsample score 3.0 is better than old score 2.0. Continue to full eval and add to candidate pool.",
                    "Iteration 2: Found a better program on the valset with score 0.7.",
                ]
            )
        )

        self.assertEqual(summary["base_score"], 0.5)
        self.assertEqual(summary["proposal_attempts"], 2)
        self.assertEqual(summary["proposal_texts_started"], 2)
        self.assertEqual(summary["accepted_full_eval_candidates"], 1)
        self.assertEqual(summary["rejected_candidates"], 1)
        self.assertEqual(summary["better_valset_events"], [{"iteration": 2, "score": 0.7}])

    def test_summarize_gepa_run_includes_result_when_present(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            run_dir = Path(tmp)
            (run_dir / "run_log.txt").write_text(
                "Iteration 0: Base program full valset score: 0.4 over 2 / 2 examples\n",
                encoding="utf-8",
            )
            (run_dir / "gepa-result.json").write_text(
                json.dumps(
                    {
                        "best_idx": 1,
                        "total_metric_calls": 12,
                        "num_full_val_evals": 2,
                        "candidates": [{"routing_policy": "a"}, {"routing_policy": "b"}],
                        "val_aggregate_scores": [0.4, 0.6],
                    }
                ),
                encoding="utf-8",
            )

            summary = summarize_gepa_run(run_dir)

            self.assertEqual(summary["run_log"]["base_score"], 0.4)
            self.assertEqual(summary["result"]["best_idx"], 1)
            self.assertEqual(summary["result"]["num_candidates"], 2)
            self.assertEqual(summary["result"]["val_aggregate_scores"], [0.4, 0.6])

    def test_render_gepa_run_report_includes_score_charts(self) -> None:
        html = render_gepa_run_report(
            {
                "run_dir": "prompt-optimizer/out/example",
                "run_log": {
                    "base_score": 0.4,
                    "proposal_attempts": 2,
                    "accepted_full_eval_candidates": 1,
                    "rejected_candidates": 1,
                    "selected_events": [
                        {"iteration": 1, "candidate_idx": 0, "score": 0.4},
                        {"iteration": 2, "candidate_idx": 1, "score": 0.6},
                    ],
                    "proposal_events": [
                        {
                            "iteration": 1,
                            "old_subsample_sum": 2.0,
                            "new_subsample_sum": 1.0,
                            "delta": -1.0,
                            "accepted_for_full_eval": False,
                        },
                        {
                            "iteration": 2,
                            "old_subsample_sum": 2.0,
                            "new_subsample_sum": 3.0,
                            "delta": 1.0,
                            "accepted_for_full_eval": True,
                        },
                    ],
                    "better_valset_events": [{"iteration": 2, "score": 0.6}],
                },
                "result": {
                    "best_idx": 1,
                    "total_metric_calls": 12,
                    "num_candidates": 2,
                    "val_aggregate_scores": [0.4, 0.6],
                },
            }
        )

        self.assertIn("Validation Score Over Iterations", html)
        self.assertIn("Proposal Subsample Delta", html)
        self.assertIn("Final Candidate Scores", html)
        self.assertIn("accepted", html)
        self.assertIn("rejected", html)

    def test_write_gepa_run_report_writes_default_html_path(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            run_dir = Path(tmp)
            (run_dir / "run_log.txt").write_text(
                "\n".join(
                    [
                        "Iteration 0: Base program full valset score: 0.4 over 2 / 2 examples",
                        "Iteration 1: Selected program 0 score: 0.4",
                    ]
                ),
                encoding="utf-8",
            )

            output_path = write_gepa_run_report(run_dir)

            self.assertEqual(output_path, run_dir / "score_report.html")
            self.assertIn("Validation Score Over Iterations", output_path.read_text(encoding="utf-8"))

    def test_summarize_evaluation_report_counts_final_metrics(self) -> None:
        summary = summarize_evaluation_report(
            {
                "candidate": "candidate-a",
                "mean_score": 0.4,
                "row_reports": [
                    {
                        "gold_topics": ["acp"],
                        "predicted_topics": ["acp"],
                        "error": None,
                    },
                    {
                        "gold_topics": ["security"],
                        "predicted_topics": ["security", "config"],
                        "error": None,
                    },
                    {
                        "gold_topics": ["gateway", "sessions"],
                        "predicted_topics": [],
                        "error": "final_json was not called",
                    },
                ],
            }
        )

        self.assertEqual(summary["candidate"], "candidate-a")
        self.assertEqual(summary["rows"], 3)
        self.assertEqual(summary["true_positives"], 2)
        self.assertEqual(summary["false_positives"], 1)
        self.assertEqual(summary["false_negatives"], 2)
        self.assertAlmostEqual(summary["precision"], 2 / 3)
        self.assertAlmostEqual(summary["recall"], 0.5)
        self.assertAlmostEqual(summary["micro_f1"], 4 / 7)
        self.assertEqual(summary["exact_matches"], 1)
        self.assertEqual(summary["over_label_events"], 1)
        self.assertEqual(summary["over_label_total"], 1)
        self.assertEqual(summary["structural_failures"], 1)
        self.assertAlmostEqual(summary["mean_gold_labels"], 4 / 3)
        self.assertEqual(summary["mean_predicted_labels"], 1.0)
        self.assertEqual(summary["per_topic"]["config"]["false_positives"], 1)


if __name__ == "__main__":
    unittest.main()
