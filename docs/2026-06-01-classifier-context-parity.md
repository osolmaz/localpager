---
title: Classifier Context Parity
date: 2026-06-01
---

# Classifier Context Parity

Localpager must give the classifier the issue or PR content directly. It should
not depend on the model opening a GitHub URL.

This document records the behavior from the original notifier classifier wrapper
that needs to be carried into Localpager.

## Problem

The original classifier wrapper fetched GitHub context before model execution.
It rendered a prompt that included the item title, body, labels, comments,
changed files, and selected PR diff.

Current Localpager mostly passes only the target URL plus the prompt profile and
topic taxonomy. This makes the local model say it cannot inspect the link and
return empty topics.

The topic schema fix prevents invented labels, but it does not fix missing
input context.

## Original Behavior

The original wrapper did this before calling the model:

1. Parse the target as a GitHub URL, `owner/repo#number`, or number plus repo.
2. Detect whether the target is a PR or issue when needed.
3. Fetch GitHub content with `gh`.
4. For PRs, fetch:
   - number, URL, title, state, author
   - labels
   - body
   - comments
   - changed files
   - patch diff
5. For issues, fetch:
   - number, URL, title, state, author
   - labels
   - body
   - comments
6. Truncate body, comments, changed files, and diff to configured limits.
7. Score diff chunks by relevant keywords and include selected excerpts.
8. Escape control-like tags in user content before injecting it into the prompt.
9. Load allowed topic values from the schema.
10. Optionally load topic keyword hints from the dataset support files.
11. Render a full prompt with the fetched context.
12. Run the local model with the final schema.
13. Validate final JSON before printing it to the worker.

The key design point: model browsing was optional. The prompt already contained
enough context to classify the item.

## Historical Gap

Before this migration, Localpager had only part of that behavior:

- `internal/sources/gitcrawl/gitcrawl.go` reads gitcrawl title, author, URL,
  content hash, timestamps, and state, but ignores `body`, `labels_json`, and
  `raw_json`.
- `internal/sources/github/github.go` reads GitHub body, but the stored item
  model discards it.
- `internal/localpager/models.go` has no `body` or `labels_json` columns.
- `scripts/localpager-render-profile.mjs` renders target URL/ref and taxonomy,
  but not item context.
- `scripts/localpager-classifier` calls `localpager-agent` with the rendered
  prompt, but does not enrich the prompt with GitHub context.

The result was a prompt that said "inspect this target" while only providing a
URL. Local models could not reliably do that.

## Implemented Behavior

Localpager now stores the source body and labels on `localpager_items`, renders
a GitHub context file before classification, and passes that file to
`localpager-classifier` as `--github-context-file`.

The renderer replaces `__GITHUB_CONTEXT__` in the configured prompt template.
The default prompt tells the model to classify from that injected context
instead of assuming it can browse a GitHub URL.

For GitHub items, the context builder can include:

- stored URL, title, state, author, repo, number, body, and labels
- issue and PR comments fetched from GitHub
- PR changed files fetched from GitHub
- selected PR diff excerpts fetched from GitHub
- caveats when optional fetched context is unavailable

Deployments can set these options under `classifier.context.github`.

## Target Behavior

Localpager should render a prompt section like this for GitHub items:

````text
GitHub item:
- Repository: owner/repo
- Type: pull_request
- Number: 123
- URL: https://github.com/owner/repo/pull/123
- Title: ...
- State: open
- Author: ...
- Labels: bug, local-models
- Changed file count: 4
- Changed files: path/a.ts, path/b.ts

Body:
```markdown
...
```

Comments/context:
```markdown
...
```

Diff/context:
```diff
...
```
````

The exact prompt remains deployment-configurable, but the available context
should be provided by Localpager through stable template variables.

## Data Model

Add durable fields to `localpager_items`:

- `body TEXT`
- `labels_json TEXT`

Keep heavier or frequently changing context out of the item row unless needed:

- comments can be fetched at classification time
- PR changed files can be fetched at classification time
- PR diff can be fetched at classification time

This keeps the normal enqueue path cheap while still letting the classifier get
complete context.

## Context Config

Add `classifier.context` config:

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

Defaults should be conservative and useful:

- include body and labels by default
- include comments by default
- include changed files and diff for PRs by default
- truncate all large text fields

## Template Variables

Extend prompt rendering with these variables:

- `__TARGET__`
- `__ALLOWED_TOPICS_JSON__`
- `__TOPIC_TAXONOMY_JSON__`
- `__TOPIC_DESCRIPTIONS__`
- `__GITHUB_CONTEXT__`

`__GITHUB_CONTEXT__` should contain the fully rendered, escaped, truncated
context block. This keeps deployment prompt templates simple while keeping
context assembly in code.

## Implementation Plan

1. Done: add body and labels fields to `IngestItem`.
2. Done: add `Body` and `LabelsJSON` columns to `Item`.
3. Done: update `upsertGenericItem` so fresh source updates persist body and
   labels.
4. Done: update the gitcrawl source query to read `threads.body` and
   `threads.labels_json`.
5. Done: update the GitHub API source to collect labels and persist them.
6. Done: add a GitHub context builder in Localpager that can:
   - use stored title/body/labels
   - fetch comments from GitHub when enabled
   - fetch PR changed files and diff when enabled
   - truncate large sections
   - escape control-like tags
7. Done: pass the rendered context to `localpager-classifier`.
8. Done: make `localpager-render-profile.mjs` replace `__GITHUB_CONTEXT__`.
9. Done: update default and local prompt templates to include
   `__GITHUB_CONTEXT__`.
10. Done: keep runtime schema enum generation exactly as-is.

## Tests

Add tests that fail if Localpager regresses back to URL-only prompting:

- gitcrawl source maps body and labels into `IngestItem`.
- GitHub API source maps body and labels into `IngestItem`.
- ingest persists body and labels.
- rendered classifier prompt contains title, body, labels, and URL.
- PR context includes changed files and selected diff when enabled.
- issue context includes comments when enabled.
- context truncation marks omitted content.
- control-like tags in issue text are escaped.
- invalid topics are still rejected by the rendered runtime schema.

## Rollout

1. Implement body and labels persistence first.
2. Restart the local service and verify new prompt files include body and
   labels.
3. Add comments and PR changed files.
4. Add diff selection.
5. Reprocess a small recent set and compare topic accuracy before enabling
   broad live use.

The first acceptance check is simple: a newly processed prompt file must include
the actual GitHub title, body, and labels, not just a URL.
