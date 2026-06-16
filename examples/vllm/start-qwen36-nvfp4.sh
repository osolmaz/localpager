#!/usr/bin/env bash
set -euo pipefail

profile="${1:-${HOME}/.config/localpager/vllm-qwen36-nvfp4.env}"

set -a
source "$profile"
set +a

cd "$VLLM_WORKDIR"
source "$VLLM_VENV/bin/activate"

exec vllm serve "$VLLM_MODEL" \
  --host "$VLLM_HOST" \
  --port "$VLLM_PORT" \
  --trust-remote-code \
  --quantization modelopt \
  --kv-cache-dtype fp8 \
  --moe-backend flashinfer_b12x \
  --gpu-memory-utilization "$VLLM_GPU_MEMORY_UTILIZATION" \
  --max-model-len "$VLLM_MAX_MODEL_LEN" \
  --max-num-seqs "$VLLM_MAX_NUM_SEQS" \
  --max-num-batched-tokens "$VLLM_MAX_NUM_BATCHED_TOKENS" \
  --enable-prefix-caching \
  --reasoning-parser "$VLLM_REASONING_PARSER" \
  --enable-auto-tool-choice \
  --tool-call-parser "$VLLM_TOOL_CALL_PARSER" \
  --no-enable-flashinfer-autotune
