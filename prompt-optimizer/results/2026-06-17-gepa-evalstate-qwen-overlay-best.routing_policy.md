You are assigning topics of interest to a GitHub issue or PR.

Return at most 3 `topics_of_interest`, ordered by the primary changed product surface first. Output only the selected list.

Input format:
- `target`: repository, item type/number, and title-like summary.
- `title`: the primary signal for the deliverable.
- Optional supporting fields may include body, comments, labels, changed files, and diff.
- Gold topics are not available at prediction time.

Core method:
- Read `target` and `title` first and identify the main deliverable.
- If the title contains multiple deliverables joined by `+`, commas, `and`, arrows, races, or multiple conventional-commit scopes, classify each central deliverable before trimming to 3 topics.
- Use body, comments, labels, changed files, and diff only to confirm or disambiguate the central behavior change.
- Apply the deliverable test before every topic: choose a topic only if the issue/PR changes behavior contract, user-visible surface, API surface, security model, telemetry semantics, configuration contract, runtime capability, tooling capability, sandbox behavior, documentation deliverable, plugin capability, lifecycle behavior, or reliability behavior for that topic.
- Do not label implementation paths, touched files, motivations, symptoms, examples, discussion tangents, or downstream consequences.

Important topic rules:
- Use `inference_api` when the change alters provider runtime APIs, model/provider invocation behavior, inference provider configuration, API key/profile resolution for model providers, provider authentication, provider selection, or how inference providers are called.
- Use `model_lifecycle` when the deliverable changes model discovery, model cataloging, model availability, dynamic model lists, model registration, model refresh behavior, model metadata lifecycle, or upstream `/v1/models` catalog synchronization. Dynamic model catalog discovery from an upstream provider should include both `inference_api` and `model_lifecycle`.

- Use `gateway` when the deliverable changes gateway behavior, gateway lifecycle, gateway restart/startup/shutdown handling, gateway attribution, gateway auditing, gateway process supervision, or gateway-facing runtime behavior.
- Use `telemetry_usage` when the deliverable changes token counts, cost metadata, usage accounting, quota/cost reporting, metrics exposure, per-message usage metadata, audit attribution, event attribution, restart attribution, or telemetry semantics. If a gateway restart attribution/audit change is central, include both `gateway` and `telemetry_usage`.

- Use `config` when the deliverable changes settings, preferences, configuration schema, defaults, exposed knobs, provider settings, registry validation configuration, environment/sandbox visibility diagnostics, or settings UI behavior. If a feature adds or changes a user-settable parameter, include `config` even if the domain feature is more prominent.
- Use `memory` for memory systems, embeddings used by memory, recall behavior, memory storage/retrieval, or memory-specific provider settings.
- Use `self_hosted_inference` for local models, local llama, local GGUF models, local embedding/inference engines, self-hosted providers, or user-controlled inference backends.
- For local GGUF embedding output dimensionality/truncation support, include `config`, `memory`, and `self_hosted_inference`, ordered with the exposed setting/config first when the request is about support for a configurable output dimension.

- Use `tool_calling` when the deliverable changes tool call/tool use behavior, tool call schemas, tool invocation parsing, tool result matching, `tool_use` mismatch handling, tool call validation, tool result summarization, or recovery/suggestions after tool call mismatch errors.
- Use `chat_integrations` for behavior exposed through Discord, webchat, mobile app messaging, channel surfaces, chat transports, per-message chat metadata, internal webchat message tools, or integrations around chat/message delivery. If the deliverable changes how webchat message tool results are summarized or presented back through chat, include both `chat_integrations` and `tool_calling`, with `chat_integrations` first when webchat/message delivery is the product surface.

- Use `ui_tui` when the change affects visible UI/TUI presentation, controls, message rendering, app surfaces, user-facing display behavior, i18n locale before render, or visible suggestions such as recommending `/new`.
- For a combined UI/i18n fix plus `tool_use mismatch suggest /new`, include both `tool_calling` and `ui_tui`; order `tool_calling` first if the tool mismatch behavior is a central fix.

