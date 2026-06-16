from __future__ import annotations

import json
import os
import re
import subprocess
import tempfile
from dataclasses import dataclass
from pathlib import Path
from typing import Protocol

from prompt_optimizer.dataset import DEFAULT_TAXONOMY_PATH, DS4Row, FeedbackPoolRow, REPO_ROOT

DEFAULT_CLASSIFIER_COMMAND = REPO_ROOT / "scripts/localpager-classifier"
DEFAULT_SCHEMA_PATH = REPO_ROOT / "schemas/classification.schema.json"
DEFAULT_MAX_TOKENS = 8192


@dataclass(frozen=True)
class ClassifierOutput:
    topics_of_interest: tuple[str, ...]
    description: str
    caveats: tuple[str, ...] = ()
    error: str | None = None


class ClassifierHarness(Protocol):
    """Classifier runtime used by LocalpagerAdapter.

    Production implementations must call `localpager-agent`; tests can provide a
    deterministic fake with the same method shape.
    """

    def classify(self, row: FeedbackPoolRow, prompt_text: str) -> ClassifierOutput:
        ...


@dataclass(frozen=True)
class StaticClassifierHarness:
    predictions: dict[str, tuple[str, ...]]

    def classify(self, row: FeedbackPoolRow, prompt_text: str) -> ClassifierOutput:
        del prompt_text
        return ClassifierOutput(
            topics_of_interest=self.predictions.get(row.ds4.id, ()),
            description="static test prediction",
        )


@dataclass(frozen=True)
class LocalpagerAgentHarness:
    """Subprocess harness around the production localpager classifier wrapper."""

    model: str
    classifier_command: Path = DEFAULT_CLASSIFIER_COMMAND
    schema_path: Path = DEFAULT_SCHEMA_PATH
    topic_taxonomy_path: Path = DEFAULT_TAXONOMY_PATH
    base_url: str | None = None
    context_window: int | None = None
    thinking: str = "medium"
    max_tokens: int = DEFAULT_MAX_TOKENS
    timeout_ms: int = 900_000
    state_dir: Path | None = None
    extra_agent_args: tuple[str, ...] = ()

    def classify(self, row: FeedbackPoolRow, prompt_text: str) -> ClassifierOutput:
        with tempfile.TemporaryDirectory(prefix=f"localpager-gepa-{row.ds4.id}-") as tmp:
            tmp_path = Path(tmp)
            prompt_path = tmp_path / "candidate.prompt.md"
            context_path = tmp_path / "github-context.md"
            prompt_path.write_text(prompt_text, encoding="utf-8")
            context_path.write_text(row.ds4.github_context or render_ds4_context(row.ds4), encoding="utf-8")
            args = [
                str(self.classifier_command),
                row.ds4.target or row.ds4.url,
                "--model",
                self.model,
                "--schema",
                str(self.schema_path),
                "--prompt-template",
                str(prompt_path),
                "--topic-taxonomy",
                str(self.topic_taxonomy_path),
                "--github-context-file",
                str(context_path),
                *self.extra_agent_args,
            ]
            env = self._env()
            try:
                completed = subprocess.run(
                    args,
                    cwd=REPO_ROOT,
                    env=env,
                    text=True,
                    stdin=subprocess.DEVNULL,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    timeout=(self.timeout_ms / 1000) + 30,
                    check=False,
                )
            except subprocess.TimeoutExpired as exc:
                return ClassifierOutput(
                    topics_of_interest=(),
                    description="",
                    error=f"classifier timed out after {exc.timeout:.0f}s",
                )
            if completed.returncode != 0:
                return ClassifierOutput(
                    topics_of_interest=(),
                    description="",
                    error=_tail(
                        f"classifier exit {completed.returncode}: "
                        f"{completed.stderr or completed.stdout}"
                    ),
                )
            return parse_classifier_stdout(completed.stdout)

    def _env(self) -> dict[str, str]:
        env = os.environ.copy()
        env["LOCALPAGER_AGENT_THINKING"] = self.thinking
        env["LOCALPAGER_AGENT_MAX_TOKENS"] = str(self.max_tokens)
        env["LOCALPAGER_AGENT_TIMEOUT_MS"] = str(self.timeout_ms)
        if self.base_url:
            env["LOCALPAGER_AGENT_BASE_URL"] = self.base_url
        if self.context_window is not None:
            env["LOCALPAGER_AGENT_CONTEXT_WINDOW"] = str(self.context_window)
        if self.state_dir is not None:
            env["LOCALPAGER_CLASSIFIER_STATE_DIR"] = str(self.state_dir)
        return env


