# Gemma 4 26B-A4B NVFP4 on DGX Spark

Local benchmark date: 2026-06-17.

## Recommended Profile

Model:

```text
nvidia/Gemma-4-26B-A4B-NVFP4
```

vLLM:

```text
0.23.1rc1.dev49+ga7fdfeef7
```

Serving settings for text-only LocalPager classification:

```text
--host 127.0.0.1
--port 8000
--trust-remote-code
--gpu-memory-utilization 0.65
--max-model-len 8192
--max-num-seqs 32
--max-num-batched-tokens 8192
--enable-prefix-caching
--reasoning-parser gemma4
--enable-auto-tool-choice
--tool-call-parser gemma4
--kv-cache-dtype fp8
--mm-processor-cache-gb 0
--moe-backend cutlass
--language-model-only
--no-enable-flashinfer-autotune
```

The critical setting is `--moe-backend cutlass`. With `auto`, this vLLM build
selected `FLASHINFER_CUTLASS`, and startup hit the service memory cap while
compiling FlashInfer generated CUDA kernels. With `cutlass`, the log confirms:

```text
Using 'VLLM_CUTLASS' NvFp4 MoE backend
```

## Files

- `examples/vllm/gemma4-26b-a4b-nvfp4.env.example`: copyable vLLM profile.
- `examples/vllm/start-nvfp4.sh`: launcher that reads the profile.
- `examples/vllm/localpager-worker-gemma4-26b-a4b-nvfp4.snippet.json`:
  LocalPager worker config snippet for this server.
- `examples/systemd/localpager-vllm-gemma4-26b-a4b-nvfp4.service`: optional
  user systemd service with the same memory cap used during local testing.
- `examples/vllm/benchmark-openai-completions.mjs`: small OpenAI-compatible
  throughput probe for reproducing the completion-token numbers.

## Throughput Results

Local OpenAI-compatible API, `max_tokens=512`, `temperature=0`, 37 prompt
tokens per request:

| Profile | Concurrent Requests | Total Completion Tokens | Wall Time | Aggregate Tok/s | Per-request Tok/s |
| --- | ---: | ---: | ---: | ---: | ---: |
| `seqs=1`, eager, `batched_tokens=1024` | 1 | 512 | 20.089s | 25.486 | 25.486 |
| `seqs=1`, no eager, `batched_tokens=1024` | 1 | 512 | 18.179s | 28.165 | 28.165 |
| `seqs=2`, no eager, `batched_tokens=2048` | 2 | 1024 | 16.050s | 63.802 | 31.906 |
| `seqs=4`, no eager, `batched_tokens=4096` | 4 | 2048 | 17.634s | 116.139 | 29.038-29.084 |
| `seqs=8`, no eager, `batched_tokens=8192` | 8 | 4096 | 18.227s | 224.721 | 28.093-28.144 |
| `seqs=16`, no eager, `batched_tokens=8192` | 16 | 8192 | 20.346s | 402.630 | 25.167-25.218 |
| `seqs=32`, no eager, `batched_tokens=8192` | 32 | 16384 | 21.684s | 755.586 | 23.615-23.677 |

The recommended `seqs=32` service reported ready after 180s. It stayed below
the `MemoryMax=90G` user-service cap during startup and benchmark runs.

Reproduce the simple throughput probe against a running server:

```bash
node examples/vllm/benchmark-openai-completions.mjs \
  --base-url http://127.0.0.1:8000/v1 \
  --model nvidia/Gemma-4-26B-A4B-NVFP4 \
  --concurrency 32 \
  --requests 32 \
  --max-tokens 512
```

For LocalPager classifier runs, keep `worker.max_concurrency` at or below the
server `--max-num-seqs` value and keep `worker.agent_max_tokens` at `4096` for
robust structured output.

## Failed Profiles

These profiles were tested under a `MemoryMax=90G` systemd user-service cap and
were killed by the cgroup OOM killer:

| Profile | Result |
| --- | --- |
| `max_num_seqs=4`, `max_num_batched_tokens=8192`, `max_model_len=32768` | OOM during startup |
| `max_num_seqs=1`, `max_num_batched_tokens=4096`, `max_model_len=32768` | Guard stopped before OOM |
| `max_num_seqs=1`, `max_num_batched_tokens=3072`, `max_model_len=32768` | Guard stopped before OOM |
| `max_num_seqs=1`, `max_num_batched_tokens=2496`, `max_model_len=8192` | OOM with `FLASHINFER_CUTLASS` |
| `language_model_only=1`, `max_num_batched_tokens=1024` | OOM with `FLASHINFER_CUTLASS` |
| `enforce_eager=1`, `language_model_only=1`, `max_num_batched_tokens=1024` | OOM with `FLASHINFER_CUTLASS` |
| `mm_processor_cache_gb=0`, `enforce_eager=1`, `language_model_only=1` | OOM with `FLASHINFER_CUTLASS` |

The failed runs show that lowering concurrency and disabling multimodal serving
were not sufficient by themselves. The backend selection was the deciding
factor.
