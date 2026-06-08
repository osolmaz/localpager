import { createServer } from "node:http";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

import { describe, expect, it } from "vitest";

import type { LocalpagerAgentOptions } from "../src/agent/options.js";
import { run } from "../src/cli/cli.js";
import type { RuntimeConfig } from "../src/pi/config.js";
import { createLaunchPlan } from "../src/pi/launch.js";
import { createFinalSchemaRuntime, readFinalSchemaOutput } from "../src/structured/final-schema.js";
import { extractFinalJsonPayload, recoveryForwardedArgs } from "../src/structured/recovery.js";

describe("structured output", () => {
  it("ships a context-agnostic example schema", async () => {
    const raw = await readFile(
      path.join(process.cwd(), "examples", "schemas", "binary-classifier.schema.json"),
      "utf8"
    );
    const parsed = JSON.parse(raw) as {
      type?: string;
      required?: string[];
      properties?: { label?: { enum?: string[] } };
    };

    expect(parsed.type).toBe("object");
    expect(parsed.required).toEqual([
      "is_match",
      "label",
      "confidence",
      "summary",
      "reasons",
      "caveats"
    ]);
    expect(parsed.properties?.label?.enum).toEqual([
      "match",
      "partial_match",
      "no_match",
      "unclear"
    ]);
  });

  it("creates a final_json extension from a schema", async () => {
    const stateDir = await mkdtemp(path.join(os.tmpdir(), "localpager-agent-structured-"));
    try {
      const schemaPath = path.join(stateDir, "schema.json");
      await writeFile(schemaPath, JSON.stringify(schema()), "utf8");
      const runtime = await createFinalSchemaRuntime(schemaPath, stateDir);
      const source = await readFile(runtime.extensionPath, "utf8");

      expect(runtime.outputPath).toMatch(/final-output\.json$/u);
      expect(runtime.instruction).toContain("call the final_json tool exactly once");
      expect(source).toContain('name: "final_json"');
      expect(source).toContain('"is_local_model_related"');
      expect(source).toContain('"interest"');
    } finally {
      await rm(stateDir, { recursive: true, force: true });
    }
  });

  it("prints captured structured output as pretty JSON", async () => {
    const stateDir = await mkdtemp(path.join(os.tmpdir(), "localpager-agent-structured-"));
    try {
      const outputPath = path.join(stateDir, "final-output.json");
      await writeFile(outputPath, '{"interest":"i0"}\n', "utf8");
      await expect(readFinalSchemaOutput(outputPath)).resolves.toBe('{\n  "interest": "i0"\n}\n');
    } finally {
      await rm(stateDir, { recursive: true, force: true });
    }
  });

  it("extracts Gemma pseudo final_json text", () => {
    const payload = extractFinalJsonPayload(
      'call:final_json{caveats:[<|"|>limited context<|"|>],topics_of_interest:[<|"|>local_model_providers<|"|>,<|"|>local_models<|"|>],description:<|"|>LM Studio setup touches optional API key and context length.<|"|>}<tool_call|>'
    );

    expect(payload).toEqual({
      caveats: ["limited context"],
      topics_of_interest: ["local_model_providers", "local_models"],
      description: "LM Studio setup touches optional API key and context length."
    });
  });

  it("builds recovery args for the same session", () => {
    const args = recoveryForwardedArgs(
      ["--tools", "bash", "-p", "classify this", "--session", "/tmp/old.jsonl"],
      "/tmp/current.jsonl",
      { topics_of_interest: ["local_models"], description: "done", caveats: [] }
    );

    expect(args[0]).toBe("--tools");
    expect(args).toContain("--session");
    expect(args).toContain("/tmp/current.jsonl");
    expect(args).not.toContain("/tmp/old.jsonl");
    expect(args).not.toContain("classify this");
  });

  it("recovers pseudo final_json output in the same Pi session", async () => {
    const stateDir = await mkdtemp(path.join(os.tmpdir(), "localpager-agent-recovery-"));
    const server = createModelServer();
    try {
      const port = await listen(server);
      const scriptPath = path.join(stateDir, "fake-pi.mjs");
      const schemaPath = path.join(stateDir, "schema.json");
      const sessionDir = path.join(stateDir, "sessions");
      await writeFile(schemaPath, JSON.stringify(classificationSchema()), "utf8");
      await writeFile(scriptPath, fakePiSource(), "utf8");

      const result = await run([
        "--base-url",
        `http://127.0.0.1:${String(port)}/v1`,
        "--state-dir",
        stateDir,
        "--session-dir",
        sessionDir,
        "--pi-command",
        `node ${scriptPath}`,
        "--final-schema",
        schemaPath,
        "-p",
        "classify"
      ]);

      expect(result.code).toBe(0);
      expect(JSON.parse(result.stdout)).toEqual({
        caveats: [],
        topics_of_interest: ["local_model_providers", "local_models"],
        description: "Recovered in the same session."
      });
      const session = await readFile(path.join(sessionDir, "fake-session.jsonl"), "utf8");
      expect(session).toContain("call:final_json");
      expect(session).toContain('"name":"final_json"');
    } finally {
      await close(server);
      await rm(stateDir, { recursive: true, force: true });
    }
  });

  it("adds the generated extension and final_json allowlist to Pi args", async () => {
    const runtime = {
      extensionPath: "/tmp/final-json-extension.ts",
      outputPath: "/tmp/final-output.json",
      instruction: "call final_json"
    };
    const reposhellRuntime = {
      extensionPath: "/tmp/reposhell-bash-extension.ts",
      instruction: "use bash only if needed"
    };
    const plan = await createLaunchPlan(
      {
        ...options("/tmp/localpager-agent-state"),
        forwardedArgs: ["--tools", "bash", "-p", "classify"]
      },
      runtimeConfig("/tmp/localpager-agent-state"),
      "gemma-4-e4b-it",
      runtime,
      reposhellRuntime
    );

    expect(plan.finalSchemaOutputPath).toBe("/tmp/final-output.json");
    expect(plan.args).toEqual([
      "--provider",
      "local-openai",
      "--model",
      "gemma-4-e4b-it",
      "--thinking",
      "off",
      "--extension",
      "/tmp/reposhell-bash-extension.ts",
      "--append-system-prompt",
      "use bash only if needed",
      "--extension",
      "/tmp/final-json-extension.ts",
      "--append-system-prompt",
      "call final_json",
      "--tools",
      "bash,final_json",
      "-p",
      "classify"
    ]);
  });

  it("adds explicit final_json tools when no allowlist is provided", async () => {
    const plan = await createLaunchPlan(
      { ...options("/tmp/localpager-agent-state"), forwardedArgs: ["-p", "classify"] },
      runtimeConfig("/tmp/localpager-agent-state"),
      "gemma-4-e4b-it",
      {
        extensionPath: "/tmp/final-json-extension.ts",
        outputPath: "/tmp/final-output.json",
        instruction: "call final_json"
      }
    );

    expect(plan.args).toContain("--tools");
    expect(plan.args).toContain("final_json");
  });

  it("adds reposhell bash extension without final schema", async () => {
    const plan = await createLaunchPlan(
      { ...options("/tmp/localpager-agent-state"), forwardedArgs: ["-p", "inspect"] },
      runtimeConfig("/tmp/localpager-agent-state"),
      "gemma-4-e4b-it",
      undefined,
      {
        extensionPath: "/tmp/reposhell-bash-extension.ts",
        instruction: "use bash only if needed"
      }
    );

    expect(plan.finalSchemaOutputPath).toBeUndefined();
    expect(plan.args).toEqual([
      "--provider",
      "local-openai",
      "--model",
      "gemma-4-e4b-it",
      "--thinking",
      "off",
      "--extension",
      "/tmp/reposhell-bash-extension.ts",
      "--append-system-prompt",
      "use bash only if needed",
      "--tools",
      "bash",
      "-p",
      "inspect"
    ]);
  });

  it("rejects bash allowlist without reposhell extension", async () => {
    await expect(
      createLaunchPlan(
        {
          ...options("/tmp/localpager-agent-state"),
          forwardedArgs: ["--tools", "bash", "-p", "classify"]
        },
        runtimeConfig("/tmp/localpager-agent-state"),
        "gemma-4-e4b-it",
        {
          extensionPath: "/tmp/final-json-extension.ts",
          outputPath: "/tmp/final-output.json",
          instruction: "call final_json"
        }
      )
    ).rejects.toThrow("--tools bash requires --reposhell-socket");
  });

  it("rejects duplicate tool allowlist flags", async () => {
    await expect(
      createLaunchPlan(
        {
          ...options("/tmp/localpager-agent-state"),
          forwardedArgs: ["--tools", "final_json", "-t", "bash", "-p", "classify"]
        },
        runtimeConfig("/tmp/localpager-agent-state"),
        "gemma-4-e4b-it",
        {
          extensionPath: "/tmp/final-json-extension.ts",
          outputPath: "/tmp/final-output.json",
          instruction: "call final_json"
        }
      )
    ).rejects.toThrow("duplicate --tools flags");
  });

  it("rejects schema mode when Pi tools are disabled", async () => {
    await expect(
      createLaunchPlan(
        { ...options("/tmp/localpager-agent-state"), forwardedArgs: ["--no-tools"] },
        runtimeConfig("/tmp/localpager-agent-state"),
        "gemma-4-e4b-it",
        {
          extensionPath: "/tmp/final-json-extension.ts",
          outputPath: "/tmp/final-output.json",
          instruction: "call final_json"
        }
      )
    ).rejects.toThrow("--final-schema cannot be used with --no-tools");

    await expect(
      createLaunchPlan(
        { ...options("/tmp/localpager-agent-state"), forwardedArgs: ["-nt"] },
        runtimeConfig("/tmp/localpager-agent-state"),
        "gemma-4-e4b-it",
        {
          extensionPath: "/tmp/final-json-extension.ts",
          outputPath: "/tmp/final-output.json",
          instruction: "call final_json"
        }
      )
    ).rejects.toThrow("--final-schema cannot be used with --no-tools");
  });

  it("rejects schema mode outside Pi print mode", async () => {
    await expect(
      createLaunchPlan(
        { ...options("/tmp/localpager-agent-state"), forwardedArgs: ["classify"] },
        runtimeConfig("/tmp/localpager-agent-state"),
        "gemma-4-e4b-it",
        {
          extensionPath: "/tmp/final-json-extension.ts",
          outputPath: "/tmp/final-output.json",
          instruction: "call final_json"
        }
      )
    ).rejects.toThrow("--final-schema requires Pi print mode");
  });
});

