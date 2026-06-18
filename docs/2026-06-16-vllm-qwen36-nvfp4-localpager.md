---
title: vLLM Qwen NVFP4 LocalPager Setup
author: Bob <dutifulbob@gmail.com>
date: 2026-06-16
---

# vLLM Qwen NVFP4 LocalPager Setup

This records the recommended shape for serving
`nvidia/Qwen3.6-35B-A3B-NVFP4` to LocalPager through vLLM.

vLLM is not a model manager like LM Studio. The normal setup is one vLLM server
process per loaded model:

```bash
vllm serve nvidia/Qwen3.6-35B-A3B-NVFP4 --port 8000 ...
```

To switch models, stop that process and start another one, or run a second vLLM
server on another port if the machine has enough memory.

## Files

- `examples/vllm/qwen36-nvfp4.env.example`: copyable vLLM profile.
- `examples/vllm/start-qwen36-nvfp4.sh`: launcher that reads the profile.
- `examples/vllm/localpager-worker-qwen36-nvfp4.snippet.json`: LocalPager
  worker config snippet.
- `examples/vllm/benchmark-openai-completions.mjs`: reusable throughput probe
  for the OpenAI-compatible endpoint.
- `examples/systemd/localpager-vllm-qwen36-nvfp4.service`: optional user
  systemd service.

## Manual Start

Copy the profile and edit paths if needed:

```bash
mkdir -p ~/.config/localpager
cp examples/vllm/qwen36-nvfp4.env.example ~/.config/localpager/vllm-qwen36-nvfp4.env
```

Start the server:

```bash
examples/vllm/start-qwen36-nvfp4.sh ~/.config/localpager/vllm-qwen36-nvfp4.env
```

Check the OpenAI-compatible endpoint:

```bash
curl -s http://127.0.0.1:8000/v1/models | jq -r '.data[0].id'
```

Expected:

```text
nvidia/Qwen3.6-35B-A3B-NVFP4
```

## LocalPager Config

Merge this into `~/.config/localpager/config.json`:

```json
{
  "worker": {
    "max_concurrency": 4,
    "model": "nvidia/Qwen3.6-35B-A3B-NVFP4",
    "agent_base_url": "http://127.0.0.1:8000/v1",
    "agent_context_window": 32768,
    "agent_max_tokens": 4096,
    "agent_timeout_ms": 120000
  }
}
```

`worker.max_concurrency` should stay at or below the vLLM server's
`--max-num-seqs`. For this profile both are `4`.

Use `4096` output tokens for classifier runs that need robust structured output.
Shorter limits can create structural failures that are not prompt failures.

To measure aggregate completion-token throughput against the running server:

```bash
node examples/vllm/benchmark-openai-completions.mjs \
  --base-url http://127.0.0.1:8000/v1 \
  --model nvidia/Qwen3.6-35B-A3B-NVFP4 \
  --concurrency 4 \
  --requests 4 \
  --max-tokens 512
```

## Tool Choice

LocalPi may send OpenAI requests with `tool_choice: "auto"`. vLLM rejects those
unless the server was started with both flags:

```bash
--enable-auto-tool-choice --tool-call-parser qwen3_xml
```

The example launcher includes both flags.

## User Service

Install the optional service:

```bash
mkdir -p ~/.config/systemd/user
cp examples/systemd/localpager-vllm-qwen36-nvfp4.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now localpager-vllm-qwen36-nvfp4.service
```

Then restart LocalPager after vLLM is up:

```bash
systemctl --user restart localpager-worker.service
```

For stricter ordering, add a user-level override for
`localpager-worker.service`:

```ini
[Unit]
After=localpager-vllm-qwen36-nvfp4.service
Wants=localpager-vllm-qwen36-nvfp4.service
```

## Current Measured Envelope

On the GB10/cu130 nightly setup, the memory-bounded c4 profile ran without an
OOM. The observed throughput varied when other tests were running, so treat
these as local smoke numbers rather than a benchmark guarantee:

| concurrency | per-stream observed | aggregate observed |
| --- | ---: | ---: |
| 1 | 27-73 tok/s | 27-73 tok/s |
| 2 | 27-58 tok/s | 54-116 tok/s |
| 3 | 24-45 tok/s | 71-136 tok/s |
| 4 | 24-46 tok/s | 95-182 tok/s |

The setup is conservative: `gpu-memory-utilization=0.65`,
`max-model-len=32768`, `max-num-seqs=4`, and `max-num-batched-tokens=8192`.
