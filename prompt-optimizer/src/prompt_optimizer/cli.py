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
from prompt_optimizer.run import (
    DEFAULT_CONCURRENCY,
    DEFAULT_MAX_TOKENS,
    DEFAULT_MODEL,
    GEPARunConfig,
    HarnessConfig,
    default_output_dir,
    evaluate_seed,
    load_optimizer_inputs,
    localpager_agent_harness,
    run_gepa,
    static_empty_harness,
)


def main() -> None:
    parser = argparse.ArgumentParser(prog="localpager-prompt-optimizer")
    subparsers = parser.add_subparsers(dest="command")

    summary = subparsers.add_parser("summary", help="print deterministic input summary")
    summary.add_argument("--ds4", type=Path, default=DEFAULT_DS4_PATH)
    summary.add_argument("--feedback-manifest", type=Path, default=DEFAULT_FEEDBACK_MANIFEST_PATH)
    summary.add_argument("--taxonomy", type=Path, default=DEFAULT_TAXONOMY_PATH)
    summary.add_argument("--seed-prompt", type=Path, default=DEFAULT_SEED_PROMPT_PATH)

    evaluate = subparsers.add_parser("evaluate-seed", help="evaluate the seed prompt without optimizing")
    _add_input_args(evaluate)
    evaluate.add_argument("--limit", type=int, default=1, help="number of feedback rows to evaluate")
    evaluate.add_argument("--concurrency", type=int, default=DEFAULT_CONCURRENCY)
    evaluate.add_argument(
        "--harness",
        choices=("static-empty", "localpager-agent"),
        default="static-empty",
        help="static-empty is no-model and localpager-agent runs the production wrapper",
    )
    _add_harness_args(evaluate)

    optimize = subparsers.add_parser("optimize", help="run GEPA with the localpager-agent harness")
    _add_input_args(optimize)
    optimize.add_argument("--output-dir", type=Path, default=None)
    optimize.add_argument("--row-limit", type=int, default=None)
    optimize.add_argument("--max-metric-calls", type=int, required=True)
    optimize.add_argument("--reflection-minibatch-size", type=int, default=4)
    optimize.add_argument("--seed", type=int, default=0)
    optimize.add_argument("--concurrency", type=int, default=DEFAULT_CONCURRENCY)
    _add_harness_args(optimize)

    args = parser.parse_args()
    if args.command == "summary":
        print(_summary(args.ds4, args.feedback_manifest, args.taxonomy, args.seed_prompt))
        return
    if args.command == "evaluate-seed":
        print(_evaluate_seed(args))
        return
    if args.command == "optimize":
        print(_optimize(args))
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


def _evaluate_seed(args: argparse.Namespace) -> str:
    inputs = load_optimizer_inputs(
        ds4_path=args.ds4,
        feedback_manifest_path=args.feedback_manifest,
        taxonomy_path=args.taxonomy,
        seed_prompt_path=args.seed_prompt,
    )
    if args.harness == "static-empty":
        harness = static_empty_harness()
    else:
        harness = localpager_agent_harness(_harness_config(args))
    payload = evaluate_seed(
        inputs=inputs,
        harness=harness,
        concurrency=args.concurrency,
        limit=args.limit,
    )
    payload["harness"] = args.harness
    payload["concurrency"] = args.concurrency
    return json.dumps(payload, indent=2, sort_keys=True)


def _optimize(args: argparse.Namespace) -> str:
    inputs = load_optimizer_inputs(
        ds4_path=args.ds4,
        feedback_manifest_path=args.feedback_manifest,
        taxonomy_path=args.taxonomy,
        seed_prompt_path=args.seed_prompt,
    )
    output_dir = args.output_dir if args.output_dir is not None else default_output_dir()
    config = GEPARunConfig(
        output_dir=output_dir,
        max_metric_calls=args.max_metric_calls,
        reflection_minibatch_size=args.reflection_minibatch_size,
        seed=args.seed,
        row_limit=args.row_limit,
        harness=_harness_config(args),
    )
    return json.dumps(run_gepa(inputs=inputs, config=config), indent=2, sort_keys=True)


def _add_input_args(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--ds4", type=Path, default=DEFAULT_DS4_PATH)
    parser.add_argument("--feedback-manifest", type=Path, default=DEFAULT_FEEDBACK_MANIFEST_PATH)
    parser.add_argument("--taxonomy", type=Path, default=DEFAULT_TAXONOMY_PATH)
    parser.add_argument("--seed-prompt", type=Path, default=DEFAULT_SEED_PROMPT_PATH)


def _add_harness_args(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--model", default=DEFAULT_MODEL)
    parser.add_argument("--max-tokens", type=int, default=DEFAULT_MAX_TOKENS)
    parser.add_argument("--timeout-ms", type=int, default=900_000)
    parser.add_argument("--base-url", default=None)
    parser.add_argument("--context-window", type=int, default=None)
    parser.add_argument("--state-dir", type=Path, default=None)


def _harness_config(args: argparse.Namespace) -> HarnessConfig:
    return HarnessConfig(
        model=args.model,
        concurrency=args.concurrency,
        max_tokens=args.max_tokens,
        timeout_ms=args.timeout_ms,
        base_url=args.base_url,
        context_window=args.context_window,
        state_dir=args.state_dir,
    )


def die(message: str) -> NoReturn:
    raise SystemExit(message)


if __name__ == "__main__":
    main()
