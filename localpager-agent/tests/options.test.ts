import { describe, expect, it } from "vitest";

import { parseLocalpagerAgentArgs } from "../src/agent/options.js";

describe("localpager-agent option parsing", () => {
  it("keeps pi args as pass-through arguments", () => {
    const options = parseLocalpagerAgentArgs(["--model", "gemma-4-e4b-it", "-p", "write a plan"]);
    expect(options.model).toBe("gemma-4-e4b-it");
    expect(options.forwardedArgs).toEqual(["-p", "write a plan"]);
  });

  it("uses -- to forward pi flags that localpager-agent also owns", () => {
    const options = parseLocalpagerAgentArgs([
      "--model",
      "gemma-4-e4b-it",
      "--",
      "--model",
      "ignored-by-wrapper"
    ]);
    expect(options.model).toBe("gemma-4-e4b-it");
    expect(options.forwardedArgs).toEqual(["--model", "ignored-by-wrapper"]);
  });

  it("parses final schema flags as localpager-agent options", () => {
    expect(parseLocalpagerAgentArgs(["--final-schema", "schema.json"]).finalSchemaPath).toBe(
      "schema.json"
    );
    expect(parseLocalpagerAgentArgs(["--schema", "schema.json"]).finalSchemaPath).toBe(
      "schema.json"
    );
  });

  it("can disable the appended final schema instruction", () => {
    const options = parseLocalpagerAgentArgs(["--no-final-schema-instruction", "-p", "classify"]);

    expect(options.finalSchemaInstruction).toBe(false);
    expect(options.forwardedArgs).toEqual(["-p", "classify"]);
  });

  it("parses prompt template flags as localpager-agent options", () => {
    const options = parseLocalpagerAgentArgs([
      "--prompt-template",
      "prompts/classifier.hbs",
      "--prompt-vars-file",
      "base-vars.json",
      "--prompt-vars-file",
      "row-vars.json",
      "--prompt-var",
      "title=Fix LM Studio streaming",
      "--write-rendered-prompt",
      "/tmp/rendered.prompt.txt"
    ]);

    expect(options.promptTemplatePath).toBe("prompts/classifier.hbs");
    expect(options.promptVarsPaths).toEqual(["base-vars.json", "row-vars.json"]);
    expect(options.promptVars).toEqual(["title=Fix LM Studio streaming"]);
    expect(options.renderedPromptPath).toBe("/tmp/rendered.prompt.txt");
  });

  it("parses reposhell flags as localpager-agent options", () => {
    const options = parseLocalpagerAgentArgs([
      "--reposhell-socket",
      "/tmp/localpager.sock",
      "--reposhell-default-repo",
      "openclaw",
      "--reposhell-visible-repos",
      "openclaw,clawhub",
      "-p",
      "classify"
    ]);
    expect(options.reposhellSocket).toBe("/tmp/localpager.sock");
    expect(options.reposhellDefaultRepo).toBe("openclaw");
    expect(options.reposhellVisibleRepos).toEqual(["openclaw", "clawhub"]);
    expect(options.forwardedArgs).toEqual(["-p", "classify"]);
  });

  it("parses OpenAI-compatible sampling flags as localpager-agent options", () => {
    const options = parseLocalpagerAgentArgs([
      "--temperature",
      "0",
      "--top-p",
      "1",
      "--seed",
      "1234",
      "--presence-penalty",
      "0",
      "--frequency-penalty",
      "0",
      "-p",
      "classify"
    ]);

    expect(options.sampling).toEqual({
      temperature: 0,
      topP: 1,
      seed: 1234,
      presencePenalty: 0,
      frequencyPenalty: 0
    });
    expect(options.forwardedArgs).toEqual(["-p", "classify"]);
  });

  it("rejects invalid sampling values before launching Pi", () => {
    expect(() => parseLocalpagerAgentArgs(["--temperature", "3"])).toThrow(
      "--temperature must be a number between 0 and 2"
    );
    expect(() => parseLocalpagerAgentArgs(["--top-p", "2"])).toThrow(
      "--top-p must be a number between 0 and 1"
    );
    expect(() => parseLocalpagerAgentArgs(["--seed", "1.5"])).toThrow(
      "--seed must be a non-negative integer"
    );
    expect(() => parseLocalpagerAgentArgs(["--presence-penalty", "3"])).toThrow(
      "--presence-penalty must be a number between -2 and 2"
    );
    expect(() => parseLocalpagerAgentArgs(["--frequency-penalty", "-3"])).toThrow(
      "--frequency-penalty must be a number between -2 and 2"
    );
  });
});
