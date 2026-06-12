import path from "node:path";

import { Command } from "commander";

import { normalizeBaseUrl } from "../llm/openai.js";
import type { SamplingOptions } from "../sampling/request-params.js";
import { samplingOptionsFromEntries } from "../sampling/request-params.js";

export type LocalpagerAgentBackend = "openai-compatible" | "pi-builtin";

export type LocalpagerAgentOptions = {
  readonly backend: LocalpagerAgentBackend;
  readonly baseUrl: string;
  readonly model: string;
  readonly providerId: string;
  readonly stateDir: string;
  readonly sessionDir: string;
  readonly piCommand: string;
  readonly thinking: string;
  readonly contextWindow: number | undefined;
  readonly maxTokens: number;
  readonly sampling: SamplingOptions;
  readonly timeoutMs: number;
  readonly finalSchemaPath: string | undefined;
  readonly finalSchemaInstruction: boolean;
  readonly promptTemplatePath: string | undefined;
  readonly promptVarsPaths: readonly string[];
  readonly promptVars: readonly string[];
  readonly renderedPromptPath: string | undefined;
  readonly reposhellSocket: string | undefined;
  readonly reposhellDefaultRepo: string | undefined;
  readonly reposhellVisibleRepos: readonly string[];
  readonly status: boolean;
  readonly forwardedArgs: readonly string[];
};

export function defaultOptions(): LocalpagerAgentOptions {
  const home = envString("HOME", ".");
  const stateDir = envString(
    "LOCALPAGER_AGENT_STATE_DIR",
    path.join(home, ".local/state/localpager-agent")
  );
  return {
    backend: envBackend("LOCALPAGER_AGENT_BACKEND", "openai-compatible"),
    baseUrl: normalizeBaseUrl(envString("LOCALPAGER_AGENT_BASE_URL", "http://127.0.0.1:1234/v1")),
    model: envString("LOCALPAGER_AGENT_MODEL", "auto"),
    providerId: envString("LOCALPAGER_AGENT_PROVIDER_ID", "local-openai"),
    stateDir,
    sessionDir: defaultSessionDir(stateDir),
    piCommand: envString(
      "LOCALPAGER_AGENT_PI_CMD",
      "npx -y @earendil-works/pi-coding-agent@latest"
    ),
    thinking: envString("LOCALPAGER_AGENT_THINKING", "off"),
    contextWindow: envOptionalPositiveInteger("LOCALPAGER_AGENT_CONTEXT_WINDOW"),
    maxTokens: envPositiveInteger("LOCALPAGER_AGENT_MAX_TOKENS", "8192"),
    sampling: envSamplingOptions(),
    timeoutMs: envPositiveInteger("LOCALPAGER_AGENT_TIMEOUT_MS", "3000"),
    finalSchemaPath: process.env["LOCALPAGER_AGENT_FINAL_SCHEMA"],
    finalSchemaInstruction: envBoolean("LOCALPAGER_AGENT_FINAL_SCHEMA_INSTRUCTION", true),
    promptTemplatePath: process.env["LOCALPAGER_AGENT_PROMPT_TEMPLATE"],
    promptVarsPaths: splitCSV(process.env["LOCALPAGER_AGENT_PROMPT_VARS_FILE"] ?? ""),
    promptVars: [],
    renderedPromptPath: process.env["LOCALPAGER_AGENT_WRITE_RENDERED_PROMPT"],
    reposhellSocket: process.env["LOCALPAGER_REPOSHELL_SOCKET"],
    reposhellDefaultRepo: process.env["LOCALPAGER_REPOSHELL_DEFAULT_REPO"],
    reposhellVisibleRepos: splitCSV(process.env["LOCALPAGER_REPOSHELL_VISIBLE_REPOS"] ?? ""),
    status: false,
    forwardedArgs: []
  };
}

export function parseLocalpagerAgentArgs(args: readonly string[]): LocalpagerAgentOptions {
  const options = defaultOptions();
  if (hasLocalHelp(args)) {
    return { ...options, forwardedArgs: ["--help"] };
  }
  const command = localpagerAgentCommand();
  command.parse([...args], { from: "user" });
  const parsed = command.opts<CommanderOptions>();
  return applyCommanderOptions(options, parsed, args, forwardedPiArgs(args));
}

