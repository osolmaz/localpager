# Structured Output

Localpager Agent supports structured final answers for workflows that need machine-readable output from a local model.

Example: inspect a document, issue, or pull request with Pi tools, then classify whether it matches a criterion.

## How It Works

Use `--final-schema <path>` or the shorter `--schema <path>` in Pi print mode (`-p` or `--print`).

Localpager Agent does not modify Pi and does not proxy the model API. Instead, it creates a temporary Pi extension for the run:

1. Localpager Agent reads the JSON Schema file.
2. It generates a Pi extension that registers a `final_json` tool with that schema as its parameters.
3. It starts Pi with that extension plus an extra instruction telling the model to call `final_json` when the work is done.
4. If reposhell is configured, Pi can use that read-only `bash` tool during the investigation.
5. When the model calls `final_json`, Pi validates the tool arguments against
   the schema, the extension writes the JSON to disk, and the run terminates.
6. Localpager Agent prints only the captured JSON.

This keeps the agent loop freeform while making the final answer structured.

## CLI

```bash
localpager-agent --final-schema ./examples/schemas/binary-classifier.schema.json -p "classify whether this issue is release-blocking: <text>"
```

Alias:

```bash
localpager-agent --schema ./examples/schemas/binary-classifier.schema.json -p "classify whether this issue is release-blocking: <text>"
```

For maintained prompts, use a prompt template instead of inline `-p` text:

```bash
localpager-agent \
  --final-schema ./examples/schemas/binary-classifier.schema.json \
  --prompt-template ./examples/prompts/binary-classifier.hbs \
  --prompt-vars-file ./examples/prompts/binary-classifier.vars.json
```

You can also set a default schema with:

```bash
export LOCALPAGER_AGENT_FINAL_SCHEMA=./examples/schemas/binary-classifier.schema.json
```

## Prompt Control

By default, schema runs replace Pi's coding-agent system prompt with a short
structured-output prompt and append a final-schema instruction. A caller can
pass Pi `--system-prompt` to replace the base prompt while keeping the appended
`final_json` instruction.

For complete control over the system prompt text, pass both a custom
`--system-prompt` and `--no-final-schema-instruction`:

```bash
localpager-agent \
  --final-schema ./examples/schemas/binary-classifier.schema.json \
  --no-final-schema-instruction \
  --system-prompt "Your full system prompt here." \
  -p "classify whether this issue is release-blocking: <text>"
```

The `final_json` tool is still registered and schema-validated. This option only
removes the generated appended prompt text, so the caller's custom system prompt
must still tell the model when and how to call `final_json`.

## Tool Configuration

Localpager Agent owns Pi tool configuration. Caller-supplied Pi tool flags such
as `--tools`, `-t`, `--no-tools`, and `-nt` are rejected. Schema runs expose
`final_json`; reposhell runs expose read-only `bash`; other Pi tools are not
passed through.

`--final-schema` also requires Pi print mode (`-p`, `--print`, or
`--prompt-template`). Localpager Agent suppresses Pi's normal stdout during
schema runs so it can print only the captured JSON, which is not compatible
with Pi's interactive terminal UI. Schema runs close Pi stdin instead of
inheriting the caller's stdin; this keeps batch runs from accidentally feeding
terminal input to Pi while stdout is reserved for the captured JSON.

## Example Schema

```json
{
  "type": "object",
  "additionalProperties": false,
  "required": ["is_match", "label", "confidence", "summary", "reasons", "caveats"],
  "properties": {
    "is_match": { "type": "boolean" },
    "label": {
      "type": "string",
      "enum": ["match", "partial_match", "no_match", "unclear"]
    },
    "confidence": { "type": "number", "minimum": 0, "maximum": 1 },
    "summary": { "type": "string" },
    "reasons": { "type": "array", "items": { "type": "string" } },
    "caveats": { "type": "array", "items": { "type": "string" } }
  }
}
```

## Example Output

```json
{
  "is_match": true,
  "label": "match",
  "confidence": 0.86,
  "summary": "The issue describes a startup crash that blocks the release candidate path.",
  "reasons": [
    "The issue affects the release branch.",
    "The failure happens before the main UI can load."
  ],
  "caveats": ["The impact on patch releases was not checked."]
}
```

## Reliability Notes

The schema must be a JSON object schema with root `type: "object"`.

If the model never calls `final_json`, localpager-agent exits with a clear error
instead of printing unstructured text. For long prompts on thinking models,
choose a large enough `--max-tokens` budget for the model to finish reasoning
and still call `final_json`.

This design works even when the local OpenAI-compatible backend does not support OpenAI `response_format: { type: "json_schema" }`, because Pi validates the final tool arguments before executing the tool.
