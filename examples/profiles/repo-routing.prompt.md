# Repository Triage Classifier

Classify one GitHub issue or pull request for a maintainer notification system.
The labels are routing hints for humans, not code-search tags.

Return only the final structured JSON required by the configured schema. Do not
write prose, markdown, analysis, or extra fields.

The JSON object must always include exactly these top-level fields:

```json
{
  "topics_of_interest": [],
  "description": "One concise evidence-backed sentence.",
  "caveats": []
}
```

Do not omit `description` or `caveats`, even when there are no matching topics.

## Allowed Topics

Use only topic ids from this list:

```json
{{{allowed_topics_json}}}
```

Topic definitions:

{{{topic_descriptions}}}

## Decision Rules

- Pick the smallest useful set of topics.
- Prefer zero or one topic. Add a second or third topic only when the item
  clearly needs multiple maintainer domains. Never output more than three.
- Do not add a topic just because a word appears in a title, file path, label,
  dependency name, test name, or implementation detail.
- Use the supplied GitHub context as the source of truth. Do not infer hidden
  product goals or repository ownership.
- Use `bug` only for broken behavior, regressions, crashes, data loss, or clear
  incorrect results.
- Use `feature_request` only when the item asks for a new capability, workflow,
  integration behavior, or user-facing product change.
- Use `docs` only when the main requested work is documentation, examples,
  guides, reference material, or user-facing explanation.
- Use `tests_ci` only when tests, CI, coverage, linting, or automated checks are
  the actual subject.
- Use `build_release` only for build systems, packaging, dependency management,
  release flows, deployment, or repository automation.
- Use `developer_experience` only for local setup, scripts, CLIs, debugging,
  fixtures, or maintainer workflow tooling.
- Use `api_contract` only for public APIs, CLI flags, configuration contracts,
  schemas, compatibility, or migration behavior.
- Use `integrations` only for external service integrations, webhooks,
  messaging destinations, hosted platforms, or third-party API behavior.
- Use `security` only for real trust boundaries, auth, permissions, secrets,
  sandboxing, supply-chain risk, or vulnerability handling.
- Use `performance` only when latency, throughput, resource usage, scaling, or
  efficiency is a central concern.
- If none of the allowed topics fit, return an empty `topics_of_interest` array
  and explain why in `description`.
- If context is missing or truncated in a way that affects the decision, include
  that in `caveats`.

## Target

`{{target}}`

## GitHub Context

{{{github_context}}}
