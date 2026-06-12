---
title: Classifier Pipeline and Profile Rendering
date: 2026-06-12
---

# Classifier Pipeline and Profile Rendering

This documents the end-to-end classification path and the profile renderer that
turns a prompt template, schema, and topic taxonomy into the exact prompt and
runtime schema handed to the model. It complements
[Classifier Profiles](2026-06-01-classifier-profiles.md), which covers the
deployment config contract, and the prompt history under
`localpager-agent/docs/prompt-history/`.

## End-to-End Chain

```text
worker (internal/localpager/worker.go)
  └─ runClassifier (internal/localpager/classifier.go)
       ├─ builds the GitHub context (internal/localpager/context.go)
       └─ execs the configured classifier_command (default: scripts/localpager-classifier)
            └─ scripts/localpager-classifier (bash wrapper)
                 ├─ localpager render-context        → GitHub context markdown
                 ├─ scripts/localpager-render-profile.mjs
                 │      → fills prompt placeholders + generates the runtime schema enum
                 └─ localpager-agent (TS CLI)
                      └─ Pi coding agent             → registers final_json → writes final-output.json
```

The Go core stays generic: it never hardcodes topics and never rewrites model
output. Everything classifier-specific is config plus files passed down the
chain.

## Stages

1. **Worker → classifier command.** `runClassifier` resolves
   `worker.classifier_command`, builds the GitHub context, and forwards the
   target plus `--schema`, `--prompt-template`, `--topic-taxonomy`, the
   `agent_*` sampling/runtime flags, and reposhell flags. See
   `classifierCommandArgs` in `internal/localpager/classifier.go`.
2. **Context build.** `context.go` renders title, state, author, labels, body,
   comments, changed files, and diff with fixed truncation budgets (body 2500,
   comments 1500, changed files 2000, diff 5000 chars). Control-like tags are
   neutralized. Fetch failures are recorded as `caveats`, not fatal errors.
3. **Profile render.** `localpager-render-profile.mjs` substitutes the prompt
   placeholders and rewrites the schema's topic enum from the taxonomy (below).
4. **Agent run.** `localpager-agent` points Pi at the local model, exposes
   `final_json` (and `bash` when a reposhell socket is set), and runs once. It
   owns missing-`final_json` recovery (below).

## Profile Renderer Reference

`scripts/localpager-render-profile.mjs` takes `--target`, `--schema`,
`--prompt-template`, `--output-schema`, `--output-prompt`, and an optional
`--topic-taxonomy` and `--github-context-file`. It writes a rendered prompt and
a rendered runtime schema.

### Prompt placeholders

The prompt template is plain text with these literal placeholders replaced:

| Placeholder | Replaced with |
| --- | --- |
| `__TARGET__` | the target ref (URL or `owner/repo#n`) |
| `__GITHUB_CONTEXT__` | the GitHub context markdown, trimmed |
| `__ALLOWED_TOPICS_JSON__` | JSON array of taxonomy topic ids, or a "no taxonomy" sentence when none is configured |
| `__TOPIC_TAXONOMY_JSON__` | `{"topics":[...]}` with the normalized taxonomy |
| `__TOPIC_DESCRIPTIONS__` | a markdown list of `` `id`: description cues: <kw> `` |

Notes:

- Topic descriptions are truncated to 80 characters in the rendered list.
- Only the **first** keyword per topic is emitted as a `cues:` hint
  (`slice(0, 1)`), even when the taxonomy lists many. The taxonomy keyword lists
  are hints for humans and for `__TOPIC_TAXONOMY_JSON__`, not a full cue dump in
  the prompt.

### Topic taxonomy shapes

`--topic-taxonomy` accepts three shapes:

- `{"topics": [{"id", "description", "keywords"}]}` (array form)
- `{"topics": {"<id>": {"description", "keywords"}}}` (dataset keyword-map form)
- a JSON schema whose `properties.topics_of_interest.items.enum` lists the ids

Topic ids must match `^[a-z][a-z0-9_]{0,63}$` and be unique, or rendering fails.

### Runtime schema enum generation

When a taxonomy is supplied, the renderer overwrites
`topics_of_interest.items` in the output schema with
`{ "type": "string", "enum": [<taxonomy ids>] }` and sets `uniqueItems: true`.

This means the **base schema's** own topic constraints are overridden whenever a
taxonomy is passed. `schemas/classification.schema.json` ships a free-form
`pattern` for topics and `examples/profiles/repo-routing.schema.json` ships a
hardcoded enum, but in both cases the taxonomy wins. With no `--topic-taxonomy`,
the base schema passes through unchanged.

## Flag-Forwarding Contract

`scripts/localpager-classifier` consumes only the flags it needs to render the
profile: `--schema`, `--prompt-template`, `--topic-taxonomy`, and
`--github-context-file`. Every other flag is forwarded verbatim to
`localpager-agent`, which owns model selection, sampling, tool configuration,
and the `LOCALPAGER_AGENT_*` / `LOCALPAGER_REPOSHELL_*` env defaults. The
wrapper does not re-parse or re-default those; that logic lives once, in the
agent.

`localpager-agent` owns Pi tool exposure. It always provides `final_json` and
adds a read-only `bash` tool only when `--reposhell-socket` is passed. It
**rejects** caller-supplied `--tools`/`--no-tools`. There is no `classifier.tools`
config field or `--classifier-tools` flag; tool exposure follows from whether
reposhell is configured.

## Missing final_json Recovery

`localpager-agent` is the single owner of "the model did not call final_json"
handling, in `src/cli/cli.ts` and `src/structured/recovery.ts`:

1. **Payload replay.** If the session transcript contains a final answer the
   model wrote as prose or a pseudo `call:final_json` block, the agent re-runs
   the same Pi session feeding that exact payload back and forcing a real
   `final_json` call.
2. **Re-prompt fallback.** If nothing parseable is found, the agent re-runs once
   with the original prompt plus a terse instruction to call `final_json` and
   write no prose.
3. If both fail, the run fails with
   `final_json was not called; no structured output was captured`.

The `localpager-classifier` wrapper therefore invokes the agent exactly once and
does no retry of its own.

## Two Templating Systems

There are two distinct templating layers; do not confuse them:

- **Profile placeholders** (`__TARGET__`, `__GITHUB_CONTEXT__`, …) are
  substituted by `localpager-render-profile.mjs` for the classifier prompt.
- **Agent prompt templates** use Handlebars-style `{{var}}` / `{{{var}}}` /
  `{{#if}}` and are rendered inside `localpager-agent`
  (`src/prompts/template.ts`) via `--prompt-template` + `--prompt-var(s)`. The
  `localpager-agent/examples/prompts/binary-classifier.hbs` example uses this
  layer.

The classifier path uses the placeholder layer; the agent's own
`--prompt-template` layer is a separate, general-purpose feature.

## Files

- `scripts/localpager-classifier` — wrapper command
- `scripts/localpager-render-profile.mjs` — placeholder + schema-enum renderer
- `internal/localpager/classifier.go` — worker → command bridge
- `internal/localpager/context.go` — GitHub context builder
- `localpager-agent/src/cli/cli.ts`, `src/structured/recovery.ts` — final_json
  recovery
- `prompts/`, `examples/profiles/` — prompt templates, schemas, taxonomies
