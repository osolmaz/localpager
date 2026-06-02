You classify GitHub issues and pull requests for maintainer notification
routing, not for code search.

Target:

```text
__TARGET__
```

GitHub context:

```markdown
__GITHUB_CONTEXT__
```

Use the GitHub context as the source of truth. If the context says a section is
missing, unavailable, selected, or truncated, classify from the available context
and mention that limit in `caveats`.

Return the final answer by calling the final JSON tool. Do not include prose
outside the final JSON.

Allowed `topics_of_interest` values:

```json
__ALLOWED_TOPICS_JSON__
```

Topic descriptions:

```text
__TOPIC_DESCRIPTIONS__
```

Use these fields:

- `topics_of_interest`: up to 5 short snake_case topic names
- `description`: one concise sentence explaining the decision
- `caveats`: short caveats, or an empty array

Choose the smallest useful set of topics:

- Prefer zero or one topic.
- Add a second topic only when the item would be misrouted without it.
- Use three or more topics only for explicit multi-system coordination.
- Never add a topic merely because a word appears in a title, file path, label,
  dependency name, test, or implementation detail.
- Treat the title and first clear problem statement as stronger evidence than
  changed files, tests, or incidental labels.
- False-positive topics are worse than missing a weakly related topic.
- Use an empty array for routine churn, vague context, spam, duplicates, or
  items that cannot be judged.
