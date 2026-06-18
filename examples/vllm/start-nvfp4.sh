#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 /path/to/vllm-profile.env" >&2
  exit 2
fi

profile="$1"

source "$profile"

if [[ -n "${FLASHINFER_DISABLE_VERSION_CHECK:-}" ]]; then
  export FLASHINFER_DISABLE_VERSION_CHECK
fi
if [[ -n "${CUTE_DSL_ARCH:-}" ]]; then
  export CUTE_DSL_ARCH
fi

cd "$VLLM_WORKDIR"
source "$VLLM_VENV/bin/activate"

args=(
  serve "$VLLM_MODEL"
  --host "$VLLM_HOST"
  --port "$VLLM_PORT"
  --trust-remote-code
  --gpu-memory-utilization "$VLLM_GPU_MEMORY_UTILIZATION"
  --max-model-len "$VLLM_MAX_MODEL_LEN"
  --max-num-seqs "$VLLM_MAX_NUM_SEQS"
  --max-num-batched-tokens "$VLLM_MAX_NUM_BATCHED_TOKENS"
  --enable-prefix-caching
  --reasoning-parser "$VLLM_REASONING_PARSER"
  --enable-auto-tool-choice
  --tool-call-parser "$VLLM_TOOL_CALL_PARSER"
)

if [[ -n "${VLLM_QUANTIZATION:-}" ]]; then
  args+=(--quantization "$VLLM_QUANTIZATION")
fi

if [[ -n "${VLLM_KV_CACHE_DTYPE:-}" ]]; then
  args+=(--kv-cache-dtype "$VLLM_KV_CACHE_DTYPE")
fi

if [[ -n "${VLLM_MM_PROCESSOR_CACHE_GB:-}" ]]; then
  args+=(--mm-processor-cache-gb "$VLLM_MM_PROCESSOR_CACHE_GB")
fi

if [[ "${VLLM_ENFORCE_EAGER:-0}" == "1" ]]; then
  args+=(--enforce-eager)
fi

if [[ -n "${VLLM_MOE_BACKEND:-}" ]]; then
  args+=(--moe-backend "$VLLM_MOE_BACKEND")
fi

if [[ "${VLLM_LANGUAGE_MODEL_ONLY:-0}" == "1" ]]; then
  args+=(--language-model-only)
fi

if [[ "${VLLM_SKIP_MM_PROFILING:-0}" == "1" ]]; then
  args+=(--skip-mm-profiling)
fi

if [[ "${VLLM_DISABLE_FLASHINFER_AUTOTUNE:-0}" == "1" ]]; then
  args+=(--no-enable-flashinfer-autotune)
fi

exec vllm "${args[@]}"