function applyCommanderOptions(
  options: LocalpagerAgentOptions,
  parsed: CommanderOptions,
  args: readonly string[],
  forwardedArgs: readonly string[]
): LocalpagerAgentOptions {
  return {
    ...options,
    backend: optionalParsed(parsed.backend, options.backend, parseBackend),
    baseUrl: optionalParsed(parsed.baseUrl, options.baseUrl, normalizeBaseUrl),
    model: optionalValue(parsed.model, options.model),
    providerId: optionalValue(parsed.providerId, options.providerId),
    stateDir: optionalValue(parsed.stateDir, options.stateDir),
    sessionDir: optionalValue(parsed.sessionDir, options.sessionDir),
    piCommand: optionalValue(parsed.piCommand, options.piCommand),
    thinking: optionalValue(parsed.thinking, options.thinking),
    contextWindow: optionalParsed(
      parsed.contextWindow,
      options.contextWindow,
      parsePositiveInteger
    ),
    maxTokens: optionalParsed(parsed.maxTokens, options.maxTokens, parsePositiveInteger),
    sampling: parsedSamplingOptions(options.sampling, parsed),
    timeoutMs: optionalParsed(parsed.timeoutMs, options.timeoutMs, parsePositiveInteger),
    finalSchemaPath: optionalValue(
      lastLocalOptionValue(args, ["--final-schema", "--schema"]),
      options.finalSchemaPath
    ),
    finalSchemaInstruction: resolvedFinalSchemaInstruction(options, args),
    promptTemplatePath: optionalValue(parsed.promptTemplate, options.promptTemplatePath),
    promptVarsPaths: appendedValues(options.promptVarsPaths, parsed.promptVarsFile),
    promptVars: appendedValues(options.promptVars, parsed.promptVar),
    renderedPromptPath: optionalValue(parsed.writeRenderedPrompt, options.renderedPromptPath),
    reposhellSocket: optionalValue(parsed.reposhellSocket, options.reposhellSocket),
    reposhellDefaultRepo: optionalValue(parsed.reposhellDefaultRepo, options.reposhellDefaultRepo),
    reposhellVisibleRepos: optionalParsed(
      parsed.reposhellVisibleRepos,
      options.reposhellVisibleRepos,
      splitCSV
    ),
    status: optionalValue(parsed.status, options.status),
    forwardedArgs
  };
}

function optionalValue<T>(value: T | undefined, fallback: T): T {
  return value ?? fallback;
}

function optionalParsed<T>(value: string | undefined, fallback: T, parse: (value: string) => T): T {
  return value === undefined ? fallback : parse(value);
}

function appendedValues<T>(current: readonly T[], next: readonly T[] | undefined): readonly T[] {
  return next === undefined ? current : [...current, ...next];
}

function resolvedFinalSchemaInstruction(
  options: LocalpagerAgentOptions,
  args: readonly string[]
): boolean {
  return hasLocalFlag(args, "--no-final-schema-instruction")
    ? false
    : options.finalSchemaInstruction;
}

function parsedSamplingOptions(
  current: SamplingOptions,
  parsed: CommanderOptions
): SamplingOptions {
  return samplingOptionsFromEntries({
    ...current,
    temperature: optionalParsed(parsed.temperature, current.temperature, parseTemperature),
    topP: optionalParsed(parsed.topP, current.topP, parseTopP),
    seed: optionalParsed(parsed.seed, current.seed, parseSeed),
    presencePenalty: optionalParsed(
      parsed.presencePenalty,
      current.presencePenalty,
      parsePresencePenalty
    ),
    frequencyPenalty: optionalParsed(
      parsed.frequencyPenalty,
      current.frequencyPenalty,
      parseFrequencyPenalty
    )
  });
}

function parseTemperature(value: string): number {
  return parseBoundedNumber(value, "--temperature", 0, 2);
}

function parseTopP(value: string): number {
  return parseBoundedNumber(value, "--top-p", 0, 1);
}

