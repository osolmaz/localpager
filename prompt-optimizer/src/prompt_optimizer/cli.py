from __future__ import annotations

import argparse
import json
from dataclasses import asdict
from pathlib import Path
from typing import NoReturn

from prompt_optimizer.dataset import (
    DEFAULT_DS4_PATH,
    DEFAULT_FEEDBACK_MANIFEST_PATH,
    DEFAULT_TAXONOMY_PATH,
    build_feedback_pool,
    load_taxonomy,
)
from prompt_optimizer.metric import SCORING_CONFIG
from prompt_optimizer.prompt import DEFAULT_SEED_PROMPT_PATH, load_seed_prompt


def main() -> None:
    parser = argparse.ArgumentParser(prog="localpager-prompt-optimizer")
    subparsers = parser.add_subparsers(dest="command")

    summary = subparsers.add_parser("summary", help="print deterministic input summary")
    summary.add_argument("--ds4", type=Path, default=DEFAULT_DS4_PATH)
    summary.add_argument("--feedback-manifest", type=Path, default=DEFAULT_FEEDBACK_MANIFEST_PATH)
    summary.add_argument("--taxonomy", type=Path, default=DEFAULT_TAXONOMY_PATH)
    summary.add_argument("--seed-prompt", type=Path, default=DEFAULT_SEED_PROMPT_PATH)

    args = parser.parse_args()
    if args.command == "summary":
        print(_summary(args.ds4, args.feedback_manifest, args.taxonomy, args.seed_prompt))
        return
    parser.print_help()


def _summary(ds4: Path, feedback_manifest: Path, taxonomy: Path, seed_prompt: Path) -> str:
    topics = load_taxonomy(taxonomy)
    pool = build_feedback_pool(ds4, feedback_manifest, taxonomy)
    prompt = load_seed_prompt(seed_prompt)
    payload = {
        "ds4_path": str(ds4),
        "ds4_rows": pool.source_row_count,
        "feedback_manifest_path": str(feedback_manifest),
        "feedback_rows": len(pool.rows),
        "feedback_composition": dict(sorted(pool.composition.items())),
        "taxonomy_path": str(taxonomy),
        "taxonomy_topics": len(topics),
        "seed_prompt_path": str(seed_prompt),
        "seed_prompt_sha256": prompt.template_sha256,
        "routing_policy_sha256": prompt.routing_policy_sha256,
        "scoring": asdict(SCORING_CONFIG),
    }
    return json.dumps(payload, indent=2, sort_keys=True)


def die(message: str) -> NoReturn:
    raise SystemExit(message)


if __name__ == "__main__":
    main()
