# OpenClaw Routing Classifier Repo-Read Test

Classify one OpenClaw GitHub issue or pull request for maintainer notification
routing. Return only the final structured JSON required by the schema.

Required output shape:

```json
{"topics_of_interest":[],"description":"One concise evidence-backed sentence.","caveats":[]}
```

## Target

`__TARGET__`

## GitHub Context

__GITHUB_CONTEXT__

Use this context as source of truth, but for this test run you should make one
concise read-only `bash` call when repo files can provide relevant evidence.
Prefer `pwd`, `ls`, `find`, `rg`, `grep`, `sed -n`, `cat`, `head`, `git show
--name-only`, `git ls-files`, `git grep`, `tail`, or `wc -l`. Keep the command
small and do not use `bash` for anything except reading/searching repo files.
For repo-wide text search, use `rg -n -i "phrase"` or explicit recursive grep
such as `grep -R -n -i "phrase" .`. For file discovery, use
`rg --files -g "*.ts"` or `git ls-files src`.

## Allowed Topics

```json
__ALLOWED_TOPICS_JSON__
```

Topic definitions and cue words:

__TOPIC_DESCRIPTIONS__

Before final output, delete `local_model_providers` unless the item explicitly
centers a local, self-hosted, or user-declared OpenAI-compatible backend such as
LM Studio, Ollama, vLLM, LocalAI, llama.cpp, Atomic Chat, localhost/LAN, or
private inference servers. Never use it for hosted provider catalog updates,
static model catalog entries, hosted provider manifests, hosted model
availability, hosted OAuth/keychain issues, usage/billing UI,
provider-specific TTS/speech/image behavior, or hosted API behavior.
Hosted model catalog updates are `model_releases` and sometimes `config`; never
add `local_model_providers` as a secondary label for Anthropic/Claude CLI,
OpenAI, Gemini/Vertex, Copilot, Kimi/Moonshot, or Volcengine static catalog
entries.
Hosted provider usage, balance, quota, cost, billing, and status-display work is
`telemetry_usage` or `ui_tui`; never add `local_model_providers` for
Kimi/Moonshot, Anthropic, OpenAI, Gemini/Vertex, Copilot, Volcengine, MiniMax,
or ElevenLabs usage/billing UI.

Use `local_models` only for concrete local/offline model execution. Do not use
it just because an item mentions a model ID, model catalog, model list, static
model entry, provider manifest, or hosted provider availability. Keep it for
local-model compatibility, local-model lean filtering, and local-model runtime
crashes.

Choose the smallest useful topic set. Prefer zero or one topic, add a second
only when the item would be misrouted without it, and use `caveats` for missing
or limited evidence.
