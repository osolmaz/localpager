import { spawn } from "node:child_process";
import type { StdioOptions } from "node:child_process";
import { mkdir } from "node:fs/promises";

import type { LocalpagerAgentOptions } from "../agent/options.js";
import type { RepoReaderRuntime } from "../repo-reader/bash-extension.js";
import type { FinalSchemaRuntime } from "../structured/final-schema.js";
import type { RuntimeConfig } from "./config.js";

export type LaunchPlan = {
  readonly command: string;
  readonly args: readonly string[];
  readonly env: Readonly<Record<string, string>>;
  readonly finalSchemaOutputPath: string | undefined;
};

export async function createLaunchPlan(
  options: LocalpagerAgentOptions,
  runtimeConfig: RuntimeConfig,
  model: string,
  finalSchemaRuntime?: FinalSchemaRuntime,
  repoReaderRuntime?: RepoReaderRuntime
): Promise<LaunchPlan> {
  await mkdir(options.sessionDir, { recursive: true });
  const forwardedArgs =
    finalSchemaRuntime === undefined
      ? [...options.forwardedArgs]
      : structuredOutputArgs(options.forwardedArgs, finalSchemaRuntime, repoReaderRuntime);
  return {
    command: options.piCommand,
    args: [
      "--provider",
      options.providerId,
      "--model",
      model,
      "--thinking",
      options.thinking,
      ...forwardedArgs
    ],
    finalSchemaOutputPath: finalSchemaRuntime?.outputPath,
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
    plan.finalSchemaOutputPath === undefined ? "inherit" : ["inherit", "pipe", "inherit"];
  const child = spawn(shellCommand(plan.command, plan.args), {
    shell: true,
    stdio,
    env: { ...process.env, ...plan.env }
  });
  child.stdout?.resume();
  return await new Promise<number>((resolve, reject) => {
    child.on("error", reject);
    child.on("exit", (code, signal) => {
      if (signal !== null) {
        process.kill(process.pid, signal);
        return;
      }
      resolve(code ?? 0);
    });
  });
}

function structuredOutputArgs(
  forwardedArgs: readonly string[],
  runtime: FinalSchemaRuntime,
  repoReaderRuntime: RepoReaderRuntime | undefined
): string[] {
  if (forwardedArgs.includes("--no-tools") || forwardedArgs.includes("-nt")) {
    throw new Error("--final-schema cannot be used with --no-tools");
  }
  if (hasRpcMode(forwardedArgs)) {
    throw new Error("--final-schema cannot be used with --mode rpc");
  }
  if (!hasPrintMode(forwardedArgs)) {
    throw new Error("--final-schema requires Pi print mode (-p or --print)");
  }
  const args = ensureAllowedTools(forwardedArgs, repoReaderRuntime !== undefined);
  const extensions =
    repoReaderRuntime === undefined
      ? []
      : [
          "--extension",
          repoReaderRuntime.extensionPath,
          "--append-system-prompt",
          repoReaderRuntime.instruction
        ];
  return [
    ...extensions,
    "--extension",
    runtime.extensionPath,
    "--append-system-prompt",
    runtime.instruction,
    ...args
  ];
}

function hasRpcMode(args: readonly string[]): boolean {
  return args.some((arg, index) => arg === "--mode" && args[index + 1] === "rpc");
}

function hasPrintMode(args: readonly string[]): boolean {
  return args.includes("--print") || args.includes("-p");
}

function ensureAllowedTools(args: readonly string[], repoReaderEnabled: boolean): string[] {
  const next = [...args];
  const index = toolsFlagIndex(next);
  if (index === -1) {
    return ["--tools", repoReaderEnabled ? "bash,final_json" : "final_json", ...next];
  }
  const flag = next[index];
  const value = next[index + 1];
  if (flag === undefined || value === undefined) {
    throw new Error(`${flag ?? "--tools"} requires a value`);
  }
  next[index + 1] = normalizeTools(value, repoReaderEnabled).join(",");
  return next;
}

function toolsFlagIndex(args: readonly string[]): number {
  return args.findIndex((arg) => arg === "--tools" || arg === "-t");
}

function normalizeTools(value: string, repoReaderEnabled: boolean): string[] {
  const tools = value
    .split(",")
    .map((tool) => tool.trim())
    .filter((tool) => tool.length > 0);
  if (tools.includes("bash") && !repoReaderEnabled) {
    throw new Error("--tools bash requires --repo-reader-socket");
  }
  if (repoReaderEnabled && !tools.includes("bash")) {
    tools.push("bash");
  }
  if (!tools.includes("final_json")) {
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
