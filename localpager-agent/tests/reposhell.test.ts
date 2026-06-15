import { mkdtemp, readFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

import { describe, expect, it } from "vitest";

import type { LocalpagerAgentOptions } from "../src/agent/options.js";
import { createReposhellRuntime } from "../src/reposhell/bash-extension.js";

describe("reposhell runtime", () => {
  it("tells models to use only one simple read-only command", async () => {
    const stateDir = await mkdtemp(path.join(os.tmpdir(), "localpager-agent-reposhell-"));
    try {
      const runtime = await createReposhellRuntime({
        ...options(stateDir),
        reposhellSocket: "/tmp/localpager-reposhell.sock",
        reposhellDefaultRepo: "openclaw",
        reposhellVisibleRepos: ["openclaw"]
      });
      const source = await readFile(runtime?.extensionPath ?? "", "utf8");

      expect(runtime?.instruction).toContain("Use one simple read-only command");
      expect(runtime?.instruction).toContain("do not use cd or absolute /home paths");
      expect(runtime?.instruction).toContain("Never use command chaining");
      expect(source).toContain("Run one simple read-only repository inspection command");
      expect(source).toContain("Never use cd, &&, ||, |, ;, redirects");
      expect(source).toContain("Use exactly one simple command");
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
