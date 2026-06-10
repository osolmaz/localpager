import { createServer } from "node:http";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

import { describe, expect, it } from "vitest";

import { run } from "../src/cli/cli.js";
import { promptForwardedArgs, renderPromptTemplate } from "../src/prompts/template.js";

describe("prompt templates", () => {
  it("renders variables, JSON values, and optional blocks", () => {
    const rendered = renderPromptTemplate(
      [
        "Title: {{title}}",
        "{{#if body}}",
        "Body:",
        "{{{body}}}",
        "{{/if}}",
        "Topics:",
        "{{{topics}}}"
      ].join("\n"),
      {
        title: "LM Studio streaming",
        body: "Local model stream failed.",
        topics: ["local_models", "model_serving"]
      }
    );

    expect(rendered).toBe(
      [
        "Title: LM Studio streaming",
        "",
        "Body:",
        "Local model stream failed.",
        "",
        "Topics:",
        JSON.stringify(["local_models", "model_serving"], null, 2)
      ].join("\n")
    );
  });

  it("rejects missing required variables", () => {
    expect(() => renderPromptTemplate("Title: {{title}}", {})).toThrow(
      "missing prompt template variable: title"
    );
  });

  it("renders prompt files into Pi print args", async () => {
    const stateDir = await mkdtemp(path.join(os.tmpdir(), "localpager-agent-prompt-"));
    try {
      const templatePath = path.join(stateDir, "classifier.hbs");
      const varsPath = path.join(stateDir, "vars.json");
      const renderedPath = path.join(stateDir, "rendered.prompt.txt");
      await writeFile(templatePath, "Title: {{title}}\nBody:\n{{{body}}}\n", "utf8");
      await writeFile(
        varsPath,
        JSON.stringify({ title: "from file", body: "from file body" }),
        "utf8"
      );

      const args = await promptForwardedArgs({
        promptTemplatePath: templatePath,
        promptVarsPaths: [varsPath],
        promptVars: ["title=from inline"],
        renderedPromptPath: renderedPath,
        forwardedArgs: ["--tools", "none"]
      });

      expect(args).toEqual([
        "--tools",
        "none",
        "-p",
        "Title: from inline\nBody:\nfrom file body\n"
      ]);
      await expect(readFile(renderedPath, "utf8")).resolves.toBe(
        "Title: from inline\nBody:\nfrom file body\n"
      );
    } finally {
      await rm(stateDir, { recursive: true, force: true });
    }
  });

  it("rejects prompt template combined with an existing Pi print prompt", async () => {
    await expect(
      promptForwardedArgs({
        promptTemplatePath: "prompt.hbs",
        promptVarsPaths: [],
        promptVars: [],
        renderedPromptPath: undefined,
        forwardedArgs: ["-p", "already set"]
      })
    ).rejects.toThrow("--prompt-template cannot be combined with Pi -p/--print");
  });

  it("passes rendered templates to Pi through the CLI", async () => {
    const stateDir = await mkdtemp(path.join(os.tmpdir(), "localpager-agent-prompt-cli-"));
    const server = createModelServer();
    try {
      const port = await listen(server);
      const templatePath = path.join(stateDir, "classifier.hbs");
      const varsPath = path.join(stateDir, "vars.json");
      const argsPath = path.join(stateDir, "pi-args.json");
      const fakePiPath = path.join(stateDir, "fake-pi.mjs");
      await writeFile(templatePath, "Classify: {{title}}\n{{{body}}}", "utf8");
      await writeFile(varsPath, JSON.stringify({ title: "file title", body: "file body" }), "utf8");
      await writeFile(fakePiPath, fakePiSource(argsPath), "utf8");

      const result = await run([
        "--base-url",
        `http://127.0.0.1:${String(port)}/v1`,
        "--state-dir",
        stateDir,
        "--pi-command",
        `node ${fakePiPath}`,
        "--prompt-template",
        templatePath,
        "--prompt-vars-file",
        varsPath,
        "--prompt-var",
        "title=inline title",
        "--some-pi-flag"
      ]);

      expect(result.code).toBe(0);
      const piArgs = JSON.parse(await readFile(argsPath, "utf8")) as string[];
      const promptIndex = piArgs.indexOf("-p");
      expect(promptIndex).toBeGreaterThan(-1);
      expect(piArgs[promptIndex + 1]).toBe("Classify: inline title\nfile body");
      expect(piArgs).toContain("--some-pi-flag");
    } finally {
      await close(server);
      await rm(stateDir, { recursive: true, force: true });
    }
  });

  it("launches Pi built-in providers without a local model probe", async () => {
    const stateDir = await mkdtemp(path.join(os.tmpdir(), "localpager-agent-pi-builtin-"));
    try {
      const argsPath = path.join(stateDir, "pi-args.json");
      const fakePiPath = path.join(stateDir, "fake-pi.mjs");
      await writeFile(fakePiPath, fakePiSource(argsPath), "utf8");

      const result = await run([
        "--backend",
        "pi-builtin",
        "--model",
        "openai-codex/gpt-5.3-codex-spark",
        "--state-dir",
        stateDir,
        "--pi-command",
        `node ${fakePiPath}`,
        "--thinking",
        "minimal",
        "-p",
        "classify"
      ]);

      expect(result.code).toBe(0);
      const piArgs = JSON.parse(await readFile(argsPath, "utf8")) as string[];
      expect(piArgs).toEqual([
        "--provider",
        "openai-codex",
        "--model",
        "gpt-5.3-codex-spark",
        "--thinking",
        "minimal",
        "-p",
        "classify"
      ]);
      await expect(
        readFile(path.join(stateDir, "pi-config-runtime", "models.json"), "utf8")
      ).rejects.toThrow();
    } finally {
      await rm(stateDir, { recursive: true, force: true });
    }
  });
});

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

function fakePiSource(argsPath: string): string {
  return [
    'import { writeFileSync } from "node:fs";',
    `writeFileSync(${JSON.stringify(argsPath)}, JSON.stringify(process.argv.slice(2)), "utf8");`
  ].join("\n");
}
