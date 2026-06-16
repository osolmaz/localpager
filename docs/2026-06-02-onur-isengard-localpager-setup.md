---
title: Onur Isengard Localpager Setup
date: 2026-06-02
---

# Onur Isengard Localpager Setup

This document records Onur's current Localpager deployment on `isengard`.
Generic examples in the repo should stay conservative; this file is the source
of truth for this machine's OpenClaw/Gemma setup.

## Machine

- Host: `isengard`
- Architecture: `aarch64`
- Kernel family: Ubuntu NVIDIA kernel
- CPU: 20 online ARM cores
- GPU reported by `nvidia-smi`: `NVIDIA GB10`
- RAM: `119Gi`
- Local model server: LM Studio on `http://127.0.0.1:1234/v1`

Operational rule: do not keep DS4/DeepSeek and LM Studio/Gemma loaded at the
same time on this machine.

## Live Files

- Repo: `/home/bob/repos/localpager`
- Live config: `/home/bob/.config/localpager/config.json`
- Live prompt: `/home/bob/.config/localpager/openclaw.prompt.md`
- Topic taxonomy: `/home/bob/repos/localpager/examples/profiles/openclaw-routing-topics.json`
- SQLite state: `/home/bob/.local/state/localpager/localpager.sqlite`
- Classifier artifacts: `/home/bob/.local/state/localpager/classifier`
- Reposhell state: `/home/bob/.local/state/localpager/reposhell`
- Reposhell socket: `/home/bob/.local/state/localpager/reposhell.sock`
- Service: `localpager-worker.service`
- Reposhell service: `localpager-reposhell.service`
- Enqueue timer: `localpager-enqueue-github.timer`

Secrets live outside the repo. Do not commit `secrets.env`, tokens, SQLite
state, classifier prompts, or session artifacts.

## Current Runtime

LM Studio should show:

```text
IDENTIFIER        MODEL             CONTEXT    PARALLEL
gemma-4-e4b-it    gemma-4-e4b-it    131072     3
```

Localpager should show:

```text
model=gemma-4-e4b-it
agent_base_url=http://127.0.0.1:1234/v1
agent_context_window=131072
agent_max_tokens=768
agent_timeout_ms=5000
model_unavailable_retry_delay=5m
```

The worker concurrency should match the LM Studio load:

```json
{
  "worker": {
    "max_concurrency": 3,
    "model": "gemma-4-e4b-it",
    "agent_base_url": "http://127.0.0.1:1234/v1",
    "agent_context_window": 131072,
    "agent_max_tokens": 768,
    "agent_timeout_ms": 5000,
    "model_unavailable_retry_delay": "5m"
  }
}
```

The OpenClaw GitHub context budget is:

```json
{
  "classifier": {
    "context": {
      "github": {
        "include_body": true,
        "include_labels": true,
        "include_comments": true,
        "include_changed_files": true,
        "include_diff": true,
        "max_body_chars": 2500,
        "max_comments_chars": 1500,
        "max_changed_files_chars": 2000,
        "max_diff_chars": 5000
      }
    }
  }
}
```

The reposhell setup is:

```json
{
  "reposhell": {
    "enabled": true,
    "root": "~/.local/state/localpager/reposhell",
    "socket": "~/.local/state/localpager/reposhell.sock",
    "refresh_interval": "24h",
    "command_timeout": "2s",
    "max_output_bytes": 65536,
    "repos": [
      {
        "id": "openclaw",
        "remote": "https://github.com/openclaw/openclaw.git",
        "default_ref": "origin/main",
        "refresh_interval": "24h"
      }
    ]
  },
  "classifier": {
    "reposhell_default_repo": "openclaw",
    "reposhell_visible_repos": ["openclaw"]
  }
}
```

Customize it by changing `reposhell.repos` for the repositories the service
syncs, then changing `classifier.reposhell_default_repo` and
`classifier.reposhell_visible_repos` for the repos a classifier profile may
read. The classifier's read-only `bash` tool turns on automatically when the
reposhell service is enabled and a socket is configured; `localpager-agent`
always exposes `final_json`. The default refresh interval is one day; override
it per repo only when a repo needs faster or slower updates.

## Verify State

Run:

```bash
lms ps
localpager validate --config /home/bob/.config/localpager/config.json
localpager status --config /home/bob/.config/localpager/config.json
systemctl --user is-active localpager-worker.service localpager-enqueue-github.timer
systemctl --user is-active localpager-reposhell.service
reposhell status --config /home/bob/.config/localpager/config.json
reposhell shell --config /home/bob/.config/localpager/config.json --repo openclaw --visible-repo openclaw
pgrep -af 'ds4|deepseek' || true
```

Expected:

- `lms ps` reports Gemma with `CONTEXT 131072` and `PARALLEL 3`.
- `localpager validate` reports `config_ok=true`.
- `localpager status` reports `agent_context_window=131072`.
- The worker, reposhell service, and enqueue timer are `active`.
- `reposhell status` returns JSON rather than a socket error.
- `reposhell shell` opens an interactive read-only prompt rooted at
  `/repo/openclaw`; type `exit` to leave.
