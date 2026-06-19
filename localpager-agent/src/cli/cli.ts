import { errorMessage, fail, ok, type CommandResult } from "../common/result.js";
import { parseLocalpagerAgentArgs, usage } from "../agent/options.js";
import type { LocalpagerAgentOptions } from "../agent/options.js";
import { resolveLocalModel } from "../llm/openai.js";
import type { ResolvedLocalModel } from "../llm/openai.js";
import { writeRuntimeConfig } from "../pi/config.js";
import type { RuntimeConfig, RuntimeModelMetadata } from "../pi/config.js";
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
import {
  findFinalJsonRecoveryCandidate,
  recoveryForwardedArgs,
  retryForwardedArgs
} from "../structured/recovery.js";

export async function run(args: readonly string[]): Promise<CommandResult> {
  try {
    const options = parseLocalpagerAgentArgs(args);
    if (options.forwardedArgs.length === 1 && options.forwardedArgs[0] === "--help") {
      return ok(usage());
    }

    const resolved = await resolveModel(options);
    const resolvedOptions = { ...options, providerId: resolved.providerId };
    const runtimeConfig = await writeRuntimeConfig(
      resolvedOptions,
      resolved.model,
      runtimeModelMetadata(options, resolved)
    );

    if (resolvedOptions.status) {
      return ok(statusOutput(resolvedOptions, resolved, runtimeConfig));
    }

    const runOptions = {
      ...resolvedOptions,
      forwardedArgs: await promptForwardedArgs(resolvedOptions)
    };
    const finalSchemaRuntime =
      runOptions.finalSchemaPath === undefined
        ? undefined
        : await createFinalSchemaRuntime(runOptions.finalSchemaPath, runOptions.stateDir);
    const reposhellRuntime = await createReposhellRuntime(runOptions);
    const samplingRuntime = await createSamplingRuntime(
      requestParamOptions(runOptions),
      runOptions.stateDir
    );
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

function requestParamOptions(options: LocalpagerAgentOptions): LocalpagerAgentOptions["sampling"] {
  if (options.backend !== "openai-compatible") {
    return options.sampling;
  }
  return { ...options.sampling, maxTokens: options.maxTokens };
}

type ResolvedModel = {
  readonly model: string;
  readonly providerId: string;
  readonly availableModels: readonly string[];
  readonly contextWindow?: number;
  readonly serverModelName?: string;
};

async function resolveModel(options: LocalpagerAgentOptions): Promise<ResolvedModel> {
  if (options.backend === "openai-compatible") {
    const resolved = await resolveLocalModel(options.baseUrl, options.model, options.timeoutMs);
    return localModelWithProviderId(resolved, options.providerId);
  }
  return resolvePiBuiltinModel(options);
}

function localModelWithProviderId(resolved: ResolvedLocalModel, providerId: string): ResolvedModel {
  return { ...resolved, providerId };
}

function runtimeModelMetadata(
  options: LocalpagerAgentOptions,
  resolved: ResolvedModel
): RuntimeModelMetadata {
  return {
    requestedModel: options.model,
    availableModels: resolved.availableModels,
    ...(resolved.contextWindow === undefined ? {} : { contextWindow: resolved.contextWindow }),
    ...(resolved.serverModelName === undefined ? {} : { serverModelName: resolved.serverModelName })
  };
}

function resolvePiBuiltinModel(options: LocalpagerAgentOptions): ResolvedModel {
  if (options.model === "auto") {
    throw new Error("pi-builtin backend requires an explicit --model");
  }
  const { providerId, model } = resolvePiBuiltinReference(options);
  const resolved = {
    model,
    providerId,
    availableModels: [`${providerId}/${model}`]
  };
  return options.contextWindow === undefined
    ? resolved
    : { ...resolved, contextWindow: options.contextWindow };
}

function resolvePiBuiltinReference(options: LocalpagerAgentOptions): {
  readonly providerId: string;
  readonly model: string;
} {
  if (options.providerId !== "local-openai") {
    return { providerId: options.providerId, model: options.model };
  }
  const parsed = parseProviderQualifiedModel(options.model);
  if (parsed !== undefined) {
    return parsed;
  }
  throw new Error(
    "pi-builtin backend requires --model <provider/model> or --provider-id <provider>"
  );
}

function parseProviderQualifiedModel(
  model: string
): { readonly providerId: string; readonly model: string } | undefined {
  const separatorIndex = model.indexOf("/");
  if (separatorIndex <= 0 || separatorIndex === model.length - 1) {
    return undefined;
  }
  return {
    providerId: model.slice(0, separatorIndex),
    model: model.slice(separatorIndex + 1)
  };
}

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
  const recoveryArgs =
    candidate === undefined
      ? retryForwardedArgs(options.forwardedArgs)
      : recoveryForwardedArgs(options.forwardedArgs, candidate.sessionPath, candidate.payload);
  if (recoveryArgs === undefined) {
    throw new Error("final_json was not called; no structured output was captured");
  }
  const recoveryPlan = await createLaunchPlan(
    {
      ...options,
      forwardedArgs: recoveryArgs
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
      `backend: ${options.backend}`,
      `base url: ${options.baseUrl}`,
      `model: ${resolved.model}`,
      `server model name: ${resolved.serverModelName ?? "unspecified"}`,
      `available models: ${availableModelsStatus(options, resolved)}`,
      `context window: ${String(options.contextWindow ?? resolved.contextWindow ?? "unspecified")}`,
      `provider id: ${options.providerId}`,
      `model source: ${modelSourceStatus(options)}`,
      `request params: ${requestParamsStatus(options)}`,
      `pi config dir: ${runtimeConfig.configDir}`,
      `model metadata: ${runtimeConfig.modelMetadataPath}`,
      `session dir: ${options.sessionDir}`,
      `pi command: ${options.piCommand}`
    ].join("\n") + "\n"
  );
}

function availableModelsStatus(options: LocalpagerAgentOptions, resolved: ResolvedModel): string {
  return options.backend === "pi-builtin"
    ? `${resolved.availableModels.join(", ")} (not probed; resolved by Pi)`
    : resolved.availableModels.join(", ");
}

function modelSourceStatus(options: LocalpagerAgentOptions): string {
  return options.backend === "pi-builtin"
    ? "Pi built-in registry"
    : "generated OpenAI-compatible models.json";
}

function requestParamsStatus(options: LocalpagerAgentOptions): string {
  return hasSamplingOptions(options.sampling)
    ? JSON.stringify(samplingRequestParams(options.sampling))
    : "unspecified";
}