function schema(): unknown {
  return {
    type: "object",
    additionalProperties: false,
    required: ["is_local_model_related", "interest"],
    properties: {
      is_local_model_related: { type: "boolean" },
      interest: { type: "string", enum: ["i0", "i1", "i2", "i3", "i4"] }
    }
  };
}

function classificationSchema(): unknown {
  return {
    type: "object",
    additionalProperties: false,
    required: ["topics_of_interest", "description", "caveats"],
    properties: {
      topics_of_interest: { type: "array", items: { type: "string" } },
      description: { type: "string" },
      caveats: { type: "array", items: { type: "string" } }
    }
  };
}

function options(stateDir: string): LocalpagerAgentOptions {
  return {
    baseUrl: "http://127.0.0.1:1234/v1",
    model: "auto",
    providerId: "local-openai",
    stateDir,
    sessionDir: path.join(stateDir, "sessions"),
    piCommand: "pi",
    thinking: "off",
    contextWindow: undefined,
    maxTokens: 8192,
    timeoutMs: 1000,
    finalSchemaPath: undefined,
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

function runtimeConfig(stateDir: string): RuntimeConfig {
  return {
    configDir: path.join(stateDir, "pi"),
    modelsPath: path.join(stateDir, "pi", "models.json"),
    settingsPath: path.join(stateDir, "pi", "settings.json")
  };
}

function createModelServer(): ReturnType<typeof createServer> {
  return createServer((request, response) => {
    if (request.url === "/v1/models") {
      response.writeHead(200, { "content-type": "application/json" });
      response.end(JSON.stringify({ data: [{ id: "gemma-4-e4b-it" }] }));
      return;
    }
    response.writeHead(404);
    response.end();
  });
}

async function listen(server: ReturnType<typeof createServer>): Promise<number> {
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  if (address === null || typeof address === "string") {
    throw new Error("test server did not bind to a TCP port");
  }
  return address.port;
}

async function close(server: ReturnType<typeof createServer>): Promise<void> {
  await new Promise<void>((resolve, reject) => {
    server.close((error) => {
      if (error === undefined) {
        resolve();
        return;
      }
      reject(error);
    });
  });
}

function fakePiSource(): string {
  return String.raw`
import { mkdirSync, readFileSync, writeFileSync, appendFileSync } from "node:fs";
import path from "node:path";

const args = process.argv.slice(2);
const sessionDir = process.env.PI_CODING_AGENT_SESSION_DIR;
if (sessionDir === undefined) {
  throw new Error("PI_CODING_AGENT_SESSION_DIR is required");
}
mkdirSync(sessionDir, { recursive: true });

const sessionArgIndex = args.indexOf("--session");
const sessionPath = sessionArgIndex < 0 ? path.join(sessionDir, "fake-session.jsonl") : args[sessionArgIndex + 1];
if (sessionPath === undefined) {
  throw new Error("--session requires a value");
}

if (sessionArgIndex < 0) {
  writeFileSync(sessionPath, JSON.stringify({ type: "session", version: 3 }) + "\n", "utf8");
  appendFileSync(
    sessionPath,
    JSON.stringify({
      type: "message",
      message: {
        role: "assistant",
        content: [
          {
            type: "text",
            text: "call:final_json{caveats:[],topics_of_interest:[<|\"|>local_model_providers<|\"|>,<|\"|>local_models<|\"|>],description:<|\"|>Recovered in the same session.<|\"|>}<tool_call|>"
          }
        ]
      }
    }) + "\n",
    "utf8"
  );
  process.exit(0);
}

const extensionIndex = args.indexOf("--extension");
const extensionPath = args[extensionIndex + 1];
const source = readFileSync(extensionPath, "utf8");
const outputPath = JSON.parse(source.match(/const outputPath = ("[^"]+");/)?.[1]);
const payload = {
  caveats: [],
  topics_of_interest: ["local_model_providers", "local_models"],
  description: "Recovered in the same session."
};
writeFileSync(outputPath, JSON.stringify(payload, null, 2) + "\n", "utf8");
appendFileSync(
  sessionPath,
  JSON.stringify({
    type: "message",
    message: {
      role: "assistant",
      content: [{ type: "toolCall", name: "final_json", arguments: payload }]
    }
  }) + "\n",
  "utf8"
);
`;
}
