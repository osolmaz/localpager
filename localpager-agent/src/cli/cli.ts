import { errorMessage, fail, ok, type CommandResult } from "../common/result.js";
import { parseLocalpagerAgentArgs, usage } from "../agent/options.js";
import { resolveLocalModel } from "../llm/openai.js";
import { writeRuntimeConfig } from "../pi/config.js";
import type { RuntimeConfig } from "../pi/config.js";
import { createLaunchPlan, execLaunchPlan } from "../pi/launch.js";
import { createRepoReaderRuntime } from "../repo-reader/bash-extension.js";
import { createFinalSchemaRuntime, readFinalSchemaOutput } from "../structured/final-schema.js";

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

    const finalSchemaRuntime =
      options.finalSchemaPath === undefined
        ? undefined
        : await createFinalSchemaRuntime(options.finalSchemaPath, options.stateDir);
    const repoReaderRuntime = await createRepoReaderRuntime(options);
    const plan = await createLaunchPlan(
      options,
      runtimeConfig,
      resolved.model,
      finalSchemaRuntime,
      repoReaderRuntime
    );
    const code = await execLaunchPlan(plan);
    if (code !== 0 || plan.finalSchemaOutputPath === undefined) {
      return { code, stdout: "", stderr: "" };
    }
    return ok(await readFinalSchemaOutput(plan.finalSchemaOutputPath));
  } catch (error) {
    return fail(`localpager-agent: ${errorMessage(error)}`);
  }
}

type ResolvedModel = {
  readonly model: string;
  readonly availableModels: readonly string[];
  readonly contextWindow?: number;
};

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
      `pi config dir: ${runtimeConfig.configDir}`,
      `session dir: ${options.sessionDir}`,
      `pi command: ${options.piCommand}`
    ].join("\n") + "\n"
  );
}
