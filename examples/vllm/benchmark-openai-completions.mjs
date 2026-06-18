#!/usr/bin/env node

const args = parseArgs(process.argv.slice(2));
const prompt = args.prompt ?? "Continue with plain lowercase words separated by spaces until the response limit is reached.";

const startedAt = Date.now();
const results = await runAll();
const wallSeconds = (Date.now() - startedAt) / 1000;
const completionTokens = results.reduce((sum, result) => sum + result.completion_tokens, 0);
const promptTokens = results.reduce((sum, result) => sum + result.prompt_tokens, 0);
const totalTokens = results.reduce((sum, result) => sum + result.total_tokens, 0);
const failed = results.filter((result) => result.error);

console.log(JSON.stringify({
  model: args.model,
  base_url: args.baseUrl,
  concurrency: args.concurrency,
  requests: args.requests,
  max_tokens: args.maxTokens,
  wall_seconds: Number(wallSeconds.toFixed(3)),
  ok: results.length - failed.length,
  failed: failed.length,
  prompt_tokens: promptTokens,
  completion_tokens: completionTokens,
  total_tokens: totalTokens,
  completion_tokens_per_second: Number((completionTokens / wallSeconds).toFixed(3)),
  total_tokens_per_second: Number((totalTokens / wallSeconds).toFixed(3)),
  per_request_completion_tokens_per_second: results.map((result) => result.completion_tokens_per_second)
}, null, 2));

if (failed.length > 0) {
  process.exitCode = 1;
}

async function runAll() {
  const results = [];
  let next = 0;
  const workers = Array.from({ length: args.concurrency }, async () => {
    while (next < args.requests) {
      const index = next;
      next += 1;
      results[index] = await runOne(index);
    }
  });
  await Promise.all(workers);
  return results;
}

async function runOne(index) {
  const started = Date.now();
  try {
    const response = await fetch(`${args.baseUrl.replace(/\/$/u, "")}/chat/completions`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({
        model: args.model,
        messages: [
          { role: "user", content: `${prompt}\n\nRequest index: ${index}` }
        ],
        temperature: args.temperature,
        max_tokens: args.maxTokens
      })
    });
    const text = await response.text();
    if (!response.ok) {
      return {
        error: `HTTP ${response.status}`,
        body: text.slice(0, 1000),
        prompt_tokens: 0,
        completion_tokens: 0,
        total_tokens: 0,
        completion_tokens_per_second: 0
      };
    }
    const data = JSON.parse(text);
    const seconds = (Date.now() - started) / 1000;
    const usage = data.usage ?? {};
    const completionTokens = Number(usage.completion_tokens ?? 0);
    return {
      prompt_tokens: Number(usage.prompt_tokens ?? 0),
      completion_tokens: completionTokens,
      total_tokens: Number(usage.total_tokens ?? 0),
      completion_tokens_per_second: Number((completionTokens / seconds).toFixed(3))
    };
  } catch (error) {
    return {
      error: error instanceof Error ? error.message : String(error),
      prompt_tokens: 0,
      completion_tokens: 0,
      total_tokens: 0,
      completion_tokens_per_second: 0
    };
  }
}

function parseArgs(argv) {
  const parsed = {
    baseUrl: "http://127.0.0.1:8000/v1",
    concurrency: 1,
    requests: 1,
    maxTokens: 512,
    temperature: 0
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--base-url") {
      parsed.baseUrl = value(argv, ++index, arg);
    } else if (arg === "--model") {
      parsed.model = value(argv, ++index, arg);
    } else if (arg === "--concurrency") {
      parsed.concurrency = positiveInteger(value(argv, ++index, arg), arg);
    } else if (arg === "--requests") {
      parsed.requests = positiveInteger(value(argv, ++index, arg), arg);
    } else if (arg === "--max-tokens") {
      parsed.maxTokens = positiveInteger(value(argv, ++index, arg), arg);
    } else if (arg === "--temperature") {
      parsed.temperature = Number(value(argv, ++index, arg));
      if (!Number.isFinite(parsed.temperature)) {
        throw new Error(`${arg} must be a number`);
      }
    } else if (arg === "--prompt") {
      parsed.prompt = value(argv, ++index, arg);
    } else {
      throw new Error(`unknown option: ${arg}`);
    }
  }
  if (!parsed.model) {
    throw new Error("--model is required");
  }
  if (parsed.concurrency > parsed.requests) {
    parsed.concurrency = parsed.requests;
  }
  return parsed;
}

function value(argv, index, arg) {
  const next = argv[index];
  if (!next) {
    throw new Error(`${arg} requires a value`);
  }
  return next;
}

function positiveInteger(raw, arg) {
  const parsed = Number(raw);
  if (!Number.isInteger(parsed) || parsed <= 0) {
    throw new Error(`${arg} must be a positive integer`);
  }
  return parsed;
}