- The `pgrep` command does not show a real DS4/DeepSeek server process.

## Change Model Context Or Parallelism

Use this when changing LM Studio context length, model parallelism, or
Localpager concurrency.

1. Stop the worker:

   ```bash
   systemctl --user stop localpager-worker.service
   ```

2. Confirm DS4/DeepSeek is not running:

   ```bash
   pgrep -af 'ds4|deepseek' || true
   ```

3. Estimate the new LM Studio load:

   ```bash
   lms load gemma-4-e4b-it \
     --context-length 131072 \
     --parallel 3 \
     --estimate-only \
     -y
   ```

4. Reload Gemma with the chosen values:

   ```bash
   lms unload gemma-4-e4b-it
   lms load gemma-4-e4b-it --context-length 131072 --parallel 3 -y
   ```

5. Edit `/home/bob/.config/localpager/config.json` so these match:

   ```json
   {
     "worker": {
       "max_concurrency": 3,
       "agent_context_window": 131072
     }
   }
   ```

6. Validate and restart:

   ```bash
   localpager validate --config /home/bob/.config/localpager/config.json
   systemctl --user start localpager-worker.service
   ```

7. Verify:

   ```bash
   lms ps
   localpager status --config /home/bob/.config/localpager/config.json
   ```

Rule of thumb: if LM Studio is loaded with `--parallel N`, keep
`worker.max_concurrency` at or below `N` unless there is a deliberate queueing
reason. If the model context length changes, update both the LM Studio load
command and `worker.agent_context_window`.

## Switch To vLLM Qwen NVFP4

Use `docs/2026-06-16-vllm-qwen36-nvfp4-localpager.md` for the reusable setup.
The short version is:

1. Start the vLLM server:

   ```bash
   /home/bob/.config/localpager/start-vllm-qwen36-nvfp4.sh
   ```

2. Point Localpager at the OpenAI-compatible vLLM endpoint:

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

3. Restart the worker:

   ```bash
   systemctl --user restart localpager-worker.service
   ```

For this setup, vLLM is one model per server process. `worker.max_concurrency`
should stay at or below the vLLM `--max-num-seqs` value. The Qwen NVFP4 c4
profile uses `--max-num-seqs 4`.

## Change GitHub Context Budget

Edit only this block in `/home/bob/.config/localpager/config.json`:

```json
{
  "classifier": {
    "context": {
      "github": {
        "max_body_chars": 2500,
        "max_comments_chars": 1500,
        "max_changed_files_chars": 2000,
        "max_diff_chars": 5000
      }
    }
  }
}
```

Increase these values when the model has enough context and the classifier is
missing evidence. Decrease them when prompts are too large, latency is too high,
or the model fails to return structured output. After changing the budget:

```bash
localpager validate --config /home/bob/.config/localpager/config.json
systemctl --user restart localpager-worker.service
```

## Change Paging Topics

The taxonomy controls what the classifier may output. The notification filter
controls what pages Onur.

For this setup, the full taxonomy is:

```text
/home/bob/repos/localpager/examples/profiles/openclaw-routing-topics.json
```

The paging filter is:

```json
{
  "worker": {
    "notify_topics_any": [
      "local_models",
      "self_hosted_inference",
      "local_model_providers",
      "open_weight_models",
      "acpx"
    ]
  }
}
```

To customize, add or remove topic IDs from `notify_topics_any`. Do not remove
topics from the taxonomy just to change paging behavior.

## Run DS4 Experiments Safely

DS4 uses a different endpoint and must not be loaded beside LM Studio/Gemma.

Before starting DS4:

```bash
systemctl --user stop localpager-worker.service
lms unload gemma-4-e4b-it
pgrep -af 'ds4|deepseek' || true
```

Then start DS4 through its normal workflow and run the experiment with DS4's
endpoint, typically `http://127.0.0.1:8000/v1`.

After DS4 work, stop DS4, then restore Gemma:

```bash
pgrep -af 'ds4|deepseek' || true
lms load gemma-4-e4b-it --context-length 131072 --parallel 3 -y
systemctl --user start localpager-worker.service
```

## Requeue After Runtime Changes

If stopping the worker leaves stale leases, check for active classifier
processes before requeueing:

```bash
pgrep -fc '/home/bob/repos/localpager/scripts/localpager-classifier'
localpager requeue-jobs \
  --config /home/bob/.config/localpager/config.json \
  --status running \
  --dry-run
```

If there are no active classifier processes and the dry-run count is only stale
leases:

```bash
localpager requeue-jobs \
  --config /home/bob/.config/localpager/config.json \
  --status running
```

For dead jobs caused by transient endpoint failures:

```bash
localpager requeue-jobs \
  --config /home/bob/.config/localpager/config.json \
  --status dead \
  --last-error-contains "fetch failed"
```

Do not automatically requeue `final_json was not called` failures without
reviewing the prompt/session artifact first; those are structured-output
reliability failures, not endpoint-down failures.
