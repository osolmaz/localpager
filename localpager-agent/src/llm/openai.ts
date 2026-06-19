import { asArray, asObject, optionalString } from "../common/json.js";

type Fetcher = typeof fetch;

export type ModelInfo = {
  readonly id: string;
  readonly name?: string;
  readonly contextWindow?: number;
};

export type ResolvedLocalModel = {
  readonly model: string;
  readonly availableModels: readonly string[];
  readonly contextWindow?: number;
  readonly serverModelName?: string;
};

type ResolvedLocalModelBase = {
  readonly model: string;
  readonly availableModels: readonly string[];
  readonly contextWindow?: number;
};

export function normalizeBaseUrl(value: string): string {
  return value.replace(/\/+$/u, "");
}

export async function listModels(
  baseUrl: string,
  timeoutMs = 3000,
  fetcher: Fetcher = fetch
): Promise<readonly ModelInfo[]> {
  const modelsUrl = `${normalizeBaseUrl(baseUrl)}/models`;
  let response: Response;
  try {
    response = await fetcher(modelsUrl, {
      signal: AbortSignal.timeout(timeoutMs)
    });
  } catch (error) {
    throw new Error(`model list failed for ${modelsUrl}: ${errorMessage(error)}`);
  }
  if (!response.ok) {
    throw new Error(`model list failed for ${modelsUrl} with HTTP ${String(response.status)}`);
  }
  const payload: unknown = await response.json();
  const root = asObject(payload, "models response");
  const data = asArray(root["data"], "models response data");
  return data
    .map((entry) => modelInfo(asObject(entry, "model entry")))
    .filter((model): model is ModelInfo => model !== undefined);
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

export async function resolveLocalModel(
  baseUrl: string,
  requestedModel: string,
  timeoutMs = 3000,
  fetcher: Fetcher = fetch
): Promise<ResolvedLocalModel> {
  const modelInfos = await listModels(baseUrl, timeoutMs, fetcher);
  const availableModels = modelInfos.map((model) => model.id);
  if (requestedModel === "auto") {
    return resolveAutoModel(baseUrl, modelInfos, availableModels);
  }
  if (availableModels.length > 0 && !availableModels.includes(requestedModel)) {
    throw new Error(
      `model ${requestedModel} is not reported by ${normalizeBaseUrl(baseUrl)}/models; available: ${availableModels.join(", ")}`
    );
  }
  const selected = modelInfos.find((model) => model.id === requestedModel);
  return resolvedLocalModel(
    withOptionalContextWindow({ model: requestedModel, availableModels }, selected?.contextWindow),
    selected?.name
  );
}

function resolveAutoModel(
  baseUrl: string,
  modelInfos: readonly ModelInfo[],
  availableModels: readonly string[]
): ResolvedLocalModel {
  const first = modelInfos[0];
  if (first === undefined) {
    throw new Error(`no models returned by ${normalizeBaseUrl(baseUrl)}/models`);
  }
  return resolvedLocalModel(
    withOptionalContextWindow({ model: first.id, availableModels }, first.contextWindow),
    first.name
  );
}

function modelInfo(entry: Record<string, unknown>): ModelInfo | undefined {
  const id = optionalString(entry["id"]);
  if (id === undefined) {
    return undefined;
  }
  return withOptionalContextWindow(
    withOptionalName({ id }, optionalString(entry["name"])),
    findContextWindow(entry)
  );
}

function withOptionalContextWindow<T extends Record<string, unknown>>(
  value: T,
  contextWindow: number | undefined
): T & { readonly contextWindow?: number } {
  if (contextWindow === undefined) {
    return value;
  }
  return { ...value, contextWindow };
}

function withOptionalName<T extends Record<string, unknown>>(
  value: T,
  name: string | undefined
): T & { readonly name?: string } {
  if (name === undefined || name.trim() === "") {
    return value;
  }
  return { ...value, name };
}

function resolvedLocalModel(
  value: ResolvedLocalModelBase,
  serverModelName: string | undefined
): ResolvedLocalModel {
  if (serverModelName === undefined || serverModelName.trim() === "") {
    return value;
  }
  return { ...value, serverModelName };
}

function findContextWindow(entry: Record<string, unknown>): number | undefined {
  for (const key of [
    "context_window",
    "contextWindow",
    "context_length",
    "contextLength",
    "max_context_length",
    "maxContextLength",
    "n_ctx",
    "max_input_tokens",
    "maxInputTokens"
  ]) {
    const value = positiveInteger(entry[key]);
    if (value !== undefined) {
      return value;
    }
  }
  const metadata = entry["metadata"];
  if (metadata !== null && typeof metadata === "object" && !Array.isArray(metadata)) {
    return findContextWindow(metadata as Record<string, unknown>);
  }
  return undefined;
}

function positiveInteger(value: unknown): number | undefined {
  if (typeof value === "number" && Number.isInteger(value) && value > 0) {
    return value;
  }
  if (typeof value === "string" && /^[1-9]\d*$/u.test(value)) {
    return Number.parseInt(value, 10);
  }
  return undefined;
}
