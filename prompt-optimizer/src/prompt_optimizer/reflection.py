from __future__ import annotations

import hashlib
import json
import subprocess
from dataclasses import dataclass
from pathlib import Path
from typing import Any


class CodexReflectionError(RuntimeError):
    """Raised when the Codex reflection subprocess fails."""


@dataclass(frozen=True)
class CodexRunMetadata:
    argv: tuple[str, ...]
    prompt_sha256: str
    codex_version: str
    returncode: int


@dataclass(frozen=True)
class CodexReflectionLM:
    """GEPA-compatible reflection LM wrapper around non-interactive Codex CLI."""

    executable: str = "codex"
    model: str | None = None
    profile: str | None = None
    extra_args: tuple[str, ...] = ()
    timeout_seconds: float = 300.0
    sandbox: str = "read-only"
    ephemeral: bool = True
    output_schema: Path | None = None
    require_json: bool = False

    def __post_init__(self) -> None:
        object.__setattr__(self, "last_run", None)

    def __call__(self, prompt: str | list[dict[str, Any]]) -> str:
        prompt_text = _prompt_text(prompt)
        argv = self.argv()
        version = self.codex_version()
        result = subprocess.run(
            argv,
            input=prompt_text,
            text=True,
            capture_output=True,
            timeout=self.timeout_seconds,
            check=False,
        )
        metadata = CodexRunMetadata(
            argv=tuple(argv),
            prompt_sha256=_sha256(prompt_text),
            codex_version=version,
            returncode=result.returncode,
        )
        object.__setattr__(self, "last_run", metadata)
        if result.returncode != 0:
            raise CodexReflectionError(
                f"codex reflection failed with exit {result.returncode}: {result.stderr.strip()}"
            )
        output = result.stdout.strip()
        if self.require_json:
            try:
                json.loads(output)
            except json.JSONDecodeError as exc:
                raise CodexReflectionError("codex reflection returned non-JSON output") from exc
        return output

    def argv(self) -> list[str]:
        argv = [self.executable, "exec"]
        if self.ephemeral:
            argv.append("--ephemeral")
        if self.sandbox:
            argv.extend(["--sandbox", self.sandbox])
        if self.model is not None:
            argv.extend(["--model", self.model])
        if self.profile is not None:
            argv.extend(["--profile", self.profile])
        if self.output_schema is not None:
            argv.extend(["--output-schema", str(self.output_schema)])
        argv.extend(self.extra_args)
        argv.append("-")
        return argv

    def codex_version(self) -> str:
        result = subprocess.run(
            [self.executable, "--version"],
            text=True,
            capture_output=True,
            timeout=min(self.timeout_seconds, 10.0),
            check=False,
        )
        if result.returncode != 0:
            return f"unknown: {result.stderr.strip()}"
        return result.stdout.strip()


def _prompt_text(prompt: str | list[dict[str, Any]]) -> str:
    if isinstance(prompt, str):
        return prompt
    return json.dumps(prompt, indent=2, sort_keys=True)


def _sha256(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()
