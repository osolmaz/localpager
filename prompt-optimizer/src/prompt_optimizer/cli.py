from __future__ import annotations

import argparse
import json
from dataclasses import asdict, replace
from pathlib import Path
from typing import NoReturn

from prompt_optimizer.dataset import (
    DEFAULT_DS4_PATH,
    DEFAULT_EVALSTATE_HELDOUT_PATH,
    DEFAULT_EVALSTATE_PARETO_PATH,
    DEFAULT_EVALSTATE_TRAIN_PATH,
    DEFAULT_FEEDBACK_MANIFEST_PATH,
    DEFAULT_TAXONOMY_PATH,
    DEFAULT_V2_TAXONOMY_PATH,
    load_taxonomy,
)
from prompt_optimizer.harness import ClassifierHarness
from prompt_optimizer.metric import SCORING_CONFIG
from prompt_optimizer.prompt import (
    DEFAULT_EVALSTATE_SEED_PROMPT_PATH,
    DEFAULT_SEED_PROMPT_PATH,
    load_seed_prompt,
)
from prompt_optimizer.report import summarize_evaluation_file, summarize_gepa_run, write_gepa_run_report
from prompt_optimizer.run import (
    DEFAULT_BASE_URL,
    DEFAULT_CONCURRENCY,
    DEFAULT_MAX_TOKENS,
    DEFAULT_MODEL,
    GEPARunConfig,
    HarnessConfig,
    default_output_dir,
    evaluate_seed,
    evaluate_routing_policy,
    load_evalstate_optimizer_inputs,
    load_optimizer_inputs,
    localpager_agent_harness,
    run_gepa,
    static_empty_harness,
)


def main() -> None:
    parser = argparse.ArgumentParser(prog="localpager-prompt-optimizer")
    subparsers = parser.add_subparsers(dest="command")

    summary = subparsers.add_parser("summary", help="print deterministic input summary")
    _add_input_args(summary)

    evaluate = subparsers.add_parser("evaluate-seed", help="evaluate the seed prompt without optimizing")
    _add_input_args(evaluate)
    _add_eval_split_arg(evaluate)
    evaluate.add_argument("--limit", type=int, default=1, help="number of feedback rows to evaluate")
    evaluate.add_argument("--offset", type=int, default=0, help="number of feedback rows to skip first")
    evaluate.add_argument("--concurrency", type=int, default=DEFAULT_CONCURRENCY)
    evaluate.add_argument(
        "--harness",
        choices=("static-empty", "localpager-agent"),
        default="static-empty",
        help="static-empty is no-model and localpager-agent runs the production wrapper",
    )
    _add_harness_args(evaluate)

    evaluate_candidate = subparsers.add_parser(
        "evaluate-candidate",
        help="evaluate a routing_policy candidate without optimizing",
    )
    _add_input_args(evaluate_candidate)
    _add_eval_split_arg(evaluate_candidate)
    evaluate_candidate.add_argument("--routing-policy", type=Path, required=True)
    evaluate_candidate.add_argument("--candidate-name", default="candidate")
    evaluate_candidate.add_argument("--limit", type=int, default=1, help="number of feedback rows to evaluate")
    evaluate_candidate.add_argument("--offset", type=int, default=0, help="number of feedback rows to skip first")
    evaluate_candidate.add_argument("--concurrency", type=int, default=DEFAULT_CONCURRENCY)
    evaluate_candidate.add_argument(
        "--harness",
        choices=("static-empty", "localpager-agent"),
        default="static-empty",
        help="static-empty is no-model and localpager-agent runs the production wrapper",
    )
    _add_harness_args(evaluate_candidate)

    optimize = subparsers.add_parser("optimize", help="run GEPA with the localpager-agent harness")
    _add_input_args(optimize)
    optimize.add_argument("--output-dir", type=Path, default=None)
    optimize.add_argument("--row-limit", type=int, default=None)
    optimize.add_argument("--max-metric-calls", type=int, required=True)
    optimize.add_argument("--max-candidate-proposals", type=int, default=None)
    optimize.add_argument("--reflection-minibatch-size", type=int, default=4)
    optimize.add_argument("--seed", type=int, default=0)
    optimize.add_argument("--seed-routing-policy", type=Path, default=None)
    optimize.add_argument("--concurrency", type=int, default=DEFAULT_CONCURRENCY)
    _add_harness_args(optimize)

    report_run = subparsers.add_parser("report-run", help="summarize a GEPA run directory")
    report_run.add_argument("--run-dir", type=Path, required=True)

    plot_run = subparsers.add_parser("plot-run", help="write an HTML GEPA run score report")
    plot_run.add_argument("--run-dir", type=Path, required=True)
    plot_run.add_argument("--output", type=Path, default=None)

    summarize_eval = subparsers.add_parser("summarize-evaluation", help="summarize an evaluation JSON artifact")
    summarize_eval.add_argument("--evaluation", type=Path, required=True)

    args = parser.parse_args()
    if args.command == "summary":
        print(_summary(args))
        return
    if args.command == "evaluate-seed":
        print(_evaluate_seed(args))
        return
    if args.command == "evaluate-candidate":
        print(_evaluate_candidate(args))
        return
    if args.command == "optimize":
        print(_optimize(args))
        return
    if args.command == "report-run":
        print(json.dumps(summarize_gepa_run(args.run_dir), indent=2, sort_keys=True))
        return
    if args.command == "plot-run":
        print(str(write_gepa_run_report(args.run_dir, args.output)))
        return
    if args.command == "summarize-evaluation":
        print(json.dumps(summarize_evaluation_file(args.evaluation), indent=2, sort_keys=True))
        return
    parser.print_help()