def parse_classifier_stdout(stdout: str) -> ClassifierOutput:
    try:
        raw = json.loads(stdout.strip())
    except json.JSONDecodeError as exc:
        return ClassifierOutput(
            topics_of_interest=(),
            description="",
            error=f"classifier returned non-JSON stdout: {_tail(stdout)} ({exc})",
        )
    if not isinstance(raw, dict):
        return ClassifierOutput(
            topics_of_interest=(),
            description="",
            error="classifier JSON output must be an object",
        )
    topics = raw.get("topics_of_interest")
    description = raw.get("description")
    caveats = raw.get("caveats")
    if not isinstance(topics, list) or any(not isinstance(topic, str) for topic in topics):
        return ClassifierOutput(
            topics_of_interest=(),
            description="",
            error="classifier JSON field topics_of_interest must be a string array",
        )
    if not isinstance(description, str):
        return ClassifierOutput(
            topics_of_interest=(),
            description="",
            error="classifier JSON field description must be a string",
        )
    if not isinstance(caveats, list) or any(not isinstance(caveat, str) for caveat in caveats):
        return ClassifierOutput(
            topics_of_interest=(),
            description="",
            error="classifier JSON field caveats must be a string array",
        )
    return ClassifierOutput(
        topics_of_interest=tuple(topics),
        description=description,
        caveats=tuple(caveats),
    )


def render_ds4_context(row: DS4Row) -> str:
    raw = row.raw
    lines = ["GitHub item:"]
    _append_line(lines, "Repository", row.repo)
    _append_line(lines, "Type", row.item_type)
    _append_line(lines, "Number", str(row.number))
    _append_line(lines, "URL", row.url)
    _append_line(lines, "Title", _neutralize(row.title))
    _append_line(lines, "State", _as_str(raw.get("state")))
    _append_line(lines, "Author", _as_str(raw.get("author")))
    labels = raw.get("labels")
    if isinstance(labels, list):
        _append_line(
            lines,
            "Labels",
            ", ".join(str(label) for label in labels if str(label).strip()),
        )
    changed_files = raw.get("changed_files")
    if isinstance(changed_files, list):
        _append_line(
            lines,
            "Changed files",
            _truncate(
                ", ".join(str(path) for path in changed_files if str(path).strip()),
                2000,
                "changed files",
            ),
        )
    body = _as_str(raw.get("body"))
    if body:
        lines.extend(["", "Body:", "```markdown", _truncate(_neutralize(body), 2500, "body"), "```"])
    comments = _render_comments(raw.get("comments"))
    if comments:
        lines.extend(
            [
                "",
                "Comments/context:",
                "```markdown",
                _truncate(_neutralize(comments), 1500, "comments/context"),
                "```",
            ]
        )
    diff = _as_str(raw.get("diff"))
    if diff:
        lines.extend(["", "Diff/context:", "```diff", _truncate(_neutralize(diff), 5000, "diff"), "```"])
    return "\n".join(lines).rstrip() + "\n"


def _render_comments(value: object) -> str:
    if not isinstance(value, list):
        return ""
    parts: list[str] = []
    for comment in value:
        if not isinstance(comment, dict):
            continue
        author = _as_str(comment.get("author")) or "unknown"
        created_at = _as_str(comment.get("created_at"))
        when = f" at {created_at}" if created_at else ""
        body = _as_str(comment.get("body"))
        if body:
            parts.append(f"- {author}{when}:\n{body}")
    return "\n\n".join(parts)


def _append_line(lines: list[str], label: str, value: str) -> None:
    value = value.strip()
    if value:
        lines.append(f"- {label}: {value}")


def _as_str(value: object) -> str:
    return value if isinstance(value, str) else ""


def _truncate(text: str, max_chars: int, label: str) -> str:
    if len(text) <= max_chars:
        return text
    head_size = max_chars * 7 // 10
    tail_size = max(0, max_chars - head_size - 120)
    return (
        f"{text[:head_size]}\n\n"
        f"[{label} truncated: {len(text) - head_size - tail_size} characters omitted]\n\n"
        f"{text[len(text) - tail_size:]}"
    )


def _neutralize(text: str) -> str:
    def replace(match: re.Match[str]) -> str:
        return match.group(0).replace("<", "&lt;").replace(">", "&gt;")

    return re.sub(r"(?i)</?(?:think|final|analysis|assistant|system|user)\b[^>]*>", replace, text)


def _tail(value: str, max_chars: int = 1000) -> str:
    value = value.strip()
    if len(value) <= max_chars:
        return value
    return value[-max_chars:]