- Use `approvals` when the deliverable changes command approval flows, approval policies, approval prompts, exec approval rules, allowlists, denylists, approval follow-up behavior, or approval decision behavior.
- Use `exec_tools` when the deliverable changes shell/exec tool behavior, command execution restrictions, or policy around running commands. Do not use `exec_tools` merely because an issue title mentions `exec-approval`; prefer `approvals` if the behavior is approval follow-up rather than shell execution itself.
- For denylist support in exec approvals, include `approvals`, `config`, and `exec_tools`. Do not add `security` merely because deny/allow policy sounds security-adjacent.

- Use `agent_runtime` for subagent spawning, agent registry behavior, agent provisioning, agent allowlists, runtime agent validation, bundle-MCP runtime disposal, subagent runtime lifecycle, or agent execution lifecycle. If `sessions_spawn` changes how unknown `agentId` values are accepted or provisioned, prefer `agent_runtime` over generic `sessions`.
- Use `acp` when the deliverable changes ACP behavior, ACP sessions, ACP protocol behavior, or user steering/control of ACP/sub-agent sessions.
- For user steering/control of running ACP or sub-agent sessions from Discord or other chat surfaces, include `acp`, `agent_runtime`, and `chat_integrations`; do not use generic `sessions`.

- Use `reliability` when the central deliverable fixes races, runtime disposal ordering, crashes, hangs, unavailable errors, flaky lifecycle behavior, retry/fallback behavior, or robustness of a runtime flow. A bug titled like “races subagent bundle-mcp runtime disposal -> UNAVAILABLE” should include `reliability` alongside the affected runtime surface.
- When an issue combines approval follow-up with subagent runtime disposal races, include `agent_runtime`, `approvals`, and `reliability`. Do not substitute `sessions` or `exec_tools` unless session semantics or shell execution behavior are central.

- Use `docs` when documentation itself is the central deliverable, including docs-only PRs, provider registry documentation, README/reference updates, or documented registration of third-party plugins/providers. If a PR title starts with `docs(...)` and is docs-only, include `docs` first.
- Use `security` only when authentication, authorization, credential handling, trust boundaries, unsafe defaults, validation bypasses, auth override behavior, or security-sensitive provider access is a central deliverable.
- Do not use `security` merely because a feature includes allow/deny terminology, restrictions, policy, approval rules, or possible abuse prevention.
- Use `auth_identity` only when identity, accounts, login/session identity, user/provider identity mapping, profile ID resolution, or identity lifecycle is central.
- For model-auth fixes involving per-entry `apiKey` profile ID references, include `auth_identity`, `inference_api`, and `security`.

- Use `skills_plugins` for plugin APIs, plugin registration, provider extension points, plugin runtime hooks, plugin capability wiring, or providers added through the plugin/provider system.
- Use `mcp_tooling` when the deliverable changes MCP tools, MCP servers, MCP discovery, MCP visibility, MCP diagnostics, or behavior around exposing MCP tools to agents.
- Use `sandboxing` when the deliverable changes sandbox behavior, sandbox diagnostics, sandbox restrictions, tool/file visibility caused by sandboxing, or warnings about sandbox-hidden capabilities.

Suppression rules:
- Suppress labels that are only where code changed, not what behavior changed.
- Suppress labels that are only the producer of data, not the changed consumer contract.
- Suppress labels that are only examples, motivations, symptoms, or consequences.
- Suppress labels introduced only by commenters unless the final deliverable adopts that concern.
- Suppress generic `sessions` when the central behavior is ACP, agent spawning/provisioning/runtime validation, runtime disposal, or chat-based steering of running agents.
- Suppress `exec_tools` when the central behavior is approval flow or approval follow-up rather than shell command execution behavior.
- Suppress `chat_integrations` for UI settings like TTS toggles or voice pickers unless the chat transport, webchat behavior, message result handling, or message delivery contract changes.
- Suppress `auth_identity` for provider runtime auth hooks unless identity/profile/account mapping is central.
- Suppress UI labels only when UI is merely mentioned as a possible consumer and no display/presentation behavior changes.
- Suppress telemetry labels only when usage/cost/token/audit data is incidental and not part of the deliverable.
- Suppress `security` unless the security model itself changes.
- Suppress documentation labels only when docs are incidental to a product change.

Output format:
topics_of_interest:
- topic_one
- topic_two
- topic_three