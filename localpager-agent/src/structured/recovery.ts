import { constants } from "node:fs";
import { access, readdir, readFile, stat } from "node:fs/promises";
import path from "node:path";

import type { LocalpagerAgentOptions } from "../agent/options.js";

export type FinalJsonRecoveryCandidate = {
  readonly sessionPath: string;
  readonly payload: Record<string, unknown>;
};

export async function findFinalJsonRecoveryCandidate(
  options: LocalpagerAgentOptions,
  sinceMs: number
): Promise<FinalJsonRecoveryCandidate | undefined> {
  const sessionPath = await recoverySessionPath(options, sinceMs);
  if (sessionPath === undefined) {
    return undefined;
  }
  const payload = await readSessionRecoveryPayload(sessionPath);
  return payload === undefined ? undefined : { sessionPath, payload };
}

export function recoveryForwardedArgs(
  forwardedArgs: readonly string[],
  sessionPath: string,
  payload: Record<string, unknown>
): string[] {
  return [
    ...withoutValueFlags(forwardedArgs, new Set(["-p", "--print", "--session"])),
    "--session",
    sessionPath,
    "-p",
    recoveryPrompt(payload)
  ];
}

export function retryForwardedArgs(forwardedArgs: readonly string[]): string[] | undefined {
  const prompt = promptValue(forwardedArgs);
  if (prompt === undefined) {
    return undefined;
  }
  return [
    ...withoutValueFlags(forwardedArgs, new Set(["-p", "--print", "--session"])),
    "-p",
    `${prompt}\n\n${retryInstruction}`
  ];
}

export function extractFinalJsonPayload(text: string): Record<string, unknown> | undefined {
  return (
    extractPseudoToolPayload(text) ??
    parseObject(text.trim()) ??
    parseObject(fencedJson(text)) ??
    parseObject(looseJsonObject(text))
  );
}

async function recoverySessionPath(
  options: LocalpagerAgentOptions,
  sinceMs: number
): Promise<string | undefined> {
  const explicit = explicitSessionPath(options.forwardedArgs);
  if (explicit !== undefined) {
    return (await canRead(explicit)) ? explicit : undefined;
  }
  return await newestSessionPath(options.sessionDir, sinceMs);
}

async function readSessionRecoveryPayload(
  sessionPath: string
): Promise<Record<string, unknown> | undefined> {
  const lines = (await readFile(sessionPath, "utf8")).trim().split("\n").reverse();
  for (const line of lines) {
    const payload = extractFinalJsonPayload(assistantText(line));
    if (payload !== undefined) {
      return payload;
    }
  }
  return undefined;
}

function assistantText(line: string): string {
  const parsed = parseUnknown(line);
  if (!isRecord(parsed) || parsed["type"] !== "message") {
    return "";
  }
  const message = parsed["message"];
  if (!isRecord(message) || message["role"] !== "assistant") {
    return "";
  }
  const content = message["content"];
  return Array.isArray(content) ? content.map(contentText).join("\n") : "";
}

function contentText(value: unknown): string {
  return isRecord(value) && value["type"] === "text" && typeof value["text"] === "string"
    ? value["text"]
    : "";
}

function extractPseudoToolPayload(text: string): Record<string, unknown> | undefined {
  const prefix = "call:final_json";
  const start = text.lastIndexOf(prefix);
  if (start < 0) {
    return undefined;
  }
  const afterPrefix = start + prefix.length;
  const open = text.indexOf("{", afterPrefix);
  const end = pseudoToolEnd(text, open);
  return open < 0 || end < 0 ? undefined : parseLooseObject(text.slice(open, end + 1));
}

function pseudoToolEnd(text: string, open: number): number {
  if (open < 0) {
    return -1;
  }
  const marker = text.indexOf("<tool_call|>", open);
  const limit = marker < 0 ? text.length : marker;
  return text.lastIndexOf("}", limit);
}

function parseLooseObject(raw: string): Record<string, unknown> | undefined {
  return parseObject(quoteObjectKeys(raw.replaceAll('<|"|>', '"')));
}

function quoteObjectKeys(raw: string): string {
  return raw.replace(/([{,]\s*)([A-Za-z_][A-Za-z0-9_]*)\s*:/gu, '$1"$2":');
}

function fencedJson(text: string): string | undefined {
  return /```(?:json)?\s*([\s\S]*?)```/u.exec(text)?.[1]?.trim();
}

function looseJsonObject(text: string): string | undefined {
  const open = text.indexOf("{");
  const close = text.lastIndexOf("}");
  return open < 0 || close <= open ? undefined : text.slice(open, close + 1);
}

function parseObject(raw: string | undefined): Record<string, unknown> | undefined {
  const parsed = raw === undefined ? undefined : parseUnknown(raw);
  return isRecord(parsed) ? parsed : undefined;
}

function parseUnknown(raw: string): unknown {
  try {
    return JSON.parse(raw) as unknown;
  } catch {
    return undefined;
  }
}

async function newestSessionPath(sessionDir: string, sinceMs: number): Promise<string | undefined> {
  const entries = await readdir(sessionDir, { withFileTypes: true }).catch(() => []);
  const files = entries
    .filter((entry) => entry.isFile() && entry.name.endsWith(".jsonl"))
    .map((entry) => path.join(sessionDir, entry.name));
  const stats = await Promise.all(
    files.map(async (file) => ({ file, fileStat: await stat(file) }))
  );
  return stats
    .filter(({ fileStat }) => fileStat.mtimeMs >= sinceMs - 1000)
    .sort((left, right) => right.fileStat.mtimeMs - left.fileStat.mtimeMs)[0]?.file;
}

function explicitSessionPath(args: readonly string[]): string | undefined {
  const index = args.indexOf("--session");
  const value = index < 0 ? undefined : args[index + 1];
  return value === undefined ? undefined : path.resolve(value);
}

function promptValue(args: readonly string[]): string | undefined {
  for (const flag of ["-p", "--print"]) {
    const index = args.lastIndexOf(flag);
    const value = index < 0 ? undefined : args[index + 1];
    if (value !== undefined) {
      return value;
    }
  }
  return undefined;
}

async function canRead(file: string): Promise<boolean> {
  try {
    await access(file, constants.R_OK);
    return true;
  } catch {
    return false;
  }
}

function withoutValueFlags(args: readonly string[], flags: ReadonlySet<string>): string[] {
  const kept: string[] = [];
  for (let index = 0; index < args.length; index += 1) {
    const arg = args[index];
    if (arg === undefined) {
      continue;
    }
    if (flags.has(arg)) {
      index += 1;
      continue;
    }
    kept.push(arg);
  }
  return kept;
}

const retryInstruction = [
  "Retry instruction: your previous response did not call the final_json tool.",
  "Do not call bash. Do not explain anything in prose, markdown, or JSON text.",
  "Call final_json exactly once with a JSON object that matches the schema."
].join("\n\n");

function recoveryPrompt(payload: Record<string, unknown>): string {
  return [
    "You already completed this same localpager-agent run, but your previous answer wrote a pseudo final_json call as text.",
    "Now call the real final_json tool exactly once with this exact JSON object:",
    JSON.stringify(payload, null, 2),
    "Do not call bash. Do not write prose, markdown, or JSON text. The only valid response is the final_json tool call."
  ].join("\n\n");
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
