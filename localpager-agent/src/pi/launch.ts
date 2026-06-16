import { spawn } from "node:child_process";
import type { StdioOptions } from "node:child_process";
import { mkdir } from "node:fs/promises";

import type { LocalpagerAgentOptions } from "../agent/options.js";
import type { ReposhellRuntime } from "../reposhell/bash-extension.js";
import type { SamplingRuntime } from "../sampling/request-extension.js";
import type { FinalSchemaRuntime } from "../structured/final-schema.js";
import type { RuntimeConfig } from "./config.js";

export type LaunchPlan = {
  readonly command: string;
  readonly args: readonly string[];
  readonly env: Readonly<Record<string, string>>;
  readonly stdinMode: "inherit" | "ignore";
  readonly finalSchemaOutputPath: string | undefined;
};

export async function createLaunchPlan(
  options: LocalpagerAgentOptions,
  runtimeConfig: RuntimeConfig,
  model: string,
  finalSchemaRuntime?: FinalSchemaRuntime,
  reposhellRuntime?: ReposhellRuntime,
  samplingRuntime?: SamplingRuntime
): Promise<LaunchPlan> {
  await mkdir(options.sessionDir, { recursive: true });
  const forwardedArgs =
    finalSchemaRuntime === undefined
      ? plainOutputArgs(options.forwardedArgs, reposhellRuntime, samplingRuntime)
      : structuredOutputArgs(
          options,
          options.forwardedArgs,
          finalSchemaRuntime,
          reposhellRuntime,
          samplingRuntime
        );
  return {
    command: options.piCommand,
    args: [
      "--provider",
      options.providerId,
      "--model",
      model,
      "--thinking",
      options.thinking,
      "--no-context-files",
      ...forwardedArgs
    ],
    finalSchemaOutputPath: finalSchemaRuntime?.outputPath,
    stdinMode: finalSchemaRuntime === undefined ? "inherit" : "ignore",
    env: {
      PI_CODING_AGENT_DIR: runtimeConfig.configDir,
      PI_CODING_AGENT_SESSION_DIR: options.sessionDir,
      PI_OFFLINE: process.env["PI_OFFLINE"] ?? "1",
      PI_TELEMETRY: process.env["PI_TELEMETRY"] ?? "0",
      PI_SKIP_VERSION_CHECK: process.env["PI_SKIP_VERSION_CHECK"] ?? "1"
    }
  };
}

export async function execLaunchPlan(plan: LaunchPlan): Promise<number> {
  const stdio: StdioOptions =
    plan.finalSchemaOutputPath === undefined ? "inherit" : [plan.stdinMode, "pipe", "inherit"];
  const child = spawn(shellCommand(plan.command, plan.args), {
    shell: true,
    stdio,
    env: { ...process.env, ...plan.env }
  });
  let stdoutTail = "";
  child.stdout?.on("data", (chunk: Buffer | string) => {
    stdoutTail = tailText(stdoutTail + chunk.toString());
  });
  return await new Promise<number>((resolve, reject) => {
    child.on("error", reject);
    child.on("exit", (code, signal) => {
      if (signal !== null) {
        process.kill(process.pid, signal);
        return;
      }
      if ((code ?? 0) !== 0 && stdoutTail.trim() !== "") {
        process.stderr.write(stdoutTail);
        if (!stdoutTail.endsWith("\n")) {
          process.stderr.write("\n");
        }
      }
      resolve(code ?? 0);
    });
  });
}

function tailText(value: string, maxChars = 8000): string {
  return value.length <= maxChars ? value : value.slice(value.length - maxChars);
}

function structuredOutputArgs(
  options: LocalpagerAgentOptions,
  forwardedArgs: readonly string[],
  runtime: FinalSchemaRuntime,
  reposhellRuntime: ReposhellRuntime | undefined,
  samplingRuntime: SamplingRuntime | undefined
): string[] {
  rejectCallerToolFlags(forwardedArgs);
  if (hasRpcMode(forwardedArgs)) {
    throw new Error("--final-schema cannot be used with --mode rpc");
  }
  if (!hasPrintMode(forwardedArgs)) {
    throw new Error("--final-schema requires Pi print mode (-p or --print)");
  }
  const args = withOwnedToolArgs(forwardedArgs, {
    reposhellEnabled: reposhellRuntime !== undefined,
    finalJsonRequired: true
  });
  return [
    ...structuredSystemPromptArgs(runtime, forwardedArgs),
    ...reposhellExtensionArgs(reposhellRuntime),
    ...samplingExtensionArgs(samplingRuntime),
    "--extension",
    runtime.extensionPath,
    ...finalSchemaInstructionArgs(options, runtime),
    ...args
  ];
}