def _summary(args: argparse.Namespace) -> str:
    inputs = _load_inputs(args)
    taxonomy = _taxonomy_path(args)
    seed_prompt = _seed_prompt_path(args)
    topics = load_taxonomy(taxonomy)
    prompt = load_seed_prompt(seed_prompt)
    payload = {
        "dataset": args.dataset,
        "dataset_name": inputs.dataset_name,
        "train_rows": len(inputs.rows),
        "pareto_rows": len(inputs.pareto_rows),
        "heldout_rows": len(inputs.heldout_rows),
        "taxonomy_path": str(taxonomy),
        "taxonomy_topics": len(topics),
        "seed_prompt_path": str(seed_prompt),
        "seed_prompt_sha256": prompt.template_sha256,
        "routing_policy_sha256": prompt.routing_policy_sha256,
        "scoring": asdict(SCORING_CONFIG),
        "live_harness_defaults": {
            "model": DEFAULT_MODEL,
            "base_url": DEFAULT_BASE_URL,
            "concurrency": DEFAULT_CONCURRENCY,
            "thinking": "medium",
            "max_tokens": DEFAULT_MAX_TOKENS,
        },
    }
    if args.dataset == "ds4":
        payload["ds4_path"] = str(args.ds4)
        payload["feedback_manifest_path"] = str(args.feedback_manifest)
    else:
        payload["train_split"] = str(args.train_split)
        payload["pareto_split"] = str(args.pareto_split)
        payload["heldout_split"] = str(args.heldout_split)
    return json.dumps(payload, indent=2, sort_keys=True)


def _evaluate_seed(args: argparse.Namespace) -> str:
    inputs = _inputs_for_eval(args)
    payload = evaluate_seed(
        inputs=inputs,
        harness=_evaluation_harness(args),
        concurrency=args.concurrency,
        limit=args.limit,
        offset=args.offset,
    )
    payload["harness"] = args.harness
    payload["concurrency"] = args.concurrency
    payload["offset"] = args.offset
    payload["eval_split"] = args.eval_split
    return json.dumps(payload, indent=2, sort_keys=True)


def _evaluate_candidate(args: argparse.Namespace) -> str:
    inputs = _inputs_for_eval(args)
    routing_policy = args.routing_policy.read_text(encoding="utf-8")
    payload = evaluate_routing_policy(
        inputs=inputs,
        routing_policy=routing_policy,
        harness=_evaluation_harness(args),
        concurrency=args.concurrency,
        limit=args.limit,
        offset=args.offset,
        candidate_name=args.candidate_name,
    )
    payload["harness"] = args.harness
    payload["concurrency"] = args.concurrency
    payload["offset"] = args.offset
    payload["eval_split"] = args.eval_split
    payload["routing_policy_path"] = str(args.routing_policy)
    return json.dumps(payload, indent=2, sort_keys=True)


