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
});
