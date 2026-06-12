from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from prompt_optimizer.dataset import DatasetError, build_feedback_pool, load_taxonomy


class DatasetTest(unittest.TestCase):
    def test_build_feedback_pool_uses_canonical_ds4_labels(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            taxonomy = root / "topics.json"
            ds4 = root / "ds4.jsonl"
            manifest = root / "gepa-good-60.rows.jsonl"
            taxonomy.write_text(
                json.dumps(
                    {
                        "topics": {
                            "acp": {"description": "ACP"},
                            "gateway": {"description": "Gateway"},
                            "reliability": {"description": "Reliability"},
                        }
                    }
                ),
                encoding="utf-8",
            )
            ds4.write_text(
                "\n".join(
                    [
                        json.dumps(
                            {
                                "id": "openclaw-openclaw-1",
                                "repo": "openclaw/openclaw",
                                "item_type": "github_pr",
                                "number": 1,
                                "url": "https://github.com/openclaw/openclaw/pull/1",
                                "title": "Gateway ACP work",
                                "topics_of_interest": ["acp", "gateway"],
                            }
                        ),
                        json.dumps(
                            {
                                "id": "openclaw-openclaw-2",
                                "repo": "openclaw/openclaw",
                                "item_type": "github_issue",
                                "number": 2,
                                "url": "https://github.com/openclaw/openclaw/issues/2",
                                "title": "Reliability issue",
                                "topics_of_interest": ["reliability"],
                            }
                        ),
                    ]
                )
                + "\n",
                encoding="utf-8",
            )
            manifest.write_text(
                "\n".join(
                    [
                        json.dumps(
                            {
                                "id": "openclaw-openclaw-1",
                                "source_set": "gepa-good-60",
                                "audit_bucket": "stratified",
                                "repo": "openclaw/openclaw",
                                "item_type": "github_pr",
                                "number": 1,
                                "url": "https://github.com/openclaw/openclaw/pull/1",
                                "title": "Gateway ACP work",
                                "ds4_topics": ["acp", "gateway"],
                                "teacher_topics": ["acp", "gateway", "reliability"],
                                "expected_topics": ["acp", "gateway", "reliability"],
                            }
                        ),
                        json.dumps(
                            {
                                "id": "openclaw-openclaw-2",
                                "source_set": "gepa-good-60",
                                "audit_bucket": "confusion",
                                "repo": "openclaw/openclaw",
                                "item_type": "github_issue",
                                "number": 2,
                                "url": "https://github.com/openclaw/openclaw/issues/2",
                                "title": "Reliability issue",
                                "ds4_topics": ["reliability"],
                                "teacher_topics": ["gateway", "reliability"],
                                "expected_topics": ["gateway", "reliability"],
                            }
                        ),
                    ]
                )
                + "\n",
                encoding="utf-8",
            )

            pool = build_feedback_pool(ds4, manifest, taxonomy)

        self.assertEqual(pool.source_row_count, 2)
        self.assertEqual(pool.composition, {"stratified": 1, "confusion": 1})
        self.assertEqual(pool.rows[0].ds4.topics_of_interest, ("acp", "gateway"))
        self.assertEqual(pool.rows[0].manifest.raw["expected_topics"], ["acp", "gateway", "reliability"])

    def test_rejects_feedback_rows_not_in_canonical_ds4(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            taxonomy = root / "topics.json"
            ds4 = root / "ds4.jsonl"
            manifest = root / "manifest.jsonl"
            taxonomy.write_text(json.dumps({"topics": {"acp": {}}}), encoding="utf-8")
            ds4.write_text("", encoding="utf-8")
            manifest.write_text(
                json.dumps(
                    {
                        "id": "missing",
                        "source_set": "gepa-good-60",
                        "audit_bucket": "random",
                        "repo": "openclaw/openclaw",
                        "item_type": "github_pr",
                        "number": 1,
                        "url": "https://example.test",
                        "title": "Missing",
                        "ds4_topics": [],
                    }
                )
                + "\n",
                encoding="utf-8",
            )

            with self.assertRaisesRegex(DatasetError, "missing from canonical DS4"):
                build_feedback_pool(ds4, manifest, taxonomy)

    def test_load_taxonomy_supports_dict_shape(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "topics.json"
            path.write_text(json.dumps({"topics": {"acp": {}, "gateway": {}}}), encoding="utf-8")
            self.assertEqual(load_taxonomy(path), frozenset({"acp", "gateway"}))


if __name__ == "__main__":
    unittest.main()
