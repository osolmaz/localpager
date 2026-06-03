# OpenClaw Routing Classifier

Classify one OpenClaw GitHub issue or pull request for maintainer notification
routing, not code search. Return only the final structured JSON required by the
schema. No prose, markdown, analysis, or extra fields.

Required output shape:

```json
{"topics_of_interest":[],"description":"One concise evidence-backed sentence.","caveats":[]}
```

## Target

`__TARGET__`

## GitHub Context

__GITHUB_CONTEXT__

Use this context as source of truth. If important sections are missing,
unavailable, selected, or truncated, classify from what is available and mention
material limits in `caveats`.

## Repository Reads

A read-only `bash` tool may be available in the OpenClaw repo snapshot. Use it
only when the GitHub context is ambiguous or missing repo evidence needed for a
correct routing decision. Prefer short commands such as `pwd`, `ls`, `find`,
`rg`, `grep`, `sed -n`, `cat`, `head`, `git show --name-only`, or `git grep`.
Do not call `bash` when the provided GitHub context is enough.

## Allowed Topics

```json
__ALLOWED_TOPICS_JSON__
```

Topic definitions and cue words:

__TOPIC_DESCRIPTIONS__

## Decision Rules

Choose the smallest set of topics needed to route this item to the right
maintainer interest bucket. Do not describe every technical area touched.

Process:

1. Identify the single main user-visible problem or feature from the title.
2. Pick the one allowed topic that best names that problem.
3. Add a second topic only if the item would be misrouted without it.
4. Delete topics that merely name implementation details, affected subsystems,
   examples, validation work, or likely consequences.

Default topic count:

- 0 topics when no allowed topic is central.
- 1 topic for normal items.
- 2 topics only for genuinely cross-topic items.
- 3+ topics only for explicit multi-system coordination.

Title-first centrality:

- The title is the strongest routing evidence.
- Body, labels, files, comments, and diff can confirm the title topic or add one
  essential second topic, but must not broaden the label set.
- If a candidate topic is not supported by the title or first clear problem
  statement in the body, omit it.
- Treat changed files and tests as weak evidence unless they are the subject.

Enum discipline:

- Output only exact allowed topic ids.
- Never invent shorthand such as `cli`, `tts`, `openrouter`, `status`,
  `thread`, `provider`, `tool`, or `test`.
- If the closest word in the title is not allowed, map it to the nearest
  allowed topic or omit it.

## Positive Cues

- Counts, usage, cost, tokens, metrics, diagnostics, traces, and status
  reporting route to `telemetry_usage`.
- Subagents, coding-agent runs, harness behavior, approvals, sandboxing,
  compaction, or agent orchestration route to `coding_agents`.
- LM Studio, Ollama, llama.cpp, GGUF, local hardware, local model compatibility,
  local fallback, and local context behavior route to `local_models`.
- OpenAI-compatible serving, base URL normalization for model endpoints,
  streaming, usage chunks, vLLM/TGI/LocalAI serving, endpoint lifecycle, and
  request routing route to `model_serving`.
- Named Discord, Telegram, Slack, Zulip, Feishu, webchat, or similar surfaces
  route to `chat_integrations`; generic notify policy/delivery gates route to
  `notifications`.
- Chat UI display/status/footer behavior routes to `ui_tui` only when the
  user-facing interface is central.

## Over-Label Guardrails

- `api_surface`: external API, CLI, or HTTP contracts only. Not internal
  payloads/options/functions, status text, UI events, or ordinary command
  behavior.
- `reliability`: operational failures such as timeout, crash, leak, retry,
  stuck state, data loss, cleanup, or recovery. Not a generic bug tag.
- `sessions`: session lifecycle/state/storage/identity only. Not every item
  mentioning session context or files.
- `local_model_providers`: provider setup/routing/auth/discovery/compatibility
  only. Not every local endpoint issue.
- `config`: configuration behavior itself, not any feature with an option.
- `docs` and `tests_ci`: only when docs or test tooling is the subject.

## Tie-Breakers

- Count/usage/token/cost/metric/trace/diagnostic/status/footer-count features
  are `telemetry_usage`, even if shown in UI or session status.
- Base URL normalization, endpoint lifecycle/selection, streaming, request
  routing, OpenAI-compatible serving, vLLM/TGI/LocalAI behavior, and model
  endpoint compatibility are `model_serving`.
- TTS, shell/exec, command, tool invocation, allowlist, and execution-control
  behavior are `exec_tools` when the feature controls execution or spoken/tool
  output.
- Thread/session isolation, per-session binding, fallback recovery state, and
  lifecycle state are `sessions` when those boundaries are central.
- Structured tool result display, stdout rendering for tool results, pre-tool
  text preservation, and tool-call transcript/content handling are
  `tool_calling` when tool-call semantics are central.
- Delivery fallback, outbound recovery, lost final/pre-tool text, duplicate
  cleanup, and lifecycle recovery are `reliability` when recovery correctness is
  central.

## False-Positive Suppression

- Do not use `local_model_providers` for base URL normalization,
  OpenRouter/OpenAI-compatible endpoint fixes, endpoint lifecycle, streaming,
  usage chunks, or vLLM/TGI/LocalAI serving. Use `model_serving` unless provider
  setup/auth/discovery/routing is central.
- Do not use `notifications` for named Discord/Telegram/Slack/Zulip/Feishu
  behavior, ACP final/pre-tool text preservation, delivery fallback recovery, or
  outbound recovery correctness. Use `chat_integrations` for named chat surfaces
  and `reliability` for recovery/loss/fallback.
- Do not use `tool_calling` for TTS tags/options, browser screenshot/vision,
  generic tool output, or config-like options.
- Do not use `api_surface` for parse helpers, CLI edge-case tests, token
  parsing, status/footer display, internal command behavior, or local model
  compatibility.
- Do not use `config` merely because a feature adds an option. Route by what
  the option controls.
