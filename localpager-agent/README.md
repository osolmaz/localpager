# localpager-agent

Localpager Agent is Pi with model wiring prefilled for either an
OpenAI-compatible endpoint or a Pi built-in provider.

It does not know about any project-specific workflow. For OpenAI-compatible
endpoints, it discovers the model, writes a temporary Pi config under local
state, and forwards the rest of the command line to Pi. For Pi built-in
providers, it leaves Pi's model registry and auth storage in control.

## Install

```bash
npm install
npm run build
```

During development:

```bash
npm run localpager-agent -- --status
```

After build:

```bash
node dist/src/cli/main.js --status
```

## Local Model

Localpager Agent defaults to LM Studio's OpenAI-compatible server:

```text
http://127.0.0.1:1234/v1
```

Load Gemma in LM Studio:

```bash
~/.lmstudio/bin/lms server start
~/.lmstudio/bin/lms load gemma-4-e4b-it -y
```

Check what localpager-agent will use:

```bash
localpager-agent --status
```

## Usage

Run Pi interactively on the local model:

```bash
localpager-agent
```

Run a non-interactive Pi prompt:

```bash
localpager-agent -p "summarize this repo"
```

Pin a specific local model id:

```bash
localpager-agent --model gemma-4-e4b-it -p "write a detailed implementation plan"
```

Point at a different OpenAI-compatible local server:

```bash
localpager-agent --base-url http://127.0.0.1:8000/v1 -p "review the src directory"
```

Use a Pi built-in provider, such as the ChatGPT/Codex subscription-backed
`openai-codex` provider:

```bash
localpager-agent \
  --backend pi-builtin \
  --model openai-codex/gpt-5.3-codex-spark \
  --thinking minimal \
  -p "classify this item"
```

Equivalent explicit provider form:

```bash
localpager-agent \
  --backend pi-builtin \
  --provider-id openai-codex \
  --model gpt-5.3-codex-spark \
  --thinking minimal \
  -p "classify this item"
```

Pi must already have credentials for that provider in the selected
`PI_CODING_AGENT_DIR`; use Pi's `/login` flow once if the provider is not
configured.

Pin OpenAI-compatible sampling request fields when a workflow needs repeatable
local model runs:

```bash
localpager-agent \
  --temperature 0 \
  --top-p 1 \
  --seed 1234 \
  --presence-penalty 0 \
  --frequency-penalty 0 \
  -p "classify this item"
```

Render a maintained prompt template instead of passing the prompt inline:

```bash
localpager-agent \
  --prompt-template ./examples/prompts/binary-classifier.hbs \
  --prompt-vars-file ./examples/prompts/binary-classifier.vars.json
```

Pass a Pi flag that localpager-agent also owns after `--`:

```bash
localpager-agent --model gemma-4-e4b-it -- --model some-pi-level-value
```

## Prompt Templates

Use `--prompt-template <path>` when a workflow needs a maintained prompt file
instead of ad hoc inline text. Localpager Agent renders the template, then passes
the rendered prompt to Pi as `-p <rendered prompt>`.

Template variables come from JSON object files and optional inline overrides:

```bash
localpager-agent \
  --prompt-template ./examples/prompts/binary-classifier.hbs \
  --prompt-vars-file ./examples/prompts/binary-classifier.vars.json \
  --prompt-var criterion="The item is a release blocker" \
  --write-rendered-prompt /tmp/localpager-rendered.prompt.txt
```

Supported template syntax is intentionally small:

- `{{name}}` and `{{{name}}}` insert a variable value.
- Non-string JSON values are rendered with `JSON.stringify(value, null, 2)`.
- `{{#if name}}...{{/if}}` includes a block when the variable is truthy.
- Missing required variables fail the run.

`--prompt-vars-file` can be repeated. Later files override earlier files, and
`--prompt-var key=value` overrides file values. `--prompt-template` cannot be
combined with Pi `-p` or `--print`, because the template itself supplies the Pi
print prompt.

## Structured Output

For workflows that need machine-readable final answers, use a final-only schema pass in Pi print mode: let Pi use tools normally, then force JSON schema on the final answer and validate it.

See [docs/structured-output.md](docs/structured-output.md).

Example:

```bash
localpager-agent \
  --final-schema ./examples/schemas/binary-classifier.schema.json \
  --prompt-template ./examples/prompts/binary-classifier.hbs \
  --prompt-vars-file ./examples/prompts/binary-classifier.vars.json
```

## Options

- `--backend <openai-compatible|pi-builtin>`: backend wiring mode. Default: `openai-compatible`
- `--base-url <url>`: OpenAI-compatible endpoint for the `openai-compatible` backend. Default: `http://127.0.0.1:1234/v1`
- `--model <id|auto>`: model id. Default: `auto`, meaning first id returned by `/v1/models` for the `openai-compatible` backend. The `pi-builtin` backend requires an explicit model and accepts `provider/model` shorthand.
- `--status`: print model/config status and exit
- `--provider-id <id>`: generated Pi provider id for `openai-compatible`, or Pi built-in provider id for `pi-builtin`. Default: `local-openai`
- `--state-dir <path>`: runtime state directory. Default: `~/.local/state/localpager-agent`
- `--session-dir <path>`: Pi session directory. Default: `<state-dir>/sessions`
- `--pi-command <command>`: Pi launch command. Default: `npx -y @earendil-works/pi-coding-agent@latest`
- `--thinking <level>`: Pi thinking level. Default: `off`
- `--context-window <n>`: generated model context window override. By default, localpager-agent uses model metadata when the server reports it and otherwise leaves this unset.
- `--max-tokens <n>`: max output tokens; for OpenAI-compatible backends this is
  also forwarded as request `max_tokens`. Default: `8192`
- `--temperature <n>`: OpenAI-compatible request temperature, from `0` to `2`
- `--top-p <n>`: OpenAI-compatible request `top_p`, from `0` to `1`
- `--seed <n>`: OpenAI-compatible non-negative integer request seed
- `--presence-penalty <n>`: OpenAI-compatible request `presence_penalty`, from `-2` to `2`
- `--frequency-penalty <n>`: OpenAI-compatible request `frequency_penalty`, from `-2` to `2`
- `--timeout-ms <n>`: `/v1/models` probe timeout. Default: `3000`
- `--final-schema <path>`: force the final answer through a JSON schema; requires Pi print mode (`-p` or `--print`)
- `--schema <path>`: alias for `--final-schema`
- `--prompt-template <path>`: render a maintained prompt template and pass it to Pi print mode
- `--prompt-vars-file <path>`: JSON object variables for `--prompt-template`; repeatable
- `--prompt-var <key=value>`: inline template variable override; repeatable
- `--write-rendered-prompt <path>`: write the rendered prompt text for audit/debugging

Pi tools are disabled by default, and caller-supplied Pi tool flags are not
accepted. `localpager-agent` owns Pi tool configuration: it exposes `final_json`
only when `--final-schema` is set, and exposes `bash` only through the reposhell
extension when a reposhell socket is configured. Other Pi tools such as read,
write, or edit are never passed through.

## Environment

- `LOCALPAGER_AGENT_BASE_URL`
- `LOCALPAGER_AGENT_BACKEND`
- `LOCALPAGER_AGENT_MODEL`
- `LOCALPAGER_AGENT_PROVIDER_ID`
- `LOCALPAGER_AGENT_STATE_DIR`
- `LOCALPAGER_AGENT_SESSION_DIR`
- `LOCALPAGER_AGENT_PI_CMD`
- `LOCALPAGER_AGENT_THINKING`
- `LOCALPAGER_AGENT_CONTEXT_WINDOW`
- `LOCALPAGER_AGENT_MAX_TOKENS`
- `LOCALPAGER_AGENT_TEMPERATURE`
- `LOCALPAGER_AGENT_TOP_P`
- `LOCALPAGER_AGENT_SEED`
- `LOCALPAGER_AGENT_PRESENCE_PENALTY`
- `LOCALPAGER_AGENT_FREQUENCY_PENALTY`
- `LOCALPAGER_AGENT_TIMEOUT_MS`
- `LOCALPAGER_AGENT_FINAL_SCHEMA`
- `LOCALPAGER_AGENT_PROMPT_TEMPLATE`
- `LOCALPAGER_AGENT_PROMPT_VARS_FILE`
- `LOCALPAGER_AGENT_WRITE_RENDERED_PROMPT`

## Development

```bash
npm run format
npm run lint
npm run typecheck
npm test
npm run build
npm run check
```