function parseSeed(value: string): number {
  return parseNonNegativeInteger(value, "--seed");
}

function parsePresencePenalty(value: string): number {
  return parseBoundedNumber(value, "--presence-penalty", -2, 2);
}

function parseFrequencyPenalty(value: string): number {
  return parseBoundedNumber(value, "--frequency-penalty", -2, 2);
}

export function usage(): string {
  return `${localpagerAgentCommand().helpInformation()}${helpFooter()}`;
}

type CommanderOptions = {
  readonly backend?: string;
  readonly baseUrl?: string;
  readonly model?: string;
  readonly providerId?: string;
  readonly stateDir?: string;
  readonly sessionDir?: string;
  readonly piCommand?: string;
  readonly thinking?: string;
  readonly contextWindow?: string;
  readonly maxTokens?: string;
  readonly temperature?: string;
  readonly topP?: string;
  readonly seed?: string;
  readonly presencePenalty?: string;
  readonly frequencyPenalty?: string;
  readonly timeoutMs?: string;
  readonly finalSchema?: string;
  readonly schema?: string;
  readonly promptTemplate?: string;
  readonly promptVarsFile?: readonly string[];
  readonly promptVar?: readonly string[];
  readonly writeRenderedPrompt?: string;
  readonly reposhellSocket?: string;
  readonly reposhellDefaultRepo?: string;
  readonly reposhellVisibleRepos?: string;
  readonly status?: boolean;
};

