import { mkdir, rm, writeFile } from "node:fs/promises";
import path from "node:path";

import type { LocalpagerAgentOptions } from "../agent/options.js";

export type RuntimeConfig = {
  readonly configDir: string;
  readonly modelsPath: string;
  readonly settingsPath: string;
  readonly modelMetadataPath: string;
};

export type RuntimeModelMetadata = {
  readonly requestedModel?: string;
  readonly availableModels?: readonly string[];
  readonly contextWindow?: number;
  readonly serverModelName?: string;
};

export async function writeRuntimeConfig(
  options: LocalpagerAgentOptions,
  model: string,
  metadataOrContextWindow?: RuntimeModelMetadata | number
): Promise<RuntimeConfig> {
  const metadata = normalizeRuntimeModelMetadata(metadataOrContextWindow);
  const configDir = path.join(options.stateDir, "pi-config-runtime");
  await mkdir(configDir, { recursive: true });
  const modelsPath = path.join(configDir, "models.json");
  const settingsPath = path.join(configDir, "settings.json");
  const modelMetadataPath = path.join(configDir, "model-metadata.json");
  if (options.backend === "openai-compatible") {
    await writeFile(
      modelsPath,
      `${JSON.stringify(modelsConfig(options, model, metadata), null, 2)}\n`
    );
  } else {
    await rm(modelsPath, { force: true });
  }
  await writeFile(
    settingsPath,
    `${JSON.stringify(settingsConfig(options, model, metadata), null, 2)}\n`
  );
  await writeFile(
    modelMetadataPath,
    `${JSON.stringify(modelMetadataConfig(options, model, metadata), null, 2)}\n`
  );
  return { configDir, modelsPath, settingsPath, modelMetadataPath };
}

function modelsConfig(
  options: LocalpagerAgentOptions,
  model: string,
  metadata: RuntimeModelMetadata
): unknown {
  const contextWindow = options.contextWindow ?? metadata.contextWindow;
  const compat = modelCompat(model);
  return {
    providers: {
      [options.providerId]: {
        baseUrl: options.baseUrl,
        api: "openai-completions",
        apiKey: "local",
        compat: {
          supportsDeveloperRole: false,
          supportsReasoningEffort: false
        },
        models: [
          withoutUndefined({
            id: model,
            name: modelDisplayName(model, metadata.serverModelName),
            reasoning: options.thinking !== "off" || compat !== undefined,
            compat,
            input: ["text"],
            contextWindow,
            maxTokens: options.maxTokens,
            cost: {
              input: 0,
              output: 0,
              cacheRead: 0,
              cacheWrite: 0
            }
          })
        ]
      }
    }
  };
}

type ThinkingFormat = "deepseek" | "qwen-chat-template";

function modelCompat(model: string): { readonly thinkingFormat: ThinkingFormat } | undefined {
  const thinkingFormat = thinkingFormatForModel(model);
  return thinkingFormat === undefined ? undefined : { thinkingFormat };
}

function thinkingFormatForModel(model: string): ThinkingFormat | undefined {
  const normalized = model.toLowerCase();
  if (normalized.includes("qwen")) {
    return "qwen-chat-template";
  }
  if (normalized.includes("deepseek")) {
    return "deepseek";
  }
  if (normalized.includes("gemma-12b") || normalized.includes("gemma-4-12b")) {
    return "qwen-chat-template";
  }
  return undefined;
}

function withoutUndefined(value: Record<string, unknown>): Record<string, unknown> {
  return Object.fromEntries(
    Object.entries(value).filter(([, entryValue]) => entryValue !== undefined)
  );
}

function settingsConfig(
  options: LocalpagerAgentOptions,
  model: string,
  metadata: RuntimeModelMetadata
): unknown {
  const contextWindow = options.contextWindow ?? metadata.contextWindow;
  return {
    defaultProvider: options.providerId,
    defaultModel: model,
    defaultThinkingLevel: options.thinking,
    enableInstallTelemetry: false,
    quietStartup: true,
    compaction: compactionConfig(contextWindow)
  };
}

function modelMetadataConfig(
  options: LocalpagerAgentOptions,
  model: string,
  metadata: RuntimeModelMetadata
): unknown {
  return withoutUndefined({
    backend: options.backend,
    baseUrl: options.backend === "openai-compatible" ? options.baseUrl : undefined,
    requestedModel: metadata.requestedModel ?? options.model,
    resolvedModel: model,
    serverModelName: metadata.serverModelName,
    availableModels: metadata.availableModels,
    contextWindow: options.contextWindow ?? metadata.contextWindow,
    maxTokens: options.maxTokens,
    thinking: options.thinking
  });
}

function normalizeRuntimeModelMetadata(
  value: RuntimeModelMetadata | number | undefined
): RuntimeModelMetadata {
  if (value === undefined) {
    return {};
  }
  return typeof value === "number" ? { contextWindow: value } : value;
}

function modelDisplayName(model: string, serverModelName: string | undefined): string {
  return serverModelName === undefined || serverModelName.trim() === ""
    ? `Local model (${model})`
    : `${serverModelName} (${model})`;
}

function compactionConfig(contextWindow: number | undefined): unknown {
  if (contextWindow === undefined) {
    return { enabled: false };
  }
  return {
    enabled: true,
    reserveTokens: Math.max(256, Math.min(16384, Math.floor(contextWindow / 4))),
    keepRecentTokens: Math.max(512, Math.min(20000, Math.floor(contextWindow / 2)))
  };
}
