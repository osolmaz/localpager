from __future__ import annotations

import json
import os
import tempfile
import textwrap
import unittest
from pathlib import Path

from prompt_optimizer.reflection import CodexReflectionError, CodexReflectionLM


class ReflectionTest(unittest.TestCase):
    def test_codex_reflection_lm_calls_exec_on_stdin(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            fake = _fake_codex(Path(tmp), stdout_json=True)
            lm = CodexReflectionLM(executable=str(fake), model="gpt-test", timeout_seconds=2)

            output = lm("reflect on this")

        self.assertEqual(json.loads(output), {"length": 15})
        self.assertIsNotNone(lm.last_run)
        assert lm.last_run is not None
        self.assertEqual(lm.last_run.codex_version, "codex fake 1.0")
        self.assertEqual(lm.last_run.returncode, 0)
        self.assertIn("exec", lm.last_run.argv)
        self.assertIn("--sandbox", lm.last_run.argv)
        self.assertIn("read-only", lm.last_run.argv)
        self.assertIn("--ephemeral", lm.last_run.argv)
        self.assertIn("gpt-test", lm.last_run.argv)

    def test_require_json_fails_closed(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            fake = _fake_codex(Path(tmp), stdout_json=False)
            lm = CodexReflectionLM(executable=str(fake), timeout_seconds=2, require_json=True)

            with self.assertRaisesRegex(CodexReflectionError, "non-JSON"):
                lm("reflect")

    def test_chat_message_prompt_is_serialized(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            fake = _fake_codex(Path(tmp), stdout_json=True)
            lm = CodexReflectionLM(executable=str(fake), timeout_seconds=2)

            output = json.loads(lm([{"role": "user", "content": "hello"}]))

        self.assertGreater(output["length"], len("hello"))


def _fake_codex(root: Path, stdout_json: bool) -> Path:
    script = root / "codex"
    if stdout_json:
        response = 'print(json.dumps({"length": len(payload)}))'
    else:
        response = 'print("not json")'
    script.write_text(
        textwrap.dedent(
            f"""\
            #!/usr/bin/env python3
            import json
            import sys

            if "--version" in sys.argv:
                print("codex fake 1.0")
                raise SystemExit(0)
            payload = sys.stdin.read()
            {response}
            """
        ),
        encoding="utf-8",
    )
    script.chmod(script.stat().st_mode | os.stat(script).st_mode | 0o111)
    return script


if __name__ == "__main__":
    unittest.main()
