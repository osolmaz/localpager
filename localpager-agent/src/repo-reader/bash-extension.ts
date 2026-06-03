import { mkdir, mkdtemp, writeFile } from "node:fs/promises";
import path from "node:path";

import type { LocalpagerAgentOptions } from "../agent/options.js";

export type RepoReaderRuntime = {
  readonly extensionPath: string;
  readonly instruction: string;
};

export async function createRepoReaderRuntime(
  options: LocalpagerAgentOptions
): Promise<RepoReaderRuntime | undefined> {
  if (options.repoReaderSocket === undefined) {
    return undefined;
  }
  if (options.repoReaderDefaultRepo === undefined) {
    throw new Error("--repo-reader-default-repo is required when --repo-reader-socket is set");
  }
  const runtimeDir = await createRuntimeDir(options.stateDir);
  const extensionPath = path.join(runtimeDir, "repo-reader-bash-extension.ts");
  await writeFile(
    extensionPath,
    extensionSource(
      options.repoReaderSocket,
      options.repoReaderDefaultRepo,
      options.repoReaderVisibleRepos
    ),
    "utf8"
  );
  return {
    extensionPath,
    instruction: [
      "A read-only bash tool is available for inspecting configured repository snapshots.",
      "Use bash only when the GitHub context is ambiguous or repo files are needed to classify correctly.",
      "Prefer concise commands such as pwd, ls, find, rg, grep, sed -n, cat, head, git show --name-only, and git grep.",
      "Do not use bash for routine classification when the provided GitHub context is sufficient."
    ].join("\n")
  };
}

async function createRuntimeDir(stateDir: string): Promise<string> {
  const root = path.join(stateDir, "repo-reader");
  await mkdir(root, { recursive: true });
  return await mkdtemp(path.join(root, "run-"));
}

function extensionSource(
  socketPath: string,
  defaultRepo: string,
  visibleRepos: readonly string[]
): string {
  const repos = visibleRepos.length === 0 ? [defaultRepo] : visibleRepos;
  return [
    'import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";',
    'import http from "node:http";',
    "",
    `const socketPath = ${JSON.stringify(socketPath)};`,
    `const bindPayload = ${JSON.stringify({ default_repo: defaultRepo, visible_repos: repos })};`,
    "let runId: string | undefined;",
    "",
    "type BashParams = { command?: unknown; cmd?: unknown };",
    "type BindResponse = { run_id: string; cwd: string; snapshots: Record<string, string> };",
    "type ExecResponse = {",
    "  stdout?: string;",
    "  stderr?: string;",
    "  exit_code?: number;",
    "  policy_error?: string;",
    "  truncated?: boolean;",
    "};",
    "",
    "export default function localpagerRepoReaderBashExtension(pi: ExtensionAPI): void {",
    "  pi.registerTool({",
    '    name: "bash",',
    '    label: "Bash",',
    '    description: "Run a read-only bash command in configured repository snapshots.",',
    '    promptSnippet: "Use bash to read repository files when needed. Commands are read-only and start in the configured default repo.",',
    "    promptGuidelines: [",
    '      "Use bash only when repository files are needed to resolve ambiguity.",',
    '      "Prefer rg, find, sed -n, cat, head, and git grep for concise inspection.",',
    '      "Do not attempt write, network, package-manager, interpreter, pipeline, or redirect commands."',
    "    ],",
    "    parameters: {",
    '      type: "object",',
    "      additionalProperties: false,",
    '      required: ["command"],',
    "      properties: {",
    '        command: { type: "string", description: "Read-only bash-shaped command to run." }',
    "      }",
    "    },",
    "    async execute(_toolCallId: string, params: unknown) {",
    "      const command = readCommand(params);",
    "      if (runId === undefined) {",
    '        const binding = await postJSON<BindResponse>("/bind", bindPayload);',
    "        runId = binding.run_id;",
    "      }",
    '      const result = await postJSON<ExecResponse>("/exec", { run_id: runId, command });',
    "      return {",
    '        content: [{ type: "text", text: formatResult(result) }],',
    "        details: result",
    "      };",
    "    }",
    "  });",
    "}",
    "",
    "function readCommand(params: unknown): string {",
    '  if (typeof params !== "object" || params === null) {',
    '    throw new Error("bash params must be an object");',
    "  }",
    "  const command = (params as BashParams).command;",
    '  if (typeof command !== "string" || command.trim().length === 0) {',
    '    throw new Error("bash.command must be a non-empty string");',
    "  }",
    "  return command;",
    "}",
    "",
    "function postJSON<T>(path: string, payload: unknown): Promise<T> {",
    "  return new Promise<T>((resolve, reject) => {",
    "    const body = JSON.stringify(payload);",
    "    const req = http.request(",
    "      {",
    "        socketPath,",
    "        path,",
    '        method: "POST",',
    "        headers: {",
    '          "content-type": "application/json",',
    '          "content-length": Buffer.byteLength(body)',
    "        }",
    "      },",
    "      (res) => {",
    "        const chunks: Buffer[] = [];",
    '        res.on("data", (chunk: Buffer) => chunks.push(chunk));',
    '        res.on("end", () => {',
    '          const raw = Buffer.concat(chunks).toString("utf8");',
    "          if ((res.statusCode ?? 500) < 200 || (res.statusCode ?? 500) >= 300) {",
    "            reject(new Error(`repo-reader ${path} failed: ${raw}`));",
    "            return;",
    "          }",
    "          resolve(JSON.parse(raw) as T);",
    "        });",
    "      }",
    "    );",
    '    req.on("error", reject);',
    "    req.end(body);",
    "  });",
    "}",
    "",
    "function formatResult(result: ExecResponse): string {",
    "  const parts: string[] = [];",
    "  if (result.stdout !== undefined && result.stdout.length > 0) {",
    "    parts.push(result.stdout);",
    "  }",
    "  if (result.stderr !== undefined && result.stderr.length > 0) {",
    "    parts.push(`stderr:\\n${result.stderr}`);",
    "  }",
    "  if (result.policy_error !== undefined && result.policy_error.length > 0) {",
    "    parts.push(`policy_error: ${result.policy_error}`);",
    "  }",
    "  if (result.exit_code !== undefined && result.exit_code !== 0) {",
    "    parts.push(`exit_code: ${result.exit_code}`);",
    "  }",
    "  if (result.truncated === true) {",
    '    parts.push("truncated: true");',
    "  }",
    '  return parts.length === 0 ? "exit_code: 0" : parts.join("\\n");',
    "}",
    ""
  ].join("\n");
}
