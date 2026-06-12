import { Command } from "commander";

export function createLocalpagerAgentCommand(): Command {
  return new Command()
    .name("localpager-agent")
    .description("pi, automatically wired to an OpenAI-compatible endpoint or Pi built-in provider")
    .usage("[localpager-agent options] [pi options/messages]")
    .allowUnknownOption(true)
    .allowExcessArguments(true)
    .exitOverride()
    .configureOutput({ writeOut: () => undefined, writeErr: () => undefined })
    .helpOption("-h, --help", "show this help")
    .option("--backend <name>", "openai-compatible or pi-builtin; default openai-compatible")
    .option("--base-url <url>", "OpenAI-compatible endpoint for openai-compatible backend")
    .option(
      "--model <id|auto>",
      "model to use; auto selects the first /v1/models id for openai-compatible"
    )
    .option("--status", "print local model and runtime config status")
    .option(
      "--provider-id <id>",
      "generated Pi provider id, or Pi built-in provider for pi-builtin"
    )
    .option("--state-dir <path>", "localpager-agent runtime state directory")
    .option("--session-dir <path>", "Pi session directory")
    .option("--pi-command <command>", "Pi launch command")
    .option("--thinking <level>", "Pi thinking level; default off")
    .option("--context-window <n>", "generated model context window")
    .option("--max-tokens <n>", "max output tokens; forwarded as max_tokens for openai-compatible")
    .option("--temperature <n>", "OpenAI-compatible request temperature")
    .option("--top-p <n>", "OpenAI-compatible request top_p")
    .option("--seed <n>", "OpenAI-compatible request seed")
    .option("--presence-penalty <n>", "OpenAI-compatible request presence_penalty")
    .option("--frequency-penalty <n>", "OpenAI-compatible request frequency_penalty")
    .option("--timeout-ms <n>", "/v1/models probe timeout")
    .option("--final-schema, --schema <path>", "force final schema output; requires Pi -p/--print")
    .option("--no-final-schema-instruction", "do not append the final_json system instruction")
    .option("--prompt-template <path>", "render a prompt template and pass it to Pi print mode")
    .option(
      "--prompt-vars-file <path>",
      "JSON object variables for --prompt-template; repeatable",
      collectValues,
      []
    )
    .option(
      "--prompt-var <key=value>",
      "inline template variable override; repeatable",
      collectValues,
      []
    )
    .option("--write-rendered-prompt <path>", "write rendered prompt text for audit/debugging")
    .option("--reposhell-socket <path>", "Unix socket for Localpager read-only bash")
    .option("--reposhell-default-repo <id>", "default repo id for read-only bash")
    .option(
      "--reposhell-visible-repos <ids>",
      "comma-separated repo ids visible to read-only bash"
    );
}

export function localpagerAgentUsage(): string {
  return `${createLocalpagerAgentCommand().helpInformation()}${helpFooter()}`;
}

function collectValues(value: string, previous: string[] = []): string[] {
  return [...previous, value];
}

function helpFooter(): string {
  return [
    "",
    "notes:",
    "  Pi tool flags are not accepted; Localpager owns final_json and reposhell bash exposure",
    "  Pi context-file discovery is disabled; AGENTS.md and CLAUDE.md are not loaded",
    "",
    "examples:",
    "  localpager-agent --status",
    '  localpager-agent -p "summarize this repo"',
    '  localpager-agent --model gemma-4-e4b-it -p "write a long implementation plan"',
    '  localpager-agent --backend pi-builtin --model openai-codex/gpt-5.3-codex-spark -p "classify this item"',
    "  localpager-agent -- --help",
    ""
  ].join("\n");
}
