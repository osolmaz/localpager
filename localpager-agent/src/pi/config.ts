import { mkdir, rm, writeFile } from "node:fs/promises";
import path from "node:path";

import type { LocalpagerAgentOptions } from "../agent/options.js";

export type RuntimeConfig = {
  readonly configDir: string;
  readonly modelsPath: string;
  readonly settingsPath: string;
};

export async function writeRuntimeConfig(
  options: LocalpagerAgentOptions,
  model: string,
  discoveredContextWindow?: number
): Promise<RuntimeConfig> {
  const configDir = path.join(options.stateDir, "pi-config-runtime");
  await mkdir(configDir, { recursive: true });
  const modelsPath = path.join(configDir, "models.json");
  const settingsPath = path.join(configDir, "settings.json");
  if (options.backend === "openai-compatible") {
    await writeFile(
      modelsPath,
      `${JSON.stringify(modelsConfig(options, model, discoveredContextWindow), null, 2)}\n`
    );
  } else {
    await rm(modelsPath, { force: true });
  }
  await writeFile(
    settingsPath,
    `${JSON.stringify(settingsConfig(options, model, discoveredContextWindow), null, 2)}\n`
  );
  return { configDir, modelsPath, settingsPath };
}

function modelsConfig(
  options: LocalpagerAgentOptions,
  model: string,
  discoveredContextWindow?: number
): unknown {
  const contextWindow = options.contextWindow ?? discoveredContextWindow;
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
            name: `Local model (${model})`,
            reasoning: options.thinking !== "off",
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

function withoutUndefined(value: Record<string, unknown>): Record<string, unknown> {
  return Object.fromEntries(
    Object.entries(value).filter(([, entryValue]) => entryValue !== undefined)
  );
}

function settingsConfig(
  options: LocalpagerAgentOptions,
  model: string,
  discoveredContextWindow?: number
): unknown {
  const contextWindow = options.contextWindow ?? discoveredContextWindow;
  return {
    defaultProvider: options.providerId,
    defaultModel: model,
    defaultThinkingLevel: options.thinking,
    enableInstallTelemetry: false,
    quietStartup: true,
    compaction: compactionConfig(contextWindow)
  };
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
