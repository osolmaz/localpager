import { access, mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

import { describe, expect, it } from "vitest";

import type { LocalpagerAgentOptions } from "../src/agent/options.js";
import { writeRuntimeConfig } from "../src/pi/config.js";

describe("Pi runtime config", () => {
  it("writes a local OpenAI-compatible provider config", async () => {
    const stateDir = await mkdtemp(path.join(os.tmpdir(), "localpager-agent-test-"));
    try {
      const runtime = await writeRuntimeConfig(options(stateDir), "gemma-4-e4b-it");
      const models = JSON.parse(await readFile(runtime.modelsPath, "utf8")) as {
        providers: Record<string, { baseUrl: string; models: readonly { id: string }[] }>;
      };
      expect(models.providers["local-openai"]?.baseUrl).toBe("http://127.0.0.1:1234/v1");
      expect(models.providers["local-openai"]?.models[0]?.id).toBe("gemma-4-e4b-it");
      expect(models.providers["local-openai"]?.models[0]).not.toHaveProperty("contextWindow");
      const settings = JSON.parse(await readFile(runtime.settingsPath, "utf8")) as {
        compaction?: { enabled?: boolean };
      };
      expect(settings.compaction?.enabled).toBe(false);
    } finally {
      await rm(stateDir, { recursive: true, force: true });
    }
  });

  it("writes context window only from an override or discovered metadata", async () => {
    const stateDir = await mkdtemp(path.join(os.tmpdir(), "localpager-agent-test-"));
    try {
      const runtime = await writeRuntimeConfig(options(stateDir), "gemma-4-e4b-it", 120000);
      const models = JSON.parse(await readFile(runtime.modelsPath, "utf8")) as {
        providers: Record<string, { models: readonly { contextWindow?: number }[] }>;
      };
      expect(models.providers["local-openai"]?.models[0]?.contextWindow).toBe(120000);
      const settings = JSON.parse(await readFile(runtime.settingsPath, "utf8")) as {
        compaction?: { enabled?: boolean; reserveTokens?: number; keepRecentTokens?: number };
      };
      expect(settings.compaction).toEqual({
        enabled: true,
        reserveTokens: 16384,
        keepRecentTokens: 20000
      });
    } finally {
      await rm(stateDir, { recursive: true, force: true });
    }
  });

  it("marks local models as reasoning-capable when Pi thinking is enabled", async () => {
    const stateDir = await mkdtemp(path.join(os.tmpdir(), "localpager-agent-test-"));
    try {
      const runtime = await writeRuntimeConfig(
        { ...options(stateDir), thinking: "high" },
        "gemma-12b-q4km-reason"
      );
      const models = JSON.parse(await readFile(runtime.modelsPath, "utf8")) as {
        providers: Record<
          string,
          { models: readonly { reasoning?: boolean; compat?: { thinkingFormat?: string } }[] }
        >;
      };
      expect(models.providers["local-openai"]?.models[0]?.reasoning).toBe(true);
      expect(models.providers["local-openai"]?.models[0]?.compat).toEqual({
        thinkingFormat: "qwen-chat-template"
      });
    } finally {
      await rm(stateDir, { recursive: true, force: true });
    }
  });

  it("keeps known reasoning model metadata when thinking is disabled", async () => {
    const stateDir = await mkdtemp(path.join(os.tmpdir(), "localpager-agent-test-"));
    try {
      const runtime = await writeRuntimeConfig(options(stateDir), "gemma-4-12b-it");
      const models = JSON.parse(await readFile(runtime.modelsPath, "utf8")) as {
        providers: Record<
          string,
          { models: readonly { reasoning?: boolean; compat?: { thinkingFormat?: string } }[] }
        >;
      };
      expect(models.providers["local-openai"]?.models[0]).toMatchObject({
        reasoning: true,
        compat: { thinkingFormat: "qwen-chat-template" }
      });
    } finally {
      await rm(stateDir, { recursive: true, force: true });
    }
  });

  it("maps local Qwen and DeepSeek model ids to Pi thinking formats", async () => {
    const stateDir = await mkdtemp(path.join(os.tmpdir(), "localpager-agent-test-"));
    try {
      const qwenRuntime = await writeRuntimeConfig(options(stateDir), "qwen3.6-35b-a3b-mtp");
      const qwenModels = JSON.parse(await readFile(qwenRuntime.modelsPath, "utf8")) as {
        providers: Record<string, { models: readonly { compat?: { thinkingFormat?: string } }[] }>;
      };
      const deepseekRuntime = await writeRuntimeConfig(options(stateDir), "deepseek-v4-pro");
      const deepseekModels = JSON.parse(await readFile(deepseekRuntime.modelsPath, "utf8")) as {
        providers: Record<string, { models: readonly { compat?: { thinkingFormat?: string } }[] }>;
      };
      expect(qwenModels.providers["local-openai"]?.models[0]?.compat).toEqual({
        thinkingFormat: "qwen-chat-template"
      });
      expect(deepseekModels.providers["local-openai"]?.models[0]?.compat).toEqual({
        thinkingFormat: "deepseek"
      });
    } finally {
      await rm(stateDir, { recursive: true, force: true });
    }
  });

  it("scales Pi compaction settings below small local context windows", async () => {
    const stateDir = await mkdtemp(path.join(os.tmpdir(), "localpager-agent-test-"));
    try {
      const runtime = await writeRuntimeConfig(
        { ...options(stateDir), contextWindow: 4096 },
        "gemma-4-e4b-it"
      );
      const settings = JSON.parse(await readFile(runtime.settingsPath, "utf8")) as {
        compaction?: { enabled?: boolean; reserveTokens?: number; keepRecentTokens?: number };
      };
      expect(settings.compaction).toEqual({
        enabled: true,
        reserveTokens: 1024,
        keepRecentTokens: 2048
      });
    } finally {
      await rm(stateDir, { recursive: true, force: true });
    }
  });

  it("uses Pi built-in registry without generated provider config", async () => {
    const stateDir = await mkdtemp(path.join(os.tmpdir(), "localpager-agent-test-"));
    try {
      const staleModelsPath = path.join(stateDir, "pi-config-runtime", "models.json");
      await mkdir(path.dirname(staleModelsPath), { recursive: true });
      await writeFile(staleModelsPath, JSON.stringify({ providers: { stale: {} } }), "utf8");
      const runtime = await writeRuntimeConfig(
        { ...options(stateDir), backend: "pi-builtin", providerId: "openai-codex" },
        "gpt-5.3-codex-spark",
        272000
      );

      await expect(access(runtime.modelsPath)).rejects.toThrow();
      const settings = JSON.parse(await readFile(runtime.settingsPath, "utf8")) as {
        defaultProvider?: string;
        defaultModel?: string;
        compaction?: { enabled?: boolean };
      };
      expect(settings.defaultProvider).toBe("openai-codex");
      expect(settings.defaultModel).toBe("gpt-5.3-codex-spark");
      expect(settings.compaction?.enabled).toBe(true);
    } finally {
      await rm(stateDir, { recursive: true, force: true });
    }
  });
});

function options(stateDir: string): LocalpagerAgentOptions {
  return {
    backend: "openai-compatible",
    baseUrl: "http://127.0.0.1:1234/v1",
    model: "auto",
    providerId: "local-openai",
    stateDir,
    sessionDir: path.join(stateDir, "sessions"),
    piCommand: "pi",
    thinking: "off",
    contextWindow: undefined,
    maxTokens: 8192,
    sampling: {},
    timeoutMs: 1000,
    finalSchemaPath: undefined,
    finalSchemaInstruction: true,
    promptTemplatePath: undefined,
    promptVarsPaths: [],
    promptVars: [],
    renderedPromptPath: undefined,
    reposhellSocket: undefined,
    reposhellDefaultRepo: undefined,
    reposhellVisibleRepos: [],
    status: false,
    forwardedArgs: []
  };
}