export const plainSystemPrompt = [
  "You are localpager-agent, a fast assistant running on a local model.",
  "Answer directly and keep responses concise.",
  "Use only the tools available in this run, and only when needed."
].join("\n");

function plainOutputArgs(
  forwardedArgs: readonly string[],
  reposhellRuntime: ReposhellRuntime | undefined,
  samplingRuntime: SamplingRuntime | undefined
): string[] {
  rejectCallerToolFlags(forwardedArgs);
  if (reposhellRuntime === undefined) {
    return [
      ...plainSystemPromptArgs(forwardedArgs),
      ...samplingExtensionArgs(samplingRuntime),
      ...withOwnedToolArgs(forwardedArgs, {
        reposhellEnabled: false,
        finalJsonRequired: false
      })
    ];
  }
  return [
    ...plainSystemPromptArgs(forwardedArgs),
    ...reposhellExtensionArgs(reposhellRuntime),
    ...samplingExtensionArgs(samplingRuntime),
    ...withOwnedToolArgs(forwardedArgs, {
      reposhellEnabled: true,
      finalJsonRequired: false
    })
  ];
}

function plainSystemPromptArgs(forwardedArgs: readonly string[]): string[] {
  return hasSystemPrompt(forwardedArgs) ? [] : ["--system-prompt", plainSystemPrompt];
}

function reposhellExtensionArgs(runtime: ReposhellRuntime | undefined): string[] {
  return runtime === undefined
    ? []
    : ["--extension", runtime.extensionPath, "--append-system-prompt", runtime.instruction];
}

function finalSchemaInstructionArgs(
  options: LocalpagerAgentOptions,
  runtime: FinalSchemaRuntime
): string[] {
  return options.finalSchemaInstruction ? ["--append-system-prompt", runtime.instruction] : [];
}

function samplingExtensionArgs(runtime: SamplingRuntime | undefined): string[] {
  return runtime === undefined ? [] : ["--extension", runtime.extensionPath];
}

function hasRpcMode(args: readonly string[]): boolean {
  return args.some((arg, index) => arg === "--mode" && args[index + 1] === "rpc");
}

function hasPrintMode(args: readonly string[]): boolean {
  return args.includes("--print") || args.includes("-p");
}

function structuredSystemPromptArgs(
  runtime: FinalSchemaRuntime,
  forwardedArgs: readonly string[]
): string[] {
  return hasSystemPrompt(forwardedArgs) ? [] : ["--system-prompt", runtime.systemPrompt];
}

function hasSystemPrompt(args: readonly string[]): boolean {
  return args.includes("--system-prompt");
}

type ToolOptions = {
  readonly reposhellEnabled: boolean;
  readonly finalJsonRequired: boolean;
};

function withOwnedToolArgs(args: readonly string[], options: ToolOptions): string[] {
  rejectCallerToolFlags(args);
  const tools = defaultTools(options);
  return tools.length > 0 ? ["--tools", tools.join(","), ...args] : ["--no-tools", ...args];
}

function rejectCallerToolFlags(args: readonly string[]): void {
  const flag = args.find(
    (arg) => arg === "--tools" || arg === "-t" || arg === "--no-tools" || arg === "-nt"
  );
  if (flag !== undefined) {
    throw new Error(`${flag} is not accepted; localpager-agent owns Pi tool configuration`);
  }
}

function defaultTools(options: ToolOptions): string[] {
  const tools: string[] = [];
  if (options.reposhellEnabled) {
    tools.push("bash");
  }
  if (options.finalJsonRequired) {
    tools.push("final_json");
  }
  return tools;
}

function shellCommand(command: string, args: readonly string[]): string {
  return [command, ...args.map(shellQuote)].join(" ");
}

function shellQuote(value: string): string {
  return `'${value.replaceAll("'", "'\\''")}'`;
}
