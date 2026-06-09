import path from "node:path";

import { normalizeBaseUrl } from "../llm/openai.js";
import type { SamplingOptions } from "../sampling/request-params.js";
import { samplingOptionsFromEntries } from "../sampling/request-params.js";

export type LocalpagerAgentOptions = {
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
  let options = defaultOptions();
  const forwardedArgs: string[] = [];
  for (let index = 0; index < args.length; index += 1) {
    const arg = args[index];
    if (arg === undefined) {
      continue;
    }
    if (arg === "--") {
      forwardedArgs.push(...args.slice(index + 1));
      break;
    }
    if (arg === "-h" || arg === "--help") {
      return { ...options, forwardedArgs: ["--help"] };
    }
    const parsed = parseLocalpagerAgentFlag(options, args, index);
    if (parsed !== undefined) {
      options = parsed.options;
      index += parsed.advance;
      continue;
    }
    forwardedArgs.push(arg);
  }
  return { ...options, forwardedArgs };
}

export function usage(): string {
  return `${[
    "localpager-agent - pi, automatically pointed at a local OpenAI-compatible model",
    "",
    "usage:",
    "  localpager-agent [localpager-agent options] [pi options/messages]",
    "",
    "localpager-agent options:",
    "  --base-url <url>          local OpenAI-compatible endpoint",
    "  --model <id|auto>         model to use; auto selects the first /v1/models id",
    "  --status                  print local model and runtime config status",
    "  --provider-id <id>        generated Pi provider id",
    "  --state-dir <path>        localpager-agent runtime state directory",
    "  --session-dir <path>      Pi session directory",
    "  --pi-command <command>    Pi launch command",
    "  --thinking <level>        Pi thinking level; default off",
    "  --context-window <n>      generated model context window",
    "  --max-tokens <n>          generated model max output tokens",
    "  --temperature <n>         OpenAI-compatible request temperature",
    "  --top-p <n>               OpenAI-compatible request top_p",
    "  --seed <n>                OpenAI-compatible request seed",
    "  --presence-penalty <n>    OpenAI-compatible request presence_penalty",
    "  --frequency-penalty <n>   OpenAI-compatible request frequency_penalty",
    "  --timeout-ms <n>          /v1/models probe timeout",
    "  --final-schema <path>     force final schema output; requires Pi -p/--print",
    "  --schema <path>           alias for --final-schema",
    "  --prompt-template <path>  render a prompt template and pass it to Pi print mode",
    "  --prompt-vars-file <path>",
    "                            JSON object variables for --prompt-template; repeatable",
    "  --prompt-var <key=value>  inline template variable override; repeatable",
    "  --write-rendered-prompt <path>",
    "                            write rendered prompt text for audit/debugging",
    "  --reposhell-socket <p>  Unix socket for Localpager read-only bash",
    "  --reposhell-default-repo <id>",
    "                            default repo id for read-only bash",
    "  --reposhell-visible-repos <ids>",
    "                            comma-separated repo ids visible to read-only bash",
    "  -h, --help                show this help",
    "",
    "examples:",
    "  localpager-agent --status",
    '  localpager-agent -p "summarize this repo"',
    '  localpager-agent --model gemma-4-e4b-it -p "write a long implementation plan"',
    "  localpager-agent -- --help"
  ].join("\n")}\n`;
}

type ParseResult = {
  readonly options: LocalpagerAgentOptions;
  readonly advance: number;
};

function parseLocalpagerAgentFlag(
  options: LocalpagerAgentOptions,
  args: readonly string[],
  index: number
): ParseResult | undefined {
  const arg = args[index];
  if (arg === "--status") {
    return { options: { ...options, status: true }, advance: 0 };
  }
  return arg === undefined ? undefined : parseValueFlag(options, args, index, arg);
}

type OptionUpdater = (options: LocalpagerAgentOptions, value: string) => LocalpagerAgentOptions;

const valueFlagUpdaters: Readonly<Record<string, OptionUpdater>> = {
  "--base-url": (options, value) => ({ ...options, baseUrl: normalizeBaseUrl(value) }),
  "--model": (options, value) => ({ ...options, model: value }),
  "--provider-id": (options, value) => ({ ...options, providerId: value }),
  "--state-dir": (options, value) => ({ ...options, stateDir: value }),
  "--session-dir": (options, value) => ({ ...options, sessionDir: value }),
  "--pi-command": (options, value) => ({ ...options, piCommand: value }),
  "--thinking": (options, value) => ({ ...options, thinking: value }),
  "--context-window": (options, value) => ({
    ...options,
    contextWindow: parsePositiveInteger(value)
  }),
  "--max-tokens": (options, value) => ({ ...options, maxTokens: parsePositiveInteger(value) }),
  "--temperature": (options, value) => ({
    ...options,
    sampling: { ...options.sampling, temperature: parseBoundedNumber(value, "--temperature", 0, 2) }
  }),
  "--top-p": (options, value) => ({
    ...options,
    sampling: { ...options.sampling, topP: parseBoundedNumber(value, "--top-p", 0, 1) }
  }),
  "--seed": (options, value) => ({
    ...options,
    sampling: { ...options.sampling, seed: parseNonNegativeInteger(value, "--seed") }
  }),
  "--presence-penalty": (options, value) => ({
    ...options,
    sampling: {
      ...options.sampling,
      presencePenalty: parseBoundedNumber(value, "--presence-penalty", -2, 2)
    }
  }),
  "--frequency-penalty": (options, value) => ({
    ...options,
    sampling: {
      ...options.sampling,
      frequencyPenalty: parseBoundedNumber(value, "--frequency-penalty", -2, 2)
    }
  }),
  "--timeout-ms": (options, value) => ({ ...options, timeoutMs: parsePositiveInteger(value) }),
  "--final-schema": (options, value) => ({ ...options, finalSchemaPath: value }),
  "--schema": (options, value) => ({ ...options, finalSchemaPath: value }),
  "--prompt-template": (options, value) => ({ ...options, promptTemplatePath: value }),
  "--prompt-vars-file": (options, value) => ({
    ...options,
    promptVarsPaths: [...options.promptVarsPaths, value]
  }),
  "--prompt-var": (options, value) => ({
    ...options,
    promptVars: [...options.promptVars, value]
  }),
  "--write-rendered-prompt": (options, value) => ({
    ...options,
    renderedPromptPath: value
  }),
  "--reposhell-socket": (options, value) => ({ ...options, reposhellSocket: value }),
  "--reposhell-default-repo": (options, value) => ({
    ...options,
    reposhellDefaultRepo: value
  }),
  "--reposhell-visible-repos": (options, value) => ({
    ...options,
    reposhellVisibleRepos: splitCSV(value)
  })
};

function parseValueFlag(
  options: LocalpagerAgentOptions,
  args: readonly string[],
  index: number,
  flag: string
): ParseResult | undefined {
  const updater = valueFlagUpdaters[flag];
  if (updater === undefined) {
    return undefined;
  }
  return { options: updater(options, requiredValue(args, index + 1, flag)), advance: 1 };
}

function envString(name: string, fallback: string): string {
  return process.env[name] ?? fallback;
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
