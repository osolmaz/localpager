import path from "node:path";

import { CommanderError, type Command } from "commander";

import { createLocalpagerAgentCommand, localpagerAgentUsage } from "./command.js";
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
  const command = createLocalpagerAgentCommand();
  parseCommand(command, args);
  const parsed = command.opts<CommanderOptions>();
  return applyCommanderOptions(options, parsed, forwardedPiArgs(args, command));
}

function parseCommand(command: Command, args: readonly string[]): void {
  try {
    command.parse([...args], { from: "user" });
  } catch (error) {
    if (error instanceof CommanderError && error.code === "commander.optionMissingArgument") {
      throw new Error(missingArgumentMessage(command, args, error.message));
    }
    throw error;
  }
}

function missingArgumentMessage(
  command: Command,
  args: readonly string[],
  message: string
): string {
  const typedFlag = missingValueFlag(command, args);
  if (typedFlag !== undefined) {
    return `${typedFlag} requires a value`;
  }
  const match = /option '([^']+)'/u.exec(message);
  const flagWithValue = match?.[1];
  const flag = flagWithValue?.split(/[ ,]/u)[0] ?? "option";
  return `${flag} requires a value`;
}

function missingValueFlag(command: Command, args: readonly string[]): string | undefined {
  const optionIndex = localOptionIndex(command);
  for (let index = 0; index < args.length; index += 1) {
    const arg = args[index];
    if (arg === "--") {
      return undefined;
    }
    if (arg === undefined || !optionIndex.valueFlags.has(arg)) {
      continue;
    }
    if (args[index + 1] === undefined) {
      return arg;
    }
    index += 1;
  }
  return undefined;
}

function applyCommanderOptions(
  options: LocalpagerAgentOptions,
  parsed: CommanderOptions,
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
    finalSchemaPath: optionalValue(parsed.schema, options.finalSchemaPath),
    finalSchemaInstruction: optionalValue(parsed.finalSchemaInstruction, true)
      ? options.finalSchemaInstruction
      : false,
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
  return localpagerAgentUsage();
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
  readonly schema?: string;
  readonly finalSchemaInstruction?: boolean;
  readonly promptTemplate?: string;
  readonly promptVarsFile?: readonly string[];
  readonly promptVar?: readonly string[];
  readonly writeRenderedPrompt?: string;
  readonly reposhellSocket?: string;
  readonly reposhellDefaultRepo?: string;
  readonly reposhellVisibleRepos?: string;
  readonly status?: boolean;
};

function hasLocalHelp(args: readonly string[]): boolean {
  return hasLocalFlag(args, "-h") || hasLocalFlag(args, "--help");
}

type LocalOptionIndex = {
  readonly booleanFlags: ReadonlySet<string>;
  readonly valueFlags: ReadonlySet<string>;
};

function forwardedPiArgs(args: readonly string[], command: Command): readonly string[] {
  const optionIndex = localOptionIndex(command);
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
    const advance = localFlagAdvance(arg, optionIndex);
    if (advance !== undefined) {
      index += advance;
      continue;
    }
    forwardedArgs.push(arg);
  }
  return forwardedArgs;
}

function localOptionIndex(command: Command): LocalOptionIndex {
  const booleanFlags = new Set<string>();
  const valueFlags = new Set<string>();
  for (const option of command.options) {
    const target = option.required || option.optional ? valueFlags : booleanFlags;
    if (option.short !== undefined) {
      target.add(option.short);
    }
    if (option.long !== undefined) {
      target.add(option.long);
    }
  }
  return { booleanFlags, valueFlags };
}

function localFlagAdvance(arg: string, optionIndex: LocalOptionIndex): number | undefined {
  if (optionIndex.booleanFlags.has(arg)) {
    return 0;
  }
  if (optionIndex.valueFlags.has(arg)) {
    return 1;
  }
  return isInlineLocalValueFlag(arg, optionIndex) ? 0 : undefined;
}

function isInlineLocalValueFlag(arg: string, optionIndex: LocalOptionIndex): boolean {
  for (const flag of optionIndex.valueFlags) {
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
