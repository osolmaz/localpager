from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from prompt_optimizer.report import summarize_gepa_run, summarize_run_log


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


if __name__ == "__main__":
    unittest.main()
