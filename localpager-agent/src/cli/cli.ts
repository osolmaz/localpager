import { errorMessage, fail, ok, type CommandResult } from "../common/result.js";
import { parseLocalpagerAgentArgs, usage } from "../agent/options.js";
import type { LocalpagerAgentOptions } from "../agent/options.js";
import { resolveLocalModel } from "../llm/openai.js";
import { writeRuntimeConfig } from "../pi/config.js";
import type { RuntimeConfig } from "../pi/config.js";
import { createLaunchPlan, execLaunchPlan } from "../pi/launch.js";
import { promptForwardedArgs } from "../prompts/template.js";
import { createReposhellRuntime } from "../reposhell/bash-extension.js";
import type { ReposhellRuntime } from "../reposhell/bash-extension.js";
import { createSamplingRuntime } from "../sampling/request-extension.js";
import type { SamplingRuntime } from "../sampling/request-extension.js";
import { hasSamplingOptions, samplingRequestParams } from "../sampling/request-params.js";
import {
  createFinalSchemaRuntime,
  isMissingFinalSchemaOutputError,
  readFinalSchemaOutput
} from "../structured/final-schema.js";
import type { FinalSchemaRuntime } from "../structured/final-schema.js";
import { findFinalJsonRecoveryCandidate, recoveryForwardedArgs } from "../structured/recovery.js";

export async function run(args: readonly string[]): Promise<CommandResult> {
  try {
    const options = parseLocalpagerAgentArgs(args);
    if (options.forwardedArgs.length === 1 && options.forwardedArgs[0] === "--help") {
      return ok(usage());
    }

    const resolved = await resolveLocalModel(options.baseUrl, options.model, options.timeoutMs);
    const runtimeConfig = await writeRuntimeConfig(options, resolved.model, resolved.contextWindow);

    if (options.status) {
      return ok(statusOutput(options, resolved, runtimeConfig));
    }

    const runOptions = {
      ...options,
      forwardedArgs: await promptForwardedArgs(options)
    };
    const finalSchemaRuntime =
      runOptions.finalSchemaPath === undefined
        ? undefined
        : await createFinalSchemaRuntime(runOptions.finalSchemaPath, runOptions.stateDir);
    const reposhellRuntime = await createReposhellRuntime(runOptions);
    const samplingRuntime = await createSamplingRuntime(runOptions.sampling, runOptions.stateDir);
    const startedAtMs = Date.now();
    const plan = await createLaunchPlan(
      runOptions,
      runtimeConfig,
      resolved.model,
      finalSchemaRuntime,
      reposhellRuntime,
      samplingRuntime
    );
    const code = await execLaunchPlan(plan);
    if (code !== 0 || plan.finalSchemaOutputPath === undefined) {
      return { code, stdout: "", stderr: "" };
    }
    return await readFinalSchemaResult(
      plan.finalSchemaOutputPath,
      runOptions,
      runtimeConfig,
      resolved.model,
      finalSchemaRuntime,
      reposhellRuntime,
      samplingRuntime,
      startedAtMs
    );
  } catch (error) {
    return fail(`localpager-agent: ${errorMessage(error)}`);
  }
}

type ResolvedModel = {
  readonly model: string;
  readonly availableModels: readonly string[];
  readonly contextWindow?: number;
};

async function readFinalSchemaResult(
  outputPath: string,
  options: LocalpagerAgentOptions,
  runtimeConfig: RuntimeConfig,
  model: string,
  finalSchemaRuntime: FinalSchemaRuntime | undefined,
  reposhellRuntime: ReposhellRuntime | undefined,
  samplingRuntime: SamplingRuntime | undefined,
  startedAtMs: number
): Promise<CommandResult> {
  try {
    return ok(await readFinalSchemaOutput(outputPath));
  } catch (error) {
    if (!isMissingFinalSchemaOutputError(error) || finalSchemaRuntime === undefined) {
      throw error;
    }
    return await recoverFinalSchemaResult(
      options,
      runtimeConfig,
      model,
      finalSchemaRuntime,
      reposhellRuntime,
      samplingRuntime,
      startedAtMs,
      outputPath
    );
  }
}

async function recoverFinalSchemaResult(
  options: LocalpagerAgentOptions,
  runtimeConfig: RuntimeConfig,
  model: string,
  finalSchemaRuntime: FinalSchemaRuntime,
  reposhellRuntime: ReposhellRuntime | undefined,
  samplingRuntime: SamplingRuntime | undefined,
  startedAtMs: number,
  outputPath: string
): Promise<CommandResult> {
  const candidate = await findFinalJsonRecoveryCandidate(options, startedAtMs);
  if (candidate === undefined) {
    throw new Error("final_json was not called; no structured output was captured");
  }
  const recoveryPlan = await createLaunchPlan(
    {
      ...options,
      forwardedArgs: recoveryForwardedArgs(
        options.forwardedArgs,
        candidate.sessionPath,
        candidate.payload
      )
    },
    runtimeConfig,
    model,
    finalSchemaRuntime,
    reposhellRuntime,
    samplingRuntime
  );
  const code = await execLaunchPlan(recoveryPlan);
  return code === 0
    ? ok(await readFinalSchemaOutput(outputPath))
    : { code, stdout: "", stderr: "" };
}

function statusOutput(
  options: ReturnType<typeof parseLocalpagerAgentArgs>,
  resolved: ResolvedModel,
  runtimeConfig: RuntimeConfig
): string {
  return (
    [
      `base url: ${options.baseUrl}`,
      `model: ${resolved.model}`,
      `available models: ${resolved.availableModels.join(", ")}`,
      `context window: ${String(options.contextWindow ?? resolved.contextWindow ?? "unspecified")}`,
      `provider id: ${options.providerId}`,
      `request params: ${requestParamsStatus(options)}`,
      `pi config dir: ${runtimeConfig.configDir}`,
      `session dir: ${options.sessionDir}`,
      `pi command: ${options.piCommand}`
    ].join("\n") + "\n"
  );
}

function requestParamsStatus(options: LocalpagerAgentOptions): string {
  return hasSamplingOptions(options.sampling)
    ? JSON.stringify(samplingRequestParams(options.sampling))
    : "unspecified";
}
