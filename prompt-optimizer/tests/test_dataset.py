from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from prompt_optimizer.dataset import DatasetError, build_evalstate_pool, load_taxonomy


class DatasetTest(unittest.TestCase):
    def test_load_taxonomy_supports_dict_shape(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "topics.json"
            path.write_text(json.dumps({"topics": {"acp": {}, "gateway": {}}}), encoding="utf-8")
            self.assertEqual(load_taxonomy(path), frozenset({"acp", "gateway"}))

    def test_build_evalstate_pool_uses_expected_topics_and_saved_context(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            taxonomy = root / "topics.json"
            split = root / "feedback300.jsonl"
            taxonomy.write_text(json.dumps({"topics": {"acp": {}, "gateway": {}}}), encoding="utf-8")
            split.write_text(
                json.dumps(
                    {
                        "id": "openclaw-openclaw-1",
                        "target": "openclaw/openclaw github_pr #1: Gateway ACP work",
                        "github_context": "\n".join(
                            [
                                "GitHub item:",
                                "- Repository: openclaw/openclaw",
                                "- Type: github_pr",
                                "- Number: 1",
                                "- URL: https://github.com/openclaw/openclaw/pull/1",
                                "- Title: Gateway ACP work",
                            ]
                        ),
                        "expected_topics": ["acp", "gateway"],
                        "expected_topics_json": "[\"acp\", \"gateway\"]",
                        "keywords": [],
                        "title": "Gateway ACP work",
                    }
                )
                + "\n",
                encoding="utf-8",
            )

            pool = build_evalstate_pool(split, taxonomy, split_name="feedback")

        self.assertEqual(pool.source_row_count, 1)
        self.assertEqual(pool.composition, {"feedback": 1})
        self.assertEqual(pool.rows[0].item.topics_of_interest, ("acp", "gateway"))
        self.assertEqual(pool.rows[0].item.target, "openclaw/openclaw github_pr #1: Gateway ACP work")
        self.assertEqual(pool.rows[0].item.url, "https://github.com/openclaw/openclaw/pull/1")
        self.assertIn("GitHub item:", pool.rows[0].item.github_context or "")

    def test_build_evalstate_pool_rejects_topics_outside_taxonomy(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            taxonomy = root / "topics.json"
            split = root / "feedback300.jsonl"
            taxonomy.write_text(json.dumps({"topics": {"acp": {}}}), encoding="utf-8")
            split.write_text(
                json.dumps(
                    {
                        "id": "openclaw-openclaw-1",
                        "target": "openclaw/openclaw github_pr #1: Gateway ACP work",
                        "github_context": "GitHub item:\n- URL: https://github.com/openclaw/openclaw/pull/1",
                        "expected_topics": ["gateway"],
                        "title": "Gateway ACP work",
                    }
                )
                + "\n",
                encoding="utf-8",
            )

            with self.assertRaisesRegex(DatasetError, "topic not in taxonomy: gateway"):
                build_evalstate_pool(split, taxonomy, split_name="feedback")


if __name__ == "__main__":
    unittest.main()
