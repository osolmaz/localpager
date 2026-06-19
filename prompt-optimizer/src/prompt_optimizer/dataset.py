from __future__ import annotations

import json
import re
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Iterable

REPO_ROOT = Path(__file__).resolve().parents[3]

DEFAULT_EVALSTATE_TRAIN_PATH = Path("/home/bob/repos/openclaw-git-labels/data/splits/feedback300.jsonl")
DEFAULT_EVALSTATE_PARETO_PATH = Path("/home/bob/repos/openclaw-git-labels/data/splits/pareto60.jsonl")
DEFAULT_EVALSTATE_HELDOUT_PATH = Path("/home/bob/repos/openclaw-git-labels/data/splits/bench78.jsonl")
DEFAULT_TAXONOMY_PATH = REPO_ROOT / "examples/profiles/openclaw-routing-topics.json"


class DatasetError(ValueError):
    """Raised when dataset inputs do not match the optimizer contract."""


@dataclass(frozen=True)
class OptimizerItem:
    id: str
    repo: str
    item_type: str
    number: int
    url: str
    title: str
    topics_of_interest: tuple[str, ...]
    raw: dict[str, Any]
    target: str | None = None
    github_context: str | None = None


@dataclass(frozen=True)
class OptimizerManifest:
    id: str
    source_set: str
    audit_bucket: str
    repo: str
    item_type: str
    number: int
    url: str
    title: str
    gold_topics: tuple[str, ...]
    raw: dict[str, Any]


@dataclass(frozen=True)
class FeedbackPoolRow:
    manifest: OptimizerManifest
    item: OptimizerItem


@dataclass(frozen=True)
class FeedbackPool:
    rows: tuple[FeedbackPoolRow, ...]
    composition: dict[str, int]
    source_row_count: int


def load_jsonl(path: Path) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    try:
        with path.open("r", encoding="utf-8") as handle:
            for line_no, line in enumerate(handle, 1):
                if not line.strip():
                    continue
                try:
                    value = json.loads(line)
                except json.JSONDecodeError as exc:
                    raise DatasetError(f"{path}:{line_no}: invalid JSONL row: {exc}") from exc
                if not isinstance(value, dict):
                    raise DatasetError(f"{path}:{line_no}: JSONL row must be an object")
                rows.append(value)
    except FileNotFoundError as exc:
        raise DatasetError(f"missing JSONL file: {path}") from exc
    return rows


def load_taxonomy(path: Path) -> frozenset[str]:
    try:
        root = json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError as exc:
        raise DatasetError(f"missing taxonomy file: {path}") from exc
    except json.JSONDecodeError as exc:
        raise DatasetError(f"invalid taxonomy JSON: {path}: {exc}") from exc
    topics = _topic_ids_from_taxonomy(root)
    if not topics:
        raise DatasetError(f"taxonomy has no topics: {path}")
    return frozenset(topics)


def build_evalstate_pool(split_path: Path, taxonomy_path: Path, *, split_name: str | None = None) -> FeedbackPool:
    allowed_topics = load_taxonomy(taxonomy_path)
    rows = load_evalstate_split(split_path, allowed_topics, split_name=split_name or split_path.stem)
    return FeedbackPool(
        rows=rows,
        composition={split_name or split_path.stem: len(rows)},
        source_row_count=len(rows),
    )


def load_evalstate_split(
    path: Path,
    allowed_topics: Iterable[str] | None = None,
    *,
    split_name: str,
) -> tuple[FeedbackPoolRow, ...]:
    allowed = frozenset(allowed_topics) if allowed_topics is not None else None
    rows: list[FeedbackPoolRow] = []
    seen: set[str] = set()
    for raw in load_jsonl(path):
        row = _parse_evalstate_row(raw, allowed, split_name)
        if row.item.id in seen:
            raise DatasetError(f"duplicate evalstate row id in {path}: {row.item.id}")
        seen.add(row.item.id)
        rows.append(row)
    return tuple(rows)


def _topic_ids_from_taxonomy(root: Any) -> list[str]:
    if not isinstance(root, dict):
        raise DatasetError("taxonomy root must be a JSON object")
    topics = root.get("topics")
    if isinstance(topics, dict):
        return [_validate_topic_id(key) for key in topics]
    if isinstance(topics, list):
        ids: list[str] = []
        for index, topic in enumerate(topics):
            if not isinstance(topic, dict):
                raise DatasetError(f"taxonomy topics[{index}] must be an object")
            ids.append(_validate_topic_id(_required_str(topic, "id", f"taxonomy topics[{index}]")))
        return ids
    properties = root.get("properties")
    if isinstance(properties, dict):
        topic_prop = properties.get("topics_of_interest")
        if isinstance(topic_prop, dict):
            items = topic_prop.get("items")
            if isinstance(items, dict) and isinstance(items.get("enum"), list):
                return [_validate_topic_id(_as_str(value, "topic enum value")) for value in items["enum"]]
    raise DatasetError("unsupported taxonomy shape")


def _parse_evalstate_row(
    raw: dict[str, Any],
    allowed_topics: frozenset[str] | None,
    split_name: str,
) -> FeedbackPoolRow:
    row_id = _required_str(raw, "id", "evalstate row")
    target = _required_str(raw, "target", f"evalstate row {row_id}")
    title = _required_str(raw, "title", f"evalstate row {row_id}")
    github_context = _required_str(raw, "github_context", f"evalstate row {row_id}")
    topics = _normal_topics(raw.get("expected_topics"), f"evalstate row {row_id}", allowed_topics)
    repo, item_type, number, url = _evalstate_identity(target, github_context)
    item = OptimizerItem(
        id=row_id,
        repo=repo,
        item_type=item_type,
        number=number,
        url=url or target,
        title=title,
        topics_of_interest=topics,
        raw=raw,
        target=target,
        github_context=github_context,
    )
    manifest = OptimizerManifest(
        id=row_id,
        source_set="evalstate-openclaw-git-labels",
        audit_bucket=split_name,
        repo=repo,
        item_type=item_type,
        number=number,
        url=url or target,
        title=title,
        gold_topics=topics,
        raw=raw,
    )
    return FeedbackPoolRow(manifest=manifest, item=item)


def _evalstate_identity(target: str, github_context: str) -> tuple[str, str, int, str]:
    repo = target.split(" ", 1)[0] if " " in target else ""
    item_type = "github_pr" if "github_pr" in target else "github_issue" if "github_issue" in target else "github_item"
    number = 0
    if match := re.search(r"#([0-9]+)", target):
        number = int(match.group(1))
    url = ""
    if match := re.search(r"(?m)^- URL: (\S+)$", github_context):
        url = match.group(1)
    return repo, item_type, number, url


def _normal_topics(
    value: Any, context: str, allowed_topics: frozenset[str] | None
) -> tuple[str, ...]:
    if not isinstance(value, list):
        raise DatasetError(f"{context}: topics must be a list")
    topics: list[str] = []
    seen: set[str] = set()
    for raw_topic in value:
        topic = _as_str(raw_topic, f"{context}: topic")
        if allowed_topics is not None and topic not in allowed_topics:
            raise DatasetError(f"{context}: topic not in taxonomy: {topic}")
        if topic in seen:
            continue
        seen.add(topic)
        topics.append(topic)
    return tuple(topics)


def _required_str(raw: dict[str, Any], key: str, context: str) -> str:
    return _as_str(raw.get(key), f"{context}: {key}")


def _required_int(raw: dict[str, Any], key: str, context: str) -> int:
    value = raw.get(key)
    if not isinstance(value, int):
        raise DatasetError(f"{context}: {key} must be an integer")
    return value


def _as_str(value: Any, context: str) -> str:
    if not isinstance(value, str) or value == "":
        raise DatasetError(f"{context} must be a non-empty string")
    return value


def _validate_topic_id(topic_id: str) -> str:
    if not topic_id or not topic_id.replace("_", "").isalnum() or not topic_id[0].isalpha():
        raise DatasetError(f"invalid topic id: {topic_id}")
    return topic_id
