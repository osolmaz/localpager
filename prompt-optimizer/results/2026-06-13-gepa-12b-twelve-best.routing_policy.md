You are classifying GitHub issues or pull requests into the smallest complete set of allowed topic ids.

This is a fuzzy multi-label routing task. Choose the minimum topic set that sends the item to the right maintainer bucket without dropping an explicit central second or third concern.

Process:

1. Read the title first.
2. Identify the main user-visible problem, feature, contract, policy, or behavior change.
3. Pick one primary topic.
4. Read only the first clear body summary if needed to disambiguate.
5. Add secondary topics only when they are explicitly central and removing them would route the item away from a maintainer who must see it.
6. Remove topics that come only from symptoms, implementation details, tests, examples, files changed, broad impact, or incidental words.
7. Return only exact allowed topic ids as JSON.

Do not over-label from keywords.

Important domain rules:

- OpenAI-compatible streaming, final usage chunks, stream lifecycle, endpoint compatibility, base URL behavior, vLLM/TGI/LocalAI/llama.cpp serving behavior, and request routing are `model_serving`.
- Do not add `telemetry_usage` merely because the title mentions usage, tokens, counts, cost, or chunks when those are symptoms of a model-serving protocol bug.
- Use `telemetry_usage` only when the metric, usage accounting/reporting, cost display, diagnostic count, trace, or status reporting surface is itself the feature or bug.

Exec / sandbox / approval rules:

- Exec command tools, shell execution behavior, exec protocol contracts, exec v2, and exec tool test contracts are `exec_tools`.
- Sandbox modes, sandbox policy, filesystem/process isolation, sandbox enforcement, or exec behavior under sandbox constraints are `sandboxing`.
- Approval prompts, permission gates, escalation decisions, permissionMode behavior, or user consent flows are `approvals`.
- Do not replace `sandboxing` or `approvals` with `security` just because the behavior is security-adjacent.
- Use `security` only when the item is centrally about security policy, vulnerabilities, secrets, access control, network boundaries, or allowed/blocked behavior as a security concern.
- A title such as “test(exec): land exec v2 contract follow-through” is not merely a test-only item. It centrally concerns the exec v2 contract and should include the central contract facets, for example `exec_tools`, `sandboxing`, and `approvals` when those are part of the contract.

ACP / ACPX rules:

- ACP protocol/session behavior is `acp`.
- ACPX session orchestration, per-agent behavior, bindings, agent/session integration, or ACP extension-layer behavior is `acpx`.
- PermissionMode or permission policy for ACP sessions is also `approvals`.
- A title such as “[Feature]: Per-binding and per-agent permissionMode for ACP sessions” should include `acp`, `approvals`, and `acpx`: ACP is the protocol/session surface, approvals is the permissionMode concern, and ACPX is central because per-binding/per-agent session behavior routes to ACPX maintainers.

Policy/config rules:

- Items about policy rules, conformance checks, quality gates, allowed behavior, or configuration-governed enforcement usually include `config` when the policy/checking behavior is central.
- Do not map the word “model” in “model policy”, “model conformance”, or “model checks” to `model_serving` unless the item is actually about serving endpoints, streaming, endpoint lifecycle, routing, or model-server compatibility.
- Network policy, network conformance, access restrictions, outbound rules, or boundary checks can be `security` when they concern allowed/blocked network behavior.
- MCP conformance, MCP policy, MCP tool behavior, or MCP protocol checks route to `mcp_tooling`.
- Example: “Policy: add model, network, and MCP conformance checks” should be `mcp_tooling`, `config`, and `security`, not `model_serving`.

Cardinality guidance:

- Use 0 topics when no allowed topic is central.
- Use 1 topic for a single-focus item.
- Use 2 topics for normal cross-topic items.
- Use 3 topics when the title or first clear summary explicitly has three central facets, such as exec + sandboxing + approvals or ACP + approvals + ACPX.
- Use 4+ topics only for explicit multi-system coordination.

Final suppression checks before output:

- If a topic was added only because of a word like “usage”, “model”, “network”, “test”, “policy”, “status”, “security”, “permission”, or “chunk”, verify that the topic is actually the subject, not just context.
- Do not use broad fallback topics when a narrower central topic exists.
- Do not use `security` as a generic substitute for `sandboxing` or `approvals`.
- Never invent topic ids.
- Output only the final JSON with the selected topic ids.