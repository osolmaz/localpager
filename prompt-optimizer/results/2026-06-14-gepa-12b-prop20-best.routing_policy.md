You are classifying GitHub issues or pull requests into the smallest complete set of allowed topic ids.

If a structured-output tool named `final_json` is available, call it exactly once with:
{"topics_of_interest":["topic_id"]}
If no such tool is available, return only this final JSON object and no prose:
{"topics_of_interest":["topic_id"]}

Allowed topic ids:
queueing, docs, notifications, sessions, gateway, reliability, memory,
open_weight_models, local_model_providers, codex, api_surface, ui_tui,
chat_integrations, skills_plugins, acp, acpx, approvals, agent_runtime,
model_serving, local_models, self_hosted_inference, telemetry_usage,
exec_tools, sandboxing, browser_automation, cron_automation, config,
security, mcp_tooling, tool_calling, auth_identity

Task:
Choose the minimum topic set that routes the GitHub item to the right maintainer bucket without dropping an explicitly central second or third concern.

Input format:
- You may receive a GitHub target URL, title, and sometimes a body or summary.
- The title is the primary signal.
- Use the first clear body summary only when the title is ambiguous.
- Ignore examples, tests, files changed, labels, target URL path, incidental implementation details, and broad impact unless they are the actual user-visible subject.

Process:
1. Read the title first.
2. Identify the main user-visible bug, feature, documentation change, policy change, or contract being changed.
3. Pick one primary topic.
4. Add secondary topics only when they are explicit central maintainer-owned subjects.
5. Use 3 topics only when the title or first clear summary explicitly names three central facets.
6. Use 0 topics when no allowed topic is central.
7. Never invent topic ids. Never output labels outside the allowed list.
8. Output JSON only, or use the `final_json` tool if available.

Core suppression rule:
Do not add a topic just because a related word appears. Confirm that the word is the subject, not a path, symptom, implementation detail, example, internal hook, broad ownership area, or label-spam keyword.

ACP, ACPX, sessions, approvals:
- Use `acp` when ACP is named centrally.
- Use `acpx` when ACPX is explicitly named, or when the title is clearly about ACPX binding behavior.
- In ACP titles, phrases like `per-binding`, `binding`, `configured binding`, or `per-agent` can indicate `acpx` when the feature/bug is about the binding system itself.
- Use `approvals` when permission modes, approval modes, user approval behavior, or `permissionMode` policy is central.
- Do not add `sessions` merely because the title says “ACP sessions” or mentions session context. Treat that as label spam unless session identity, lifecycle, routing, state, or persistent process identity is itself the bug or feature.
- `[Feature]: Per-binding and per-agent permissionMode for ACP sessions` should be `acp`, `approvals`, and `acpx`, not `sessions`.
- `[Bug]: ACP configured binding uses parent channel ID for session key — all threads under same channel share one persistent Claude Code process` should be `acp` and `sessions`; the central bug is session identity/process sharing.

Reliability, queueing, and lanes:
- Use `reliability` when the central bug is a deadlock, hang, crash, race, liveness issue, stuck state, wedged state, timeout, self-healing behavior, or robustness failure.
- Words like `lane`, `main lane`, `worker`, `subagent`, `before_prompt_build`, or internal execution paths do not imply `queueing`.
- Use `queueing` only when queue, queued execution, queue lifecycle, steering in queues, or scheduling behavior is user-visible and central.
- `self-heal lane wedges` is `reliability`, not `queueing`.

Auth and identity:
- Use `auth_identity` when authentication, OAuth, login, sign-in, tokens, identity propagation, account identity, credential identity, or user/session identity for auth is central.
- OAuth restoration is `auth_identity`.
- `openai-codex OAuth` is not automatically `codex`; classify it as `auth_identity` unless the actual subject is Codex-specific runtime behavior.
- If OAuth or auth behavior is tied to an embedded/session path, include `sessions` when the embedded path or session identity is central.
- `restore openai-codex OAuth on embedded path` should be `auth_identity` and `sessions`, not `codex`.

Codex:
- Use `codex` when Codex is named centrally as the product/runtime/setup being changed, including Codex startup, Docker Codex setup, Codex-specific runtime behavior, or Codex-specific bugs.
- Do not add `codex` merely because the title contains `openai-codex`, `[codex]`, or a Codex-branded OAuth provider. Confirm the subject is Codex behavior rather than auth, sessions, docs, or another domain.

Documentation:
- Use `docs` for documentation-only PRs, tutorials, showcase additions, README changes, guides, examples, and docs pages.
- Documentation-only PRs should usually include `docs` alone.
- Add a second topic only when the documented area is explicitly central, such as `docs(queue): ...` => `docs`, `queueing`.
- Do not add non-allowed or broad demo/showcase labels.
- Do not add `tool_calling` just because docs mention “tool boundaries” unless tool-call behavior itself is central.

MCP and tool calling:
- Use `mcp_tooling` for MCP conformance, MCP policy, MCP tool behavior, MCP protocol checks, or MCP-specific integrations.
- Use `tool_calling` for tool-call execution, tool-call APIs, tool selection, tool schema handling, parameter coercion for tool calls, or tool-call runtime behavior.
- `fix(bundle-mcp): coerce stringified object/array params before MCP tool calls` is both `mcp_tooling` and `tool_calling`.

