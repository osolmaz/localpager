from __future__ import annotations

import json
import re
from collections import Counter
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Iterable

REPO_ROOT = Path(__file__).resolve().parents[3]

DEFAULT_DS4_PATH = Path("/home/bob/oc/openclaw-classification-dataset/ds4.jsonl")
DEFAULT_FEEDBACK_MANIFEST_PATH = Path(
    "/home/bob/scratch/shaun-openclaw-data-rows/gepa-good-60.rows.jsonl"
)
DEFAULT_EVALSTATE_TRAIN_PATH = Path("/home/bob/repos/openclaw-git-labels/data/splits/feedback300.jsonl")
DEFAULT_EVALSTATE_PARETO_PATH = Path("/home/bob/repos/openclaw-git-labels/data/splits/pareto60.jsonl")
DEFAULT_EVALSTATE_HELDOUT_PATH = Path("/home/bob/repos/openclaw-git-labels/data/splits/bench78.jsonl")
DEFAULT_TAXONOMY_PATH = REPO_ROOT / "examples/profiles/openclaw-routing-topics.json"


class DatasetError(ValueError):
    """Raised when dataset inputs do not match the optimizer contract."""


@dataclass(frozen=True)
class DS4Row:
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
class FeedbackManifestRow:
    id: str
    source_set: str
    audit_bucket: str
    repo: str
    item_type: str
    number: int
    url: str
    title: str
    ds4_topics: tuple[str, ...]
    raw: dict[str, Any]


@dataclass(frozen=True)
class FeedbackPoolRow:
    manifest: FeedbackManifestRow
    ds4: DS4Row


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


def load_ds4(path: Path, allowed_topics: Iterable[str] | None = None) -> tuple[DS4Row, ...]:
    allowed = frozenset(allowed_topics) if allowed_topics is not None else None
    rows: list[DS4Row] = []
    seen: set[str] = set()
    for raw in load_jsonl(path):
        row = _parse_ds4_row(raw, allowed)
        if row.id in seen:
            raise DatasetError(f"duplicate DS4 row id: {row.id}")
        seen.add(row.id)
        rows.append(row)
    return tuple(rows)


def load_feedback_manifest(path: Path) -> tuple[FeedbackManifestRow, ...]:
    rows: list[FeedbackManifestRow] = []
    seen: set[str] = set()
    for raw in load_jsonl(path):
        row = _parse_feedback_manifest_row(raw)
        if row.id in seen:
            raise DatasetError(f"duplicate feedback row id: {row.id}")
        seen.add(row.id)
        rows.append(row)
    return tuple(rows)


def build_feedback_pool(ds4_path: Path, manifest_path: Path, taxonomy_path: Path) -> FeedbackPool:
    allowed_topics = load_taxonomy(taxonomy_path)
    ds4_rows = load_ds4(ds4_path, allowed_topics)
    ds4_by_id = {row.id: row for row in ds4_rows}
    manifest_rows = load_feedback_manifest(manifest_path)
    pool_rows: list[FeedbackPoolRow] = []
    for manifest in manifest_rows:
        ds4 = ds4_by_id.get(manifest.id)
        if ds4 is None:
            raise DatasetError(f"feedback row missing from canonical DS4: {manifest.id}")
        if manifest.ds4_topics and manifest.ds4_topics != ds4.topics_of_interest:
            raise DatasetError(
                "feedback manifest ds4_topics differs from canonical ds4.jsonl "
                f"for {manifest.id}: manifest={manifest.ds4_topics!r} canonical={ds4.topics_of_interest!r}"
            )
        pool_rows.append(FeedbackPoolRow(manifest=manifest, ds4=ds4))
    return FeedbackPool(
        rows=tuple(pool_rows),
        composition=dict(Counter(row.manifest.audit_bucket for row in pool_rows)),
        source_row_count=len(ds4_rows),
    )


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
        if row.ds4.id in seen:
            raise DatasetError(f"duplicate evalstate row id in {path}: {row.ds4.id}")
        seen.add(row.ds4.id)
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


def _parse_ds4_row(raw: dict[str, Any], allowed_topics: frozenset[str] | None) -> DS4Row:
    row_id = _required_str(raw, "id", "DS4 row")
    topics = _normal_topics(raw.get("topics_of_interest"), f"DS4 row {row_id}", allowed_topics)
    return DS4Row(
        id=row_id,
        repo=_required_str(raw, "repo", f"DS4 row {row_id}"),
        item_type=_required_str(raw, "item_type", f"DS4 row {row_id}"),
        number=_required_int(raw, "number", f"DS4 row {row_id}"),
        url=_required_str(raw, "url", f"DS4 row {row_id}"),
        title=_required_str(raw, "title", f"DS4 row {row_id}"),
        topics_of_interest=topics,
        raw=raw,
        target=raw.get("target") if isinstance(raw.get("target"), str) else None,
        github_context=raw.get("github_context") if isinstance(raw.get("github_context"), str) else None,
    )


def _parse_feedback_manifest_row(raw: dict[str, Any]) -> FeedbackManifestRow:
    row_id = _required_str(raw, "id", "feedback row")
    return FeedbackManifestRow(
        id=row_id,
        source_set=_required_str(raw, "source_set", f"feedback row {row_id}"),
        audit_bucket=_required_str(raw, "audit_bucket", f"feedback row {row_id}"),
        repo=_required_str(raw, "repo", f"feedback row {row_id}"),
        item_type=_required_str(raw, "item_type", f"feedback row {row_id}"),
        number=_required_int(raw, "number", f"feedback row {row_id}"),
        url=_required_str(raw, "url", f"feedback row {row_id}"),
        title=_required_str(raw, "title", f"feedback row {row_id}"),
        ds4_topics=_normal_topics(raw.get("ds4_topics"), f"feedback row {row_id}", None),
        raw=raw,
    )


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
    ds4 = DS4Row(
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
    manifest = FeedbackManifestRow(
        id=row_id,
        source_set="evalstate-openclaw-git-labels",
        audit_bucket=split_name,
        repo=repo,
        item_type=item_type,
        number=number,
        url=url or target,
        title=title,
        ds4_topics=topics,
        raw=raw,
    )
    return FeedbackPoolRow(manifest=manifest, ds4=ds4)


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
