---
title: Classifier Profiles
date: 2026-06-01
---

# Classifier Profiles

Localpager should keep its queue, source, worker, and delivery logic generic.
Classifier behavior should be configured per deployment.

The immediate problem this solves: a deployment may have a curated dataset topic
taxonomy, but the generic Localpager schema can drift into accepting arbitrary
model-generated labels. The classifier contract needs to be explicit and
configurable.

## Goals

- Let a deployment provide the exact prompt, topic list, and output schema.
- Keep notification policy separate from classification taxonomy.
- Reject topics that are not in the configured taxonomy.
- Avoid hardcoding deployment-specific labels in Localpager core.
- Preserve a small built-in default profile for simple installs.

## Non-Goals

- Do not add post-processing hacks that rewrite model topics.
- Do not make notification topics the full classifier taxonomy.
- Do not require every Localpager user to use the OpenClaw topic set.

## Implemented Config

```json
{
  "classifier": {
    "schema": "~/.config/localpager/project.schema.json",
    "prompt_template": "~/.config/localpager/project.prompt.md",
    "topic_taxonomy": "~/.config/localpager/project-topics.json"
  },
  "worker": {
    "classifier_command": "localpager-classifier",
    "notify_topics_any": [
      "local_models",
      "self_hosted_inference",
      "open_weight_models",
      "acpx"
    ]
  }
}
```

`classifier.topic_taxonomy` is the full set of topics the classifier may output.
`worker.notify_topics_any` is only the smaller paging filter.

Worker model runtime settings can also be pinned in config so service behavior
does not depend on manually loaded local-model defaults:

```json
{
  "worker": {
    "agent_base_url": "http://127.0.0.1:1234/v1",
    "agent_context_window": 8192,
    "agent_max_tokens": 768,
    "agent_timeout_ms": 5000,
    "model_unavailable_retry_delay": "5m"
  }
}
```

Treat those numbers as deployment-specific. The configured
`agent_context_window` should match the context length of the model currently
loaded at `agent_base_url`, and `worker.max_concurrency` should be compatible
with the model server's parallelism. For a concrete OpenClaw/Gemma deployment
example, see
[Onur's Isengard Setup](2026-06-02-onur-isengard-localpager-setup.md).

## Topic Taxonomy

The taxonomy should be data, not prose embedded in code:

```json
{
  "topics": [
    {
      "id": "local_models",
      "description": "Local, on-device, or open-weight model execution."
    },
    {
      "id": "acpx",
      "description": "Explicit ACPX runtime, state, harness, proxy, or CLI evidence."
    }
  ]
}
```

Localpager also accepts the dataset topic-keyword map shape:

```json
{
  "topics": {
    "local_models": {
      "description": "Local model execution and local inference behavior.",
      "keywords": ["lm studio", "ollama", "llama.cpp"]
    }
  }
}
```

The classifier wrapper should load this file and generate or validate the schema
so `topics_of_interest.items.enum` is exactly the taxonomy topic IDs.

A generic starter taxonomy lives at
`examples/profiles/repo-routing-topics.json`. Its matching prompt template is
`examples/profiles/repo-routing.prompt.md`, and its fully expanded example
schema is `examples/profiles/repo-routing.schema.json`.

The OpenClaw routing prompt lives at
`examples/profiles/openclaw-routing-v8.prompt.md`. It should be paired with the
OpenClaw topic keyword taxonomy in
`examples/profiles/openclaw-routing-topics.json`. That checked-in taxonomy was
copied from the Hugging Face dataset
`osolmaz/openclaw-classification-dataset`:
<https://huggingface.co/datasets/osolmaz/openclaw-classification-dataset>.
The Localpager profile and dataset labels use the same allowed topic IDs and
keyword hints.
That prompt is the production translation of the DS4/Gemma v8 routing policy:
title-first centrality, one-topic default, second-topic gate, and explicit
suppression of known Gemma false positives.

## Schema Contract

For the current no-interest-score contract, the deployment schema should require:

```json
{
  "topics_of_interest": ["local_models"],
  "description": "One concise grounded reason.",
  "caveats": []
}
```

The schema must reject unknown topics. For a taxonomy with `local_models` and
`acpx`, this must fail:

```json
{
  "topics_of_interest": ["random_runtime_label"],
  "description": "Reason.",
  "caveats": []
}
```

## Prompt Template

The template should be deployment-owned. Localpager should inject:

- target item URL or ref
- fetched GitHub title, body, labels, comments, changed files, and diff when enabled
- allowed topics JSON
- topic descriptions
- any deployment-specific classification policy

The prompt should tell the model to choose only from the allowed topics, use an
empty array when no listed topic applies, and pick the smallest routing set
rather than every technically related topic.

## Runtime Behavior

1. Worker calls the configured classifier command.
2. Classifier wrapper loads profile config.
3. Wrapper renders a runtime schema from `classifier.schema` and
   `classifier.topic_taxonomy`.
4. Wrapper renders the prompt template.
5. Wrapper runs `localpager-agent` with the rendered schema, pinned runtime
   settings, and a per-run session directory.
6. Schema validation rejects invalid topics before output reaches the worker.
7. Worker stores the raw JSON and `topics_json`.
8. Notification policy checks `notify_topics_any`.

Localpager builds the GitHub context before launching the classifier. If GitHub
comments, changed files, or diff cannot be fetched, the rendered context keeps
the failure detail, such as the GitHub status code.

Transient model endpoint failures are requeued with
`worker.model_unavailable_retry_delay` without burning classifier attempts.

## Implementation Checklist

- Add `classifier` config fields for schema, prompt template, and topic taxonomy.
- Make `localpager-classifier` accept those paths through flags or environment.
- Add a taxonomy loader and schema enum generation.
- Add a default generic profile for non-OpenClaw users.
- Add a small example taxonomy for maintainers.
- Add tests that the worker passes classifier profile arguments.
- Add tests that the worker passes model runtime arguments.
- Add a checked-in OpenClaw v8 routing prompt.
- Preserve prompt and session paths for live auditability.
- Add a requeue command for transient dead jobs.
- Update README to explain the difference between taxonomy topics and notification topics.
