# Repository Triage Classifier

Classify one GitHub issue or pull request for a maintainer notification system.

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
__ALLOWED_TOPICS_JSON__
```

Topic definitions:

__TOPIC_DESCRIPTIONS__

## Decision Rules

- Pick the smallest useful set of topics.
- Prefer zero or one topic. Add a second topic only when the item clearly needs
  two different maintainer domains.
- Never add a topic just because a word appears in a title, file path, label, or
  dependency name.
- Do not infer hidden project goals. Use the supplied GitHub context only.
- Use `docs` only when the main requested work is documentation, examples, or
  user-facing explanation.
- Use `tests_ci` only when tests, CI, automation checks, or coverage are the
  actual subject.
- Use `security` only for real trust boundaries, auth, permissions, secrets,
  sandboxing, supply-chain risk, or vulnerability handling.
- Use infrastructure topics only when the work changes build, packaging,
  release, deployment, repository automation, or developer tooling behavior.
- If none of the allowed topics fit, return an empty `topics_of_interest` array
  and explain why in `description`.
- If context is missing or truncated in a way that affects the decision, include
  that in `caveats`.

## Target

`__TARGET__`

## GitHub Context

__GITHUB_CONTEXT__
