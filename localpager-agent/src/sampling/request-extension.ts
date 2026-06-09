import { mkdir, mkdtemp, writeFile } from "node:fs/promises";
import path from "node:path";

import type { SamplingOptions } from "./request-params.js";
import { hasSamplingOptions, samplingRequestParams } from "./request-params.js";

export type SamplingRuntime = {
  readonly extensionPath: string;
  readonly requestParams: Readonly<Record<string, number>>;
};

export async function createSamplingRuntime(
  options: SamplingOptions,
  stateDir: string
): Promise<SamplingRuntime | undefined> {
  if (!hasSamplingOptions(options)) {
    return undefined;
  }
  const runtimeDir = await createRuntimeDir(stateDir);
  const extensionPath = path.join(runtimeDir, "request-params-extension.ts");
  const requestParams = samplingRequestParams(options);
  await writeFile(extensionPath, extensionSource(requestParams), "utf8");
  return { extensionPath, requestParams };
}

async function createRuntimeDir(stateDir: string): Promise<string> {
  const root = path.join(stateDir, "request-params");
  await mkdir(root, { recursive: true });
  return await mkdtemp(path.join(root, "run-"));
}

function extensionSource(requestParams: Readonly<Record<string, number>>): string {
  return [
    'import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";',
    "",
    `const requestParams = ${JSON.stringify(requestParams, null, 2)} as const;`,
    "",
    "export default function localpagerAgentRequestParamsExtension(pi: ExtensionAPI): void {",
    '  pi.on("before_provider_request", (event) => {',
    "    if (!isRecord(event.payload)) {",
    "      return event.payload;",
    "    }",
    "    return { ...event.payload, ...requestParams };",
    "  });",
    "}",
    "",
    "function isRecord(value: unknown): value is Record<string, unknown> {",
    '  return typeof value === "object" && value !== null && !Array.isArray(value);',
    "}",
    ""
  ].join("\n");
}
