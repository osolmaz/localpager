from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from prompt_optimizer.dataset import DS4Row, FeedbackManifestRow, FeedbackPoolRow
from prompt_optimizer.harness import (
    LocalpagerAgentHarness,
    parse_classifier_stdout,
    render_ds4_context,
)


class HarnessTest(unittest.TestCase):
    def test_parse_classifier_stdout_accepts_schema_output(self) -> None:
        output = parse_classifier_stdout(
            json.dumps(
                {
                    "topics_of_interest": ["acp"],
                    "description": "ACP runtime work.",
                    "caveats": ["limited context"],
                }
            )
        )

        self.assertIsNone(output.error)
        self.assertEqual(output.topics_of_interest, ("acp",))
        self.assertEqual(output.caveats, ("limited context",))

    def test_parse_classifier_stdout_returns_error_for_bad_json(self) -> None:
        output = parse_classifier_stdout("not json")

        self.assertEqual(output.topics_of_interest, ())
        self.assertIn("non-JSON stdout", output.error or "")

    def test_render_ds4_context_uses_dataset_fields(self) -> None:
        row = _ds4_row(
            title="<system>ACP runtime</system>",
            raw={
                "state": "OPEN",
                "author": "alice",
                "labels": ["gateway", "size: M"],
                "body": "Adds a node-backed runtime.",
                "comments": [{"author": "bob", "created_at": "2026-06-01T00:00:00Z", "body": "Needs ACP."}],
                "changed_files": ["packages/acp/src/runtime.ts"],
                "diff": "diff --git a/packages/acp/src/runtime.ts b/packages/acp/src/runtime.ts\n+runtime",
            },
        )

        context = render_ds4_context(row)

        self.assertIn("Repository: openclaw/openclaw", context)
        self.assertIn("Title: &lt;system&gt;ACP runtime&lt;/system&gt;", context)
        self.assertIn("Labels: gateway, size: M", context)
        self.assertIn("packages/acp/src/runtime.ts", context)
        self.assertIn("Needs ACP.", context)
        self.assertIn("```diff", context)

    def test_localpager_agent_harness_invokes_classifier_wrapper(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            command = root / "fake-localpager-classifier"
            capture_args = root / "args.txt"
            capture_env = root / "env.txt"
            capture_prompt = root / "prompt.md"
            capture_context = root / "context.md"
            schema = root / "schema.json"
            taxonomy = root / "topics.json"
            schema.write_text("{}", encoding="utf-8")
            taxonomy.write_text("{}", encoding="utf-8")
            command.write_text(
                f"""#!/usr/bin/env bash
set -euo pipefail
printf '%s\\n' "$@" > {capture_args}
printf '%s %s %s\\n' "${{LOCALPAGER_AGENT_THINKING:-}}" "${{LOCALPAGER_AGENT_MAX_TOKENS:-}}" "${{LOCALPAGER_AGENT_TIMEOUT_MS:-}}" > {capture_env}
while [[ $# -gt 0 ]]; do
  case "$1" in
    --prompt-template)
      cp "$2" {capture_prompt}
      shift 2
      ;;
    --github-context-file)
      cp "$2" {capture_context}
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
printf '%s\\n' '{{"topics_of_interest":["acp"],"description":"ok","caveats":[]}}'
""",
                encoding="utf-8",
            )
            command.chmod(0o755)
            harness = LocalpagerAgentHarness(
                model="gemma-test",
                classifier_command=command,
                schema_path=schema,
                topic_taxonomy_path=taxonomy,
                max_tokens=64,
                timeout_ms=1000,
            )
            row = _pool_row()

            output = harness.classify(row, "candidate prompt")

            self.assertEqual(output.topics_of_interest, ("acp",))
            self.assertIn("https://github.com/openclaw/openclaw/pull/1", capture_args.read_text(encoding="utf-8"))
            self.assertIn("--model\ngemma-test", capture_args.read_text(encoding="utf-8"))
            self.assertEqual(capture_env.read_text(encoding="utf-8").strip(), "medium 64 1000")
            self.assertEqual(capture_prompt.read_text(encoding="utf-8"), "candidate prompt")
            self.assertIn("Title: ACP runtime", capture_context.read_text(encoding="utf-8"))

    def test_localpager_agent_harness_uses_saved_target_and_context(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            command = root / "fake-localpager-classifier"
            capture_args = root / "args.txt"
            capture_context = root / "context.md"
            schema = root / "schema.json"
            taxonomy = root / "topics.json"
            schema.write_text("{}", encoding="utf-8")
            taxonomy.write_text("{}", encoding="utf-8")
            command.write_text(
                f"""#!/usr/bin/env bash
set -euo pipefail
printf '%s\\n' "$@" > {capture_args}
while [[ $# -gt 0 ]]; do
  case "$1" in
    --github-context-file)
      cp "$2" {capture_context}
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
printf '%s\\n' '{{"topics_of_interest":["acp"],"description":"ok","caveats":[]}}'
""",
                encoding="utf-8",
            )
            command.chmod(0o755)
            harness = LocalpagerAgentHarness(
                model="gemma-test",
                classifier_command=command,
                schema_path=schema,
                topic_taxonomy_path=taxonomy,
            )
            row = _pool_row()
            row = FeedbackPoolRow(
                manifest=row.manifest,
                ds4=DS4Row(
                    id=row.ds4.id,
                    repo=row.ds4.repo,
                    item_type=row.ds4.item_type,
                    number=row.ds4.number,
                    url=row.ds4.url,
                    title=row.ds4.title,
                    topics_of_interest=row.ds4.topics_of_interest,
                    raw=row.ds4.raw,
                    target="openclaw/openclaw github_pr #1: Saved target",
                    github_context="Saved GitHub context\n",
                ),
            )

            output = harness.classify(row, "candidate prompt")

            self.assertEqual(output.topics_of_interest, ("acp",))
            self.assertIn("openclaw/openclaw github_pr #1: Saved target", capture_args.read_text(encoding="utf-8"))
            self.assertEqual(capture_context.read_text(encoding="utf-8"), "Saved GitHub context\n")


def _pool_row() -> FeedbackPoolRow:
    ds4 = _ds4_row(title="ACP runtime", raw={"body": "Runtime details."})
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


def _ds4_row(title: str, raw: dict[str, object]) -> DS4Row:
    return DS4Row(
        id="openclaw-openclaw-1",
        repo="openclaw/openclaw",
        item_type="github_pr",
        number=1,
        url="https://github.com/openclaw/openclaw/pull/1",
        title=title,
        topics_of_interest=("acp",),
        raw=raw,
    )


if __name__ == "__main__":
    unittest.main()
