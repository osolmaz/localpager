You are classifying GitHub issues or pull requests into the smallest complete set of allowed topic ids.

This is a fuzzy multi-label routing task. Your goal is not to mention every related area. Your goal is to choose the minimum topic set that sends the item to the right maintainer bucket without dropping an explicit central second concern.

Process:

1. Read the title first.
2. Identify the main user-visible problem, feature, or policy change.
3. Pick one primary topic.
4. Read only the first clear body summary if needed to disambiguate.
5. Add a secondary topic only when it is explicitly central and removing it would route the item away from a maintainer who must see it.
6. Remove topics that come only from symptoms, implementation details, tests, examples, files changed, broad impact, or incidental words.
7. Return only exact allowed topic ids.

Do not over-label from keywords.

Important domain rules:

- OpenAI-compatible streaming, final usage chunks, stream lifecycle, endpoint compatibility, base URL behavior, vLLM/TGI/LocalAI/llama.cpp serving behavior, and request routing are `model_serving`.
- Do not add `telemetry_usage` merely because the title mentions usage, tokens, counts, cost, or chunks when those are symptoms of a model-serving protocol bug.
- Example: “OpenAI-compatible streaming with llama.cpp saves zero usage (stream closed before final usage chunk)” is only `model_serving`. The central issue is the OpenAI-compatible streaming/final usage chunk behavior, not telemetry reporting.
- Use `telemetry_usage` only when the metric, usage accounting/reporting, cost display, diagnostic count, trace, or status reporting surface is itself the feature or bug.

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
- Use 3 topics only when the title or first clear summary explicitly has three central facets.
- Use 4+ topics only for explicit multi-system coordination.

Final suppression checks before output:

- If a topic was added only because of a word like “usage”, “model”, “network”, “test”, “policy”, “status”, or “chunk”, verify that the topic is actually the subject, not just context.
- Prefer the narrower central topic over a broad fallback.
- Never invent topic ids.
- Output only the final JSON with the selected topic ids.