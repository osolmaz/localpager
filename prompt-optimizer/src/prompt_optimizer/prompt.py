from __future__ import annotations

import hashlib
import re
from dataclasses import dataclass
from pathlib import Path

PROMPT_OPTIMIZER_ROOT = Path(__file__).resolve().parents[2]

DEFAULT_EVALSTATE_SEED_PROMPT_PATH = (
    PROMPT_OPTIMIZER_ROOT
    / "prompts"
    / "localpager-openclaw-routing-v10-overlay-scaffold.hbs"
)
DEFAULT_EVALSTATE_SEED_OVERLAY_PATH = (
    PROMPT_OPTIMIZER_ROOT
    / "prompts"
    / "localpager-openclaw-routing-v10-overlay-seed.md"
)

REQUIRED_PLACEHOLDERS = (
    "__TARGET__",
    "__GITHUB_CONTEXT__",
    "__ALLOWED_TOPICS_JSON__",
    "__TOPIC_DESCRIPTIONS__",
)
ROUTING_POLICY_OVERLAY_PLACEHOLDER = "__ROUTING_POLICY_OVERLAY__"

ROUTING_POLICY_START = "\n## Goal\n"
ROUTING_POLICY_START_MARKERS = (
    ROUTING_POLICY_START,
    "\n## Inner Monologue\n",
)
ROUTING_POLICY_END = "\n## Target\n"


class PromptError(ValueError):
    """Raised when the seed prompt cannot be safely transformed."""


@dataclass(frozen=True)
class PromptParts:
    prefix: str
    routing_policy: str
    suffix: str

    @property
    def template(self) -> str:
        return self.prefix + self.routing_policy + self.suffix

    @property
    def template_sha256(self) -> str:
        return _sha256(self.template)

    @property
    def routing_policy_sha256(self) -> str:
        return _sha256(self.routing_policy)

    def assemble(self, routing_policy: str) -> str:
        candidate = self.prefix + routing_policy + self.suffix
        validate_placeholders(candidate)
        return candidate


def load_overlay_seed_prompt(
    scaffold_path: Path = DEFAULT_EVALSTATE_SEED_PROMPT_PATH,
    overlay_path: Path = DEFAULT_EVALSTATE_SEED_OVERLAY_PATH,
) -> PromptParts:
    try:
        scaffold = scaffold_path.read_text(encoding="utf-8")
    except FileNotFoundError as exc:
        raise PromptError(f"missing prompt scaffold: {scaffold_path}") from exc
    try:
        overlay = overlay_path.read_text(encoding="utf-8")
    except FileNotFoundError as exc:
        raise PromptError(f"missing seed overlay: {overlay_path}") from exc
    return split_overlay_seed_prompt(normalize_template_variables(scaffold), overlay)


def normalize_template_variables(template: str) -> str:
    replacements = {
        r"\{\{\s*target\s*\}\}": "__TARGET__",
        r"\{\{\{\s*github_context\s*\}\}\}": "__GITHUB_CONTEXT__",
        r"\{\{\{\s*allowed_topics_json\s*\}\}\}": "__ALLOWED_TOPICS_JSON__",
        r"\{\{\{\s*topic_descriptions\s*\}\}\}": "__TOPIC_DESCRIPTIONS__",
        r"\{\{\{?\s*routing_policy_overlay\s*\}?\}\}": ROUTING_POLICY_OVERLAY_PLACEHOLDER,
    }
    normalized = template
    for pattern, replacement in replacements.items():
        normalized = re.sub(pattern, replacement, normalized)
    validate_placeholders(normalized, allow_overlay_placeholder=True)
    return normalized


def split_seed_prompt(template: str) -> PromptParts:
    start, _start_marker = _find_first_marker(template, ROUTING_POLICY_START_MARKERS)
    end = template.find(ROUTING_POLICY_END)
    if start < 0:
        markers = ", ".join(marker.strip() for marker in ROUTING_POLICY_START_MARKERS)
        raise PromptError(f"missing routing policy start marker; expected one of: {markers}")
    if end < 0:
        raise PromptError(f"missing routing policy end marker: {ROUTING_POLICY_END.strip()}")
    if end <= start:
        raise PromptError("routing policy end marker appears before start marker")
    parts = PromptParts(
        prefix=template[: start + 1],
        routing_policy=template[start + 1 : end + 1],
        suffix=template[end + 1 :],
    )
    parts.assemble(parts.routing_policy)
    return parts


def split_overlay_seed_prompt(scaffold: str, overlay: str) -> PromptParts:
    count = scaffold.count(ROUTING_POLICY_OVERLAY_PLACEHOLDER)
    if count != 1:
        raise PromptError(
            "prompt scaffold must contain exactly one routing policy overlay "
            f"placeholder: {ROUTING_POLICY_OVERLAY_PLACEHOLDER}"
        )
    prefix, suffix = scaffold.split(ROUTING_POLICY_OVERLAY_PLACEHOLDER, maxsplit=1)
    parts = PromptParts(prefix=prefix, routing_policy=overlay, suffix=suffix)
    parts.assemble(parts.routing_policy)
    return parts


def validate_placeholders(template: str, *, allow_overlay_placeholder: bool = False) -> None:
    missing = [placeholder for placeholder in REQUIRED_PLACEHOLDERS if placeholder not in template]
    if missing:
        raise PromptError(f"missing required placeholder(s): {', '.join(missing)}")
    if not allow_overlay_placeholder and ROUTING_POLICY_OVERLAY_PLACEHOLDER in template:
        raise PromptError(f"unreplaced routing policy overlay placeholder remains: {ROUTING_POLICY_OVERLAY_PLACEHOLDER}")
    handlebars_left = re.findall(r"\{\{\{?\s*[A-Za-z0-9_]+\s*\}?\}\}", template)
    if handlebars_left:
        raise PromptError(f"un-normalized Handlebars variables remain: {', '.join(sorted(set(handlebars_left)))}")


def _sha256(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


def _find_first_marker(template: str, markers: tuple[str, ...]) -> tuple[int, str]:
    matches = [(index, marker) for marker in markers if (index := template.find(marker)) >= 0]
    if not matches:
        return -1, ""
    return min(matches, key=lambda match: match[0])