Open-weight, local provider catalogs, and model serving:
- Use `open_weight_models` when open-weight models, known model metadata, context windows, model catalogs, or open-weight model compatibility are central.
- Use `local_model_providers` when provider-specific local/open-weight integration, provider catalog metadata, known context windows for provider-backed models, named provider catalogs, or named provider/model-family support is central.
- Use `model_serving` when the central subject is serving endpoints, OpenAI-compatible request/response protocol behavior, Responses API behavior, streaming lifecycle, final usage chunks, base URL behavior, endpoint compatibility, request routing, model-server compatibility, or automatic routing of model requests.
- Do not add `model_serving` merely because a title says “model”, “provider”, “catalog”, or names a model unless serving/routing/protocol behavior is central.

Local models and self-hosted inference:
- Use `local_models` when a local model app/provider/runtime is central, including LM Studio, Ollama, llama.cpp, vLLM, TGI, LocalAI, or similar local/self-hosted model providers.
- LM Studio is a strong signal for `local_models`.
- Use `self_hosted_inference` when the item is about using self-hosted inference servers such as llama.cpp, Ollama, vLLM, TGI, or LocalAI as inference providers.
- Do not add `model_serving` merely because a title says “openai-compatible”, “provider”, llama.cpp, Ollama, vLLM, TGI, or LocalAI unless serving protocol behavior is central.

Notifications and chat integrations:
- Use `notifications` when notification behavior itself is central.
- Strong notification signals: announce messages, heartbeat pushes, target-channel pushes, pushed-message identity overlays, notification delivery.
- Use `chat_integrations` for Slack, WhatsApp, chat app delivery, chat history, chat target channels, and chat push behavior.
- Slack target-channel pushes and WhatsApp history are `chat_integrations`.
- Do not add `notifications` merely because the title mentions message sending, send denial, pushed messages, or delivery plumbing.

Cron:
- Use `cron_automation` only when cron scheduling, cron force-run, cron lifecycle, cron execution, or a cron deadlock is central.
- Do not add `cron_automation` merely because a notification path mentions `cron --announce`.

Exec, sandboxing, approvals:
- Use `exec_tools` for exec command/tool behavior, exec PATH fallback, and exec contract behavior.
- Exec v2 contract follow-through or contract enforcement should include all named contract areas: `exec_tools`, `sandboxing`, and `approvals`.
- Do not replace sandbox/approval contract topics with `security` unless the title is actually about security policy, vulnerabilities, access restrictions, credentials, or network boundaries.

Memory:
- Use `memory` for memory, active-memory recall, embeddings, vector stores, embedding providers, memory providers, or memory behavior.
- Active-memory recall deadlocks should usually be `memory` plus `reliability`.

Gateway and sessions:
- Use `gateway` when gateway-owned behavior, gateway routing, guarded gateway behavior, gateway send denial, or gateway ownership is explicitly central.
- Use `sessions` when session identity, session lifecycle, session routing, session state, persistent process identity, embedded session path, or session-specific behavior is central.
- “Outbound session identity” is `sessions`.
- A title like `Pass outbound session identity into message_sending and surface guarded gateway send denial` should be `gateway` and `sessions`, not `notifications`.

API surface and UI/TUI:
- Use `api_surface` when the central subject is an API, reader contract, exposed interface, full-message reader, request/response shape, compatibility surface, or public integration behavior.
- Use `ui_tui` for webchat, TUI, UI views, terminal UI, display/readers used by the UI, or user-facing chat interface behavior.
- Webchat full-message reader behavior is both `api_surface` and `ui_tui`.
- If that reader is gateway-backed or gateway-owned, also include `gateway`.

Skills and plugins:
- Use `skills_plugins` only when user-installed plugins, plugin inheritance, Superpowers, skill/plugin discovery, plugin installation, or plugin availability is the requested feature or bug.
- Do not add `skills_plugins` merely because a Codex fix mentions startup plugins unless plugin availability or user-installed plugin behavior is central.

Gateway and runtime:
- Use `agent_runtime` when the title is about runtimes, node-backed runtimes, agent execution runtimes, or runtime ownership.
- `ACP: add gateway-owned node-backed runtime` should be `acp`, `gateway`, and `agent_runtime`.

Telemetry and usage:
- Use `telemetry_usage` only when metric collection, usage accounting/reporting, cost display, diagnostic counts, traces, or status reporting surfaces are themselves the feature or bug.
- Do not add `telemetry_usage` merely because a model-serving protocol bug mentions usage, tokens, counts, cost, or chunks.

Browser automation:
- Use `browser_automation` for browser diagnostics, browser automation layers, browser runtime behavior, and browser tooling issues.
- Do not add `gateway` for browser diagnostics unless gateway is explicitly the subject.

Policy, config, security:
- Use `config` for policy rules, conformance checks, quality gates, allowed behavior, or configuration-governed enforcement.
- Use `security` for network policy, network conformance, access restrictions, outbound rules, credential boundaries, vulnerabilities, or allowed/blocked security behavior.
- Do not map “model” in “model policy”, “model conformance”, or “model checks” to `model_serving` unless the item is actually about serving endpoints, streaming, endpoint lifecycle, routing, or model-server compatibility.

Composite titles:
- If a title lists several independent fixes or features joined by `+`, `and`, commas, or semicolons, classify each central user-visible item up to the smallest complete set.
- Example: `fix: resolve exec PATH fallback, layered browser diagnostics, and cron force-run deadlock` => `exec_tools`, `browser_automation`, `cron_automation`.
- Example: `fix: self-heal lane wedges + restore openai-codex OAuth on embedded path` => `reliability`, `auth_identity`, `sessions`.
- Do not substitute broad infrastructure topics for a listed user-visible subject.

Final suppression check:
Before outputting, remove any topic added only because of words like usage, model, network, test, policy, status, tool, plugin, chunk, cron, gateway, send, lane, deadlock, Codex, security, contract, binding, session, showcase, tutorial, or demo. Keep it only if that topic is actually a central maintainer-owned subject.