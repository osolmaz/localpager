import { mkdir, mkdtemp, readFile, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

export type PromptTemplateOptions = {
  readonly promptTemplatePath: string | undefined;
  readonly promptVarsPaths: readonly string[];
  readonly promptVars: readonly string[];
  readonly renderedPromptPath: string | undefined;
  readonly forwardedArgs: readonly string[];
};

type PromptValues = Record<string, unknown>;

export async function promptForwardedArgs(
  options: PromptTemplateOptions
): Promise<readonly string[]> {
  validatePromptOptions(options);
  if (options.promptTemplatePath === undefined) {
    return options.forwardedArgs;
  }
  rejectExistingPrintPrompt(options.forwardedArgs);
  const values = await loadPromptValues(options.promptVarsPaths, options.promptVars);
  const template = await readFile(path.resolve(options.promptTemplatePath), "utf8");
  const prompt = renderPromptTemplate(template, values);
  const renderedPromptPath = options.renderedPromptPath ?? (await temporaryRenderedPromptPath());
  await writeRenderedPrompt(renderedPromptPath, prompt);
  return [...options.forwardedArgs, "-p", `@${path.resolve(renderedPromptPath)}`];
}

export function renderPromptTemplate(template: string, values: PromptValues): string {
  const withoutComments = template.replace(/\{\{![\s\S]*?\}\}/gu, "");
  const withBlocks = renderIfBlocks(withoutComments, values);
  return withBlocks
    .replace(/\{\{\{\s*([A-Za-z0-9_]+)\s*\}\}\}/gu, (_match, key: string) =>
      templateValue(requiredTemplateValue(values, key))
    )
    .replace(/\{\{\s*([A-Za-z0-9_]+)\s*\}\}/gu, (_match, key: string) =>
      templateValue(requiredTemplateValue(values, key))
    );
}

async function loadPromptValues(
  varsPaths: readonly string[],
  inlineVars: readonly string[]
): Promise<PromptValues> {
  const values: PromptValues = {};
  for (const varsPath of varsPaths) {
    Object.assign(values, await readVarsFile(varsPath));
  }
  for (const inlineVar of inlineVars) {
    const parsed = parseInlineVar(inlineVar);
    values[parsed.key] = parsed.value;
  }
  return values;
}

async function readVarsFile(varsPath: string): Promise<PromptValues> {
  const resolvedPath = path.resolve(varsPath);
  const parsed = JSON.parse(await readFile(resolvedPath, "utf8")) as unknown;
  if (!isRecord(parsed)) {
    throw new Error(`prompt vars file must be a JSON object: ${resolvedPath}`);
  }
  return parsed;
}

function parseInlineVar(value: string): { readonly key: string; readonly value: string } {
  const separatorIndex = value.indexOf("=");
  if (separatorIndex <= 0) {
    throw new Error("--prompt-var must use key=value");
  }
  const key = value.slice(0, separatorIndex).trim();
  if (!/^[A-Za-z0-9_]+$/u.test(key)) {
    throw new Error(`prompt variable key must be alphanumeric or underscore: ${key}`);
  }
  return { key, value: value.slice(separatorIndex + 1) };
}

function renderIfBlocks(template: string, values: PromptValues): string {
  const blockPattern =
    /\{\{#if\s+([A-Za-z0-9_]+)\s*\}\}((?:(?!\{\{#if|\{\{\/if\}\})[\s\S])*?)\{\{\/if\}\}/gu;
  let rendered = template;
  let previous = "";
  while (previous !== rendered) {
    previous = rendered;
    rendered = rendered.replace(blockPattern, (_match, key: string, body: string) =>
      isTruthyTemplateValue(values[key]) ? body : ""
    );
  }
  if (rendered.includes("{{#if") || rendered.includes("{{/if}}")) {
    throw new Error("unsupported nested prompt template block");
  }
  return rendered;
}

function requiredTemplateValue(values: PromptValues, key: string): unknown {
  if (!Object.hasOwn(values, key)) {
    throw new Error(`missing prompt template variable: ${key}`);
  }
  return values[key];
}

function templateValue(value: unknown): string {
  if (value === null || value === undefined) {
    return "";
  }
  if (typeof value === "string") {
    return value;
  }
  if (typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }
  return JSON.stringify(value, null, 2);
}

function isTruthyTemplateValue(value: unknown): boolean {
  if (Array.isArray(value)) {
    return value.length > 0;
  }
  return Boolean(value);
}

function validatePromptOptions(options: PromptTemplateOptions): void {
  if (options.promptTemplatePath !== undefined) {
    return;
  }
  if (options.promptVarsPaths.length > 0 || options.promptVars.length > 0) {
    throw new Error("--prompt-vars-file and --prompt-var require --prompt-template");
  }
  if (options.renderedPromptPath !== undefined) {
    throw new Error("--write-rendered-prompt requires --prompt-template");
  }
}

function rejectExistingPrintPrompt(args: readonly string[]): void {
  if (args.includes("-p") || args.includes("--print")) {
    throw new Error("--prompt-template cannot be combined with Pi -p/--print");
  }
}

async function writeRenderedPrompt(filePath: string, prompt: string): Promise<void> {
  const resolvedPath = path.resolve(filePath);
  await mkdir(path.dirname(resolvedPath), { recursive: true });
  await writeFile(resolvedPath, prompt, "utf8");
}

async function temporaryRenderedPromptPath(): Promise<string> {
  const directory = await mkdtemp(path.join(os.tmpdir(), "localpager-agent-prompt-"));
  return path.join(directory, "prompt.md");
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
