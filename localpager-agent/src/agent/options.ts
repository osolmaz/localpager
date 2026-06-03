import path from "node:path";

import { normalizeBaseUrl } from "../llm/openai.js";

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
  readonly timeoutMs: number;
  readonly finalSchemaPath: string | undefined;
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
    timeoutMs: envPositiveInteger("LOCALPAGER_AGENT_TIMEOUT_MS", "3000"),
    finalSchemaPath: process.env["LOCALPAGER_AGENT_FINAL_SCHEMA"],
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
    "  --timeout-ms <n>          /v1/models probe timeout",
    "  --final-schema <path>     force final schema output; requires Pi -p/--print",
    "  --schema <path>           alias for --final-schema",
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
  "--timeout-ms": (options, value) => ({ ...options, timeoutMs: parsePositiveInteger(value) }),
  "--final-schema": (options, value) => ({ ...options, finalSchemaPath: value }),
  "--schema": (options, value) => ({ ...options, finalSchemaPath: value }),
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

function splitCSV(value: string): string[] {
  return value
    .split(",")
    .map((part) => part.trim())
    .filter((part) => part.length > 0);
}
