from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from prompt_optimizer.dataset import OptimizerItem, OptimizerManifest, FeedbackPoolRow
from prompt_optimizer.harness import (
    LocalpagerAgentHarness,
    parse_classifier_stdout,
    render_item_context,
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

    def test_render_item_context_uses_dataset_fields(self) -> None:
        row = _optimizer_item(
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

        context = render_item_context(row)

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
                item=OptimizerItem(
                    id=row.item.id,
                    repo=row.item.repo,
                    item_type=row.item.item_type,
                    number=row.item.number,
                    url=row.item.url,
                    title=row.item.title,
                    topics_of_interest=row.item.topics_of_interest,
                    raw=row.item.raw,
                    target="openclaw/openclaw github_pr #1: Saved target",
                    github_context="Saved GitHub context\n",
                ),
            )

            output = harness.classify(row, "candidate prompt")

            self.assertEqual(output.topics_of_interest, ("acp",))
            self.assertIn("openclaw/openclaw github_pr #1: Saved target", capture_args.read_text(encoding="utf-8"))
            self.assertEqual(capture_context.read_text(encoding="utf-8"), "Saved GitHub context\n")

    def test_localpager_agent_harness_state_dir_env_is_absolute(self) -> None:
        harness = LocalpagerAgentHarness(model="gemma-test", state_dir=Path("relative-state-dir"))

        env = harness._env()

        self.assertTrue(Path(env["LOCALPAGER_CLASSIFIER_STATE_DIR"]).is_absolute())
        self.assertTrue(env["LOCALPAGER_CLASSIFIER_STATE_DIR"].endswith("relative-state-dir"))


def _pool_row() -> FeedbackPoolRow:
    item = _optimizer_item(title="ACP runtime", raw={"body": "Runtime details."})
    manifest = OptimizerManifest(
        id=item.id,
        source_set="unit-test",
        audit_bucket="stratified",
        repo=item.repo,
        item_type=item.item_type,
        number=item.number,
        url=item.url,
        title=item.title,
        gold_topics=item.topics_of_interest,
        raw={},
    )
    return FeedbackPoolRow(manifest=manifest, item=item)


def _optimizer_item(title: str, raw: dict[str, object]) -> OptimizerItem:
    return OptimizerItem(
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