function localpagerAgentCommand(): Command {
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
    .option("--final-schema <path>", "force final schema output; requires Pi -p/--print")
    .option("--schema <path>", "alias for --final-schema")
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

function hasLocalHelp(args: readonly string[]): boolean {
  return hasLocalFlag(args, "-h") || hasLocalFlag(args, "--help");
}

const localBooleanFlags = new Set(["--status", "--no-final-schema-instruction", "-h", "--help"]);

const localValueFlags = new Set([
  "--backend",
  "--base-url",
  "--model",
  "--provider-id",
  "--state-dir",
  "--session-dir",
  "--pi-command",
  "--thinking",
  "--context-window",
  "--max-tokens",
  "--temperature",
  "--top-p",
  "--seed",
  "--presence-penalty",
  "--frequency-penalty",
  "--timeout-ms",
  "--final-schema",
  "--schema",
  "--prompt-template",
  "--prompt-vars-file",
  "--prompt-var",
  "--write-rendered-prompt",
  "--reposhell-socket",
  "--reposhell-default-repo",
  "--reposhell-visible-repos"
]);

function forwardedPiArgs(args: readonly string[]): readonly string[] {
  const forwardedArgs: string[] = [];
  for (let index = 0; index < args.length; index += 1) {
    const arg = args[index];
    if (arg === "--") {
      forwardedArgs.push(...args.slice(index + 1));
      break;
    }
    if (arg === undefined) {
      continue;
    }
    const advance = localFlagAdvance(arg);
    if (advance !== undefined) {
      index += advance;
      continue;
    }
    forwardedArgs.push(arg);
  }
  return forwardedArgs;
}

function localFlagAdvance(arg: string | undefined): number | undefined {
  if (arg === undefined) {
    return undefined;
  }
  if (localBooleanFlags.has(arg)) {
    return 0;
  }
  if (localValueFlags.has(arg)) {
    return 1;
  }
  return isInlineLocalValueFlag(arg) ? 0 : undefined;
}

function isInlineLocalValueFlag(arg: string): boolean {
  for (const flag of localValueFlags) {
    if (arg.startsWith(`${flag}=`)) {
      return true;
    }
  }
  return false;
}

function hasLocalFlag(args: readonly string[], flag: string): boolean {
  for (const arg of args) {
    if (arg === "--") {
      return false;
    }
    if (arg === flag) {
      return true;
    }
  }
  return false;
}

function lastLocalOptionValue(
  args: readonly string[],
  names: readonly string[]
): string | undefined {
  let value: string | undefined;
  for (let index = 0; index < args.length; index += 1) {
    const arg = args[index];
    if (arg === "--") {
      return value;
    }
    if (arg === undefined) {
      continue;
    }
    if (names.includes(arg)) {
      value = requiredValue(args, index + 1, arg);
      index += 1;
      continue;
    }
    for (const name of names) {
      const prefix = `${name}=`;
      if (arg.startsWith(prefix)) {
        value = arg.slice(prefix.length);
        break;
      }
    }
  }
  return value;
}

function envString(name: string, fallback: string): string {
  return process.env[name] ?? fallback;
}

function envBoolean(name: string, fallback: boolean): boolean {
  const value = process.env[name];
  if (value === undefined) {
    return fallback;
  }
  if (["1", "true", "yes", "on"].includes(value.toLowerCase())) {
    return true;
  }
  if (["0", "false", "no", "off"].includes(value.toLowerCase())) {
    return false;
  }
  throw new Error(`${name} must be a boolean`);
}

function envBackend(name: string, fallback: LocalpagerAgentBackend): LocalpagerAgentBackend {
  return parseBackend(envString(name, fallback));
}

function parseBackend(value: string): LocalpagerAgentBackend {
  if (value === "openai-compatible" || value === "pi-builtin") {
    return value;
  }
  throw new Error(
    `invalid backend ${JSON.stringify(value)}; expected openai-compatible or pi-builtin`
  );
}

function envPositiveInteger(name: string, fallback: string): number {
  return parsePositiveInteger(envString(name, fallback));
}

function envOptionalPositiveInteger(name: string): number | undefined {
  const value = process.env[name];
  return value === undefined ? undefined : parsePositiveInteger(value);
}

function envSamplingOptions(): SamplingOptions {
  return samplingOptionsFromEntries({
    temperature: envOptionalBoundedNumber("LOCALPAGER_AGENT_TEMPERATURE", 0, 2),
    topP: envOptionalBoundedNumber("LOCALPAGER_AGENT_TOP_P", 0, 1),
    seed: envOptionalNonNegativeInteger("LOCALPAGER_AGENT_SEED"),
    presencePenalty: envOptionalBoundedNumber("LOCALPAGER_AGENT_PRESENCE_PENALTY", -2, 2),
    frequencyPenalty: envOptionalBoundedNumber("LOCALPAGER_AGENT_FREQUENCY_PENALTY", -2, 2)
  });
}

function envOptionalBoundedNumber(name: string, min: number, max: number): number | undefined {
  const value = process.env[name];
  return value === undefined ? undefined : parseBoundedNumber(value, name, min, max);
}

function envOptionalNonNegativeInteger(name: string): number | undefined {
  const value = process.env[name];
  return value === undefined ? undefined : parseNonNegativeInteger(value, name);
}

function defaultSessionDir(stateDir: string): string {
  return envString(
    "LOCALPAGER_AGENT_SESSION_DIR",
    envString("PI_CODING_AGENT_SESSION_DIR", path.join(stateDir, "sessions"))
  );
}

function requiredValue(args: readonly string[], index: number, flag: string): string {
  const value = args[index];
  if (value === undefined) {
    throw new Error(`${flag} requires a value`);
  }
  return value;
}

function parsePositiveInteger(value: string): number {
  if (!/^[1-9]\d*$/u.test(value)) {
    throw new Error(`expected a positive integer, got ${value}`);
  }
  return Number.parseInt(value, 10);
}

function parseNonNegativeInteger(value: string, label: string): number {
  if (!/^(0|[1-9]\d*)$/u.test(value)) {
    throw new Error(`${label} must be a non-negative integer, got ${value}`);
  }
  const parsed = Number.parseInt(value, 10);
  if (!Number.isSafeInteger(parsed)) {
    throw new Error(`${label} must be a safe integer, got ${value}`);
  }
  return parsed;
}

function parseBoundedNumber(value: string, label: string, min: number, max: number): number {
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed < min || parsed > max) {
    throw new Error(
      `${label} must be a number between ${String(min)} and ${String(max)}, got ${value}`
    );
  }
  return parsed;
}

function splitCSV(value: string): string[] {
  return value
    .split(",")
    .map((part) => part.trim())
    .filter((part) => part.length > 0);
}