def _optimize(args: argparse.Namespace) -> str:
    inputs = _load_inputs(args)
    output_dir = args.output_dir if args.output_dir is not None else default_output_dir()
    config = GEPARunConfig(
        output_dir=output_dir,
        max_metric_calls=args.max_metric_calls,
        max_candidate_proposals=args.max_candidate_proposals,
        reflection_minibatch_size=args.reflection_minibatch_size,
        seed=args.seed,
        row_limit=args.row_limit,
        dataset_name=inputs.dataset_name,
        seed_routing_policy=(
            args.seed_routing_policy.read_text(encoding="utf-8")
            if args.seed_routing_policy is not None
            else None
        ),
        harness=_harness_config(args),
    )
    return json.dumps(run_gepa(inputs=inputs, config=config), indent=2, sort_keys=True)


def _evaluation_harness(args: argparse.Namespace) -> ClassifierHarness:
    if args.harness == "static-empty":
        return static_empty_harness()
    return localpager_agent_harness(_harness_config(args))


def _add_input_args(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--dataset", choices=("ds4", "evalstate"), default="ds4")
    parser.add_argument("--ds4", type=Path, default=DEFAULT_DS4_PATH)
    parser.add_argument("--feedback-manifest", type=Path, default=DEFAULT_FEEDBACK_MANIFEST_PATH)
    parser.add_argument("--train-split", type=Path, default=DEFAULT_EVALSTATE_TRAIN_PATH)
    parser.add_argument("--pareto-split", type=Path, default=DEFAULT_EVALSTATE_PARETO_PATH)
    parser.add_argument("--heldout-split", type=Path, default=DEFAULT_EVALSTATE_HELDOUT_PATH)
    parser.add_argument("--taxonomy", type=Path, default=None)
    parser.add_argument("--seed-prompt", type=Path, default=None)


def _add_eval_split_arg(parser: argparse.ArgumentParser) -> None:
    parser.add_argument(
        "--eval-split",
        choices=("train", "pareto", "heldout"),
        default="train",
        help="which loaded split to evaluate for evaluate-* commands",
    )


def _add_harness_args(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--model", default=DEFAULT_MODEL)
    parser.add_argument("--max-tokens", type=int, default=DEFAULT_MAX_TOKENS)
    parser.add_argument("--thinking", default="medium")
    parser.add_argument("--timeout-ms", type=int, default=900_000)
    parser.add_argument("--base-url", default=DEFAULT_BASE_URL)
    parser.add_argument("--context-window", type=int, default=None)
    parser.add_argument("--state-dir", type=Path, default=None)


def _harness_config(args: argparse.Namespace) -> HarnessConfig:
    return HarnessConfig(
        model=args.model,
        concurrency=args.concurrency,
        max_tokens=args.max_tokens,
        thinking=args.thinking,
        timeout_ms=args.timeout_ms,
        base_url=args.base_url,
        context_window=args.context_window,
        state_dir=args.state_dir,
    )


def _load_inputs(args: argparse.Namespace):
    taxonomy = _taxonomy_path(args)
    seed_prompt = _seed_prompt_path(args)
    if args.dataset == "evalstate":
        return load_evalstate_optimizer_inputs(
            train_path=args.train_split,
            pareto_path=args.pareto_split,
            heldout_path=args.heldout_split,
            taxonomy_path=taxonomy,
            seed_prompt_path=seed_prompt,
        )
    return load_optimizer_inputs(
        ds4_path=args.ds4,
        feedback_manifest_path=args.feedback_manifest,
        taxonomy_path=taxonomy,
        seed_prompt_path=seed_prompt,
    )


def _inputs_for_eval(args: argparse.Namespace):
    inputs = _load_inputs(args)
    if args.eval_split == "pareto":
        if not inputs.pareto_rows:
            die("pareto split is not loaded for this dataset")
        return replace(inputs, rows=inputs.pareto_rows)
    if args.eval_split == "heldout":
        if not inputs.heldout_rows:
            die("heldout split is not loaded for this dataset")
        return replace(inputs, rows=inputs.heldout_rows)
    return inputs


def _taxonomy_path(args: argparse.Namespace) -> Path:
    if args.taxonomy is not None:
        return args.taxonomy
    return DEFAULT_V2_TAXONOMY_PATH if args.dataset == "evalstate" else DEFAULT_TAXONOMY_PATH


def _seed_prompt_path(args: argparse.Namespace) -> Path:
    if args.seed_prompt is not None:
        return args.seed_prompt
    return DEFAULT_EVALSTATE_SEED_PROMPT_PATH if args.dataset == "evalstate" else DEFAULT_SEED_PROMPT_PATH


def die(message: str) -> NoReturn:
    raise SystemExit(message)


if __name__ == "__main__":
    main()
