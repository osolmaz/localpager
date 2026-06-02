#!/usr/bin/env node
import { existsSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import path from "node:path";
import { spawn } from "node:child_process";

const startedAt = new Date();
const options = parseArgs(process.argv.slice(2));

if (options.help) {
  usage(0);
}
if (!options.repo) {
  usage(2);
}

await main();

async function main() {
  const repo = parseRepo(options.repo);
  const outputDir = path.resolve(options.outputDir || path.join("experiment-runs", timestamp(startedAt)));
  prepareOutputDir(outputDir, options.overwrite);
  mkdirSync(outputDir, { recursive: true });
  mkdirSync(path.join(outputDir, "contexts"), { recursive: true });
  mkdirSync(path.join(outputDir, "prompts"), { recursive: true });

  const topics = options.topicTaxonomy ? loadTopics(resolvePath(options.topicTaxonomy)) : [];
  const baseSchema = JSON.parse(readFileSync(resolvePath(options.schema), "utf8"));
  const runtimeSchema = renderSchema(baseSchema, topics);
  const promptTemplate = readFileSync(resolvePath(options.promptTemplate), "utf8");

  writeFileSync(path.join(outputDir, "schema.runtime.json"), JSON.stringify(runtimeSchema, null, 2) + "\n");
  writeFileSync(path.join(outputDir, "config.json"), JSON.stringify(redactedConfig(options, outputDir), null, 2) + "\n");

  const items = await collectItems(repo, options);
  const referenceRows = [];
  const targetRows = [];
  const resultRows = [];

  for (let index = 0; index < items.length; index += 1) {
    const item = items[index];
    const safeName = `${String(index + 1).padStart(3, "0")}-${item.kind}-${item.number}`;
    const context = renderContext(item, options);
    const prompt = renderPrompt(promptTemplate, item.url, topics, context);
    const contextPath = path.join(outputDir, "contexts", `${safeName}.md`);
    writeFileSync(contextPath, context);
    writeFileSync(path.join(outputDir, "prompts", `${safeName}.md`), prompt);

    const reference = await classify("reference", contextPath, topics, item, options);
    const target = await classify("target", contextPath, topics, item, options);
    const comparison = compareOutputs(reference.output, target.output);

    referenceRows.push({ item: itemSummary(item), ...reference });
    targetRows.push({ item: itemSummary(item), ...target });
    resultRows.push({ item: itemSummary(item), comparison });
  }

  writeJsonl(path.join(outputDir, "items.jsonl"), items.map(itemSummary));
  writeJsonl(path.join(outputDir, "reference-outputs.jsonl"), referenceRows);
  writeJsonl(path.join(outputDir, "target-outputs.jsonl"), targetRows);
  writeJsonl(path.join(outputDir, "per-row-results.jsonl"), resultRows);

  const summary = summarize({ options, repo, outputDir, startedAt, finishedAt: new Date(), items, referenceRows, targetRows, resultRows });
  writeFileSync(path.join(outputDir, "summary.md"), summary);
  process.stdout.write(`${summary}\n`);
}

function parseArgs(args) {
  const parsed = {
    help: false,
    overwrite: false,
    repo: "",
    itemType: "both",
    limit: 5,
    outputDir: "",
    schema: "schemas/classification.schema.json",
    promptTemplate: "examples/profiles/repo-routing.prompt.md",
    topicTaxonomy: "examples/profiles/repo-routing-topics.json",
    githubBaseUrl: "https://api.github.com",
    githubTokenEnv: "GITHUB_TOKEN",
    referenceBaseUrl: "",
    referenceModel: "mock",
    targetBaseUrl: "",
    targetModel: "mock",
    classifierCommand: "scripts/localpager-classifier",
    contextWindow: 0,
    maxTokens: 512,
    timeoutMs: 120000,
    maxBodyChars: 5000,
    maxCommentsChars: 4000,
    maxChangedFilesChars: 5000,
    maxDiffChars: 10000,
  };
  for (let index = 0; index < args.length; index += 1) {
    const arg = args[index];
    if (arg === "--help" || arg === "-h") {
      parsed.help = true;
    } else if (arg === "--repo") {
      parsed.repo = requiredValue(args, ++index, arg);
    } else if (arg === "--item-type") {
      parsed.itemType = requiredValue(args, ++index, arg);
    } else if (arg === "--limit") {
      parsed.limit = parsePositiveInt(requiredValue(args, ++index, arg), arg);
    } else if (arg === "--output-dir") {
      parsed.outputDir = requiredValue(args, ++index, arg);
    } else if (arg === "--overwrite") {
      parsed.overwrite = true;
    } else if (arg === "--schema") {
      parsed.schema = requiredValue(args, ++index, arg);
    } else if (arg === "--prompt-template") {
      parsed.promptTemplate = requiredValue(args, ++index, arg);
    } else if (arg === "--topic-taxonomy") {
      parsed.topicTaxonomy = requiredValue(args, ++index, arg);
    } else if (arg === "--github-base-url") {
      parsed.githubBaseUrl = requiredValue(args, ++index, arg).replace(/\/$/u, "");
    } else if (arg === "--github-token-env") {
      parsed.githubTokenEnv = requiredValue(args, ++index, arg);
    } else if (arg === "--reference-base-url") {
      parsed.referenceBaseUrl = requiredValue(args, ++index, arg).replace(/\/$/u, "");
    } else if (arg === "--reference-model") {
      parsed.referenceModel = requiredValue(args, ++index, arg);
    } else if (arg === "--target-base-url") {
      parsed.targetBaseUrl = requiredValue(args, ++index, arg).replace(/\/$/u, "");
    } else if (arg === "--target-model") {
      parsed.targetModel = requiredValue(args, ++index, arg);
    } else if (arg === "--classifier-command") {
      parsed.classifierCommand = requiredValue(args, ++index, arg);
    } else if (arg === "--context-window") {
      parsed.contextWindow = parsePositiveInt(requiredValue(args, ++index, arg), arg);
    } else if (arg === "--max-tokens") {
      parsed.maxTokens = parsePositiveInt(requiredValue(args, ++index, arg), arg);
    } else if (arg === "--timeout-ms") {
      parsed.timeoutMs = parsePositiveInt(requiredValue(args, ++index, arg), arg);
    } else if (arg === "--max-body-chars") {
      parsed.maxBodyChars = parsePositiveInt(requiredValue(args, ++index, arg), arg);
    } else if (arg === "--max-comments-chars") {
      parsed.maxCommentsChars = parsePositiveInt(requiredValue(args, ++index, arg), arg);
    } else if (arg === "--max-changed-files-chars") {
      parsed.maxChangedFilesChars = parsePositiveInt(requiredValue(args, ++index, arg), arg);
    } else if (arg === "--max-diff-chars") {
      parsed.maxDiffChars = parsePositiveInt(requiredValue(args, ++index, arg), arg);
    } else {
      throw new Error(`unsupported argument: ${arg}`);
    }
  }
  if (!["both", "issues", "prs"].includes(parsed.itemType)) {
    throw new Error("--item-type must be one of: both, issues, prs");
  }
  return parsed;
}

function requiredValue(args, index, flag) {
  const value = args[index];
  if (!value) {
    throw new Error(`${flag} requires a value`);
  }
  return value;
}

function parsePositiveInt(value, flag) {
  const parsed = Number.parseInt(value, 10);
  if (!Number.isInteger(parsed) || parsed <= 0) {
    throw new Error(`${flag} must be a positive integer`);
  }
  return parsed;
}

function usage(status) {
  const output = status === 0 ? process.stdout : process.stderr;
  output.write(
    [
      "usage: localpager-experiment --repo OWNER/REPO [--limit N] [--item-type both|issues|prs]",
      "       [--output-dir DIR] [--overwrite] [--reference-model mock] [--target-model mock]",
      "       [--target-base-url http://127.0.0.1:1234/v1 --target-model MODEL]",
      "       [--classifier-command scripts/localpager-classifier]",
      "",
    ].join("\n"),
  );
  process.exit(status);
}

function prepareOutputDir(outputDir, overwrite) {
  if (!existsSync(outputDir)) {
    return;
  }
  if (readdirSync(outputDir).length === 0) {
    return;
  }
  if (!overwrite) {
    throw new Error(`output directory is not empty: ${outputDir}. Pass --overwrite or choose a new directory.`);
  }
  rmSync(outputDir, { recursive: true, force: true });
}

function parseRepo(repo) {
  const match = repo.match(/^([^/\s]+)\/([^/\s]+)$/u);
  if (!match) {
    throw new Error(`repo must look like OWNER/REPO: ${repo}`);
  }
  return { owner: match[1], name: match[2], fullName: repo };
}

async function collectItems(repo, opts) {
  const candidates = [];
  if (opts.itemType === "both" || opts.itemType === "prs") {
    const prs = await githubJSON(opts, `/repos/${repo.fullName}/pulls?state=open&sort=updated&direction=desc&per_page=${opts.limit}`);
    for (const pr of prs) {
      candidates.push(await hydratePullRequest(repo, pr, opts));
    }
  }
  if (opts.itemType === "both" || opts.itemType === "issues") {
    const issues = await githubJSON(opts, `/repos/${repo.fullName}/issues?state=open&sort=updated&direction=desc&per_page=${opts.limit * 2}`);
    for (const issue of issues.filter((entry) => !entry.pull_request).slice(0, opts.limit)) {
      candidates.push(await hydrateIssue(repo, issue, opts));
    }
  }
  return candidates
    .sort((left, right) => new Date(right.updated_at).getTime() - new Date(left.updated_at).getTime())
    .slice(0, opts.limit);
}

async function hydrateIssue(repo, issue, opts) {
  const comments = await githubJSON(opts, `/repos/${repo.fullName}/issues/${issue.number}/comments?per_page=100`);
  return {
    repo: repo.fullName,
    kind: "issue",
    number: issue.number,
    title: issue.title || "",
    url: issue.html_url,
    state: issue.state,
    author: issue.user?.login || "",
    labels: (issue.labels || []).map((label) => label.name).filter(Boolean),
    body: issue.body || "",
    comments: comments.map(commentSummary),
    changedFiles: [],
    diff: "",
    updated_at: issue.updated_at,
  };
}

async function hydratePullRequest(repo, pr, opts) {
  const [comments, files, diff] = await Promise.all([
    githubJSON(opts, `/repos/${repo.fullName}/issues/${pr.number}/comments?per_page=100`),
    githubJSON(opts, `/repos/${repo.fullName}/pulls/${pr.number}/files?per_page=100`),
    githubText(opts, `/repos/${repo.fullName}/pulls/${pr.number}`, "application/vnd.github.v3.diff").catch((error) => `Diff unavailable: ${error.message}`),
  ]);
  return {
    repo: repo.fullName,
    kind: "pull_request",
    number: pr.number,
    title: pr.title || "",
    url: pr.html_url,
    state: pr.state,
    author: pr.user?.login || "",
    labels: (pr.labels || []).map((label) => label.name).filter(Boolean),
    body: pr.body || "",
    comments: comments.map(commentSummary),
    changedFiles: files.map(fileSummary),
    diff,
    updated_at: pr.updated_at,
  };
}

async function githubJSON(opts, endpoint) {
  const text = await githubText(opts, endpoint, "application/vnd.github+json");
  return JSON.parse(text);
}

async function githubText(opts, endpoint, accept) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), opts.timeoutMs);
  try {
    const response = await fetch(`${opts.githubBaseUrl}${endpoint}`, {
      headers: githubHeaders(opts, accept),
      signal: controller.signal,
    });
    const body = await response.text();
    if (!response.ok) {
      throw new Error(`GitHub ${response.status} ${response.statusText}: ${body.slice(0, 300)}`);
    }
    return body;
  } finally {
    clearTimeout(timer);
  }
}

function githubHeaders(opts, accept) {
  const headers = {
    Accept: accept,
    "User-Agent": "localpager-experiment",
    "X-GitHub-Api-Version": "2022-11-28",
  };
  const token = opts.githubTokenEnv ? process.env[opts.githubTokenEnv] : "";
  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }
  return headers;
}

function commentSummary(comment) {
  return {
    author: comment.user?.login || "",
    created_at: comment.created_at || "",
    body: comment.body || "",
  };
}

function fileSummary(file) {
  return {
    filename: file.filename || "",
    status: file.status || "",
    additions: file.additions || 0,
    deletions: file.deletions || 0,
    changes: file.changes || 0,
  };
}

function renderContext(item, opts) {
  const comments = item.comments
    .map((comment) => `- ${comment.author} at ${comment.created_at}: ${oneLine(comment.body)}`)
    .join("\n");
  const files = item.changedFiles
    .map((file) => `- ${file.status} ${file.filename} (+${file.additions}/-${file.deletions})`)
    .join("\n");
  return [
    "GitHub item:",
    `- Repository: ${item.repo}`,
    `- Type: ${item.kind}`,
    `- Number: ${item.number}`,
    `- URL: ${item.url}`,
    `- Title: ${neutralize(item.title)}`,
    `- State: ${item.state}`,
    `- Author: ${item.author}`,
    `- Labels: ${item.labels.length > 0 ? item.labels.join(", ") : "none"}`,
    "",
    "Body:",
    "```markdown",
    truncate(neutralize(item.body || "No body."), opts.maxBodyChars),
    "```",
    "",
    "Issue comments:",
    "```text",
    truncate(neutralize(comments || "No comments."), opts.maxCommentsChars),
    "```",
    "",
    "Changed files:",
    "```text",
    truncate(neutralize(files || "No changed files."), opts.maxChangedFilesChars),
    "```",
    "",
    "Selected diff:",
    "```diff",
    truncate(neutralize(item.diff || "No diff."), opts.maxDiffChars),
    "```",
    "",
  ].join("\n");
}

function neutralize(value) {
  return String(value).replaceAll("<system", "< system").replaceAll("</system", "</ system");
}

function truncate(value, limit) {
  if (value.length <= limit) {
    return value;
  }
  return `${value.slice(0, limit)}\n[truncated ${value.length - limit} chars]`;
}

function oneLine(value) {
  return String(value).replace(/\s+/gu, " ").trim();
}

async function classify(side, contextPath, topics, item, opts) {
  const model = side === "reference" ? opts.referenceModel : opts.targetModel;
  const baseUrl = side === "reference" ? opts.referenceBaseUrl : opts.targetBaseUrl;
  const started = Date.now();
  try {
    const output = model === "mock" ? mockOutput(topics, item) : await callLocalpagerClassifier(baseUrl, model, contextPath, item, opts);
    const validation = validateOutput(output, topics);
    return {
      model,
      base_url: baseUrl || null,
      elapsed_ms: Date.now() - started,
      output,
      valid: validation.valid,
      errors: validation.errors,
    };
  } catch (error) {
    return {
      model,
      base_url: baseUrl || null,
      elapsed_ms: Date.now() - started,
      output: null,
      valid: false,
      errors: [error.message],
    };
  }
}

function mockOutput(topics, item) {
  const topicIds = topics.map((topic) => topic.id);
  const haystack = `${item.title}\n${item.body}\n${item.labels.join(" ")}\n${item.changedFiles.map((file) => file.filename).join(" ")}`.toLowerCase();
  const selected = topicIds.filter((topic) => haystack.includes(topic.replaceAll("_", " "))).slice(0, 2);
  return {
    topics_of_interest: selected,
    description: selected.length > 0 ? `Mock matched ${selected.join(", ")}.` : "Mock found no configured topic match.",
    caveats: ["mock_model"],
  };
}

async function callLocalpagerClassifier(baseUrl, model, contextPath, item, opts) {
  const command = resolveCommand(opts.classifierCommand);
  const args = [
    item.url,
    "--model",
    model,
    "--schema",
    resolvePath(opts.schema),
    "--prompt-template",
    resolvePath(opts.promptTemplate),
    "--github-context-file",
    contextPath,
  ];
  if (opts.topicTaxonomy) {
    args.push("--topic-taxonomy", resolvePath(opts.topicTaxonomy));
  }

  const env = {
    ...process.env,
    LOCALPAGER_AGENT_MAX_TOKENS: String(opts.maxTokens),
    LOCALPAGER_AGENT_TIMEOUT_MS: String(opts.timeoutMs),
  };
  if (baseUrl) {
    env.LOCALPAGER_AGENT_BASE_URL = baseUrl;
  }
  if (opts.contextWindow > 0) {
    env.LOCALPAGER_AGENT_CONTEXT_WINDOW = String(opts.contextWindow);
  }

  const { stdout, stderr, status, signal } = await runCommand(command, args, env, opts.timeoutMs);
  if (status !== 0) {
    const suffix = signal ? `signal ${signal}` : `exit ${status}`;
    throw new Error(`classifier ${suffix}: ${stderr || stdout}`.slice(0, 1000));
  }
  try {
    return JSON.parse(stdout.trim());
  } catch (error) {
    throw new Error(`classifier returned non-JSON stdout: ${stdout.slice(0, 500)} (${error.message})`);
  }
}

function runCommand(command, args, env, timeoutMs) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, {
      cwd: process.cwd(),
      env,
      stdio: ["ignore", "pipe", "pipe"],
    });
    let stdout = "";
    let stderr = "";
    const timer = setTimeout(() => {
      child.kill("SIGTERM");
    }, timeoutMs);
    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", (chunk) => {
      stdout += chunk;
    });
    child.stderr.on("data", (chunk) => {
      stderr += chunk;
    });
    child.on("error", (error) => {
      clearTimeout(timer);
      reject(error);
    });
    child.on("close", (status, signal) => {
      clearTimeout(timer);
      resolve({ stdout, stderr, status, signal });
    });
  });
}

function compareOutputs(reference, target) {
  if (!reference || !target) {
    return {
      exact_match: false,
      true_positive: 0,
      false_positive: 0,
      false_negative: 0,
      precision: 0,
      recall: 0,
      f1: 0,
    };
  }
  const ref = new Set(reference.topics_of_interest || []);
  const got = new Set(target.topics_of_interest || []);
  let tp = 0;
  for (const topic of got) {
    if (ref.has(topic)) {
      tp += 1;
    }
  }
  const fp = got.size - tp;
  const fn = ref.size - tp;
  const precision = got.size === 0 ? (ref.size === 0 ? 1 : 0) : tp / got.size;
  const recall = ref.size === 0 ? (got.size === 0 ? 1 : 0) : tp / ref.size;
  const f1 = precision + recall === 0 ? 0 : (2 * precision * recall) / (precision + recall);
  return {
    exact_match: setEquals(ref, got),
    true_positive: tp,
    false_positive: fp,
    false_negative: fn,
    precision,
    recall,
    f1,
  };
}

function summarize({ options: opts, repo, outputDir, startedAt: start, finishedAt, items, referenceRows, targetRows, resultRows }) {
  const totals = resultRows.reduce(
    (acc, row) => {
      acc.exact += row.comparison.exact_match ? 1 : 0;
      acc.tp += row.comparison.true_positive;
      acc.fp += row.comparison.false_positive;
      acc.fn += row.comparison.false_negative;
      return acc;
    },
    { exact: 0, tp: 0, fp: 0, fn: 0 },
  );
  const precision = totals.tp + totals.fp === 0 ? (totals.fn === 0 ? 1 : 0) : totals.tp / (totals.tp + totals.fp);
  const recall = totals.tp + totals.fn === 0 ? (totals.fp === 0 ? 1 : 0) : totals.tp / (totals.tp + totals.fn);
  const f1 = precision + recall === 0 ? 0 : (2 * precision * recall) / (precision + recall);
  const invalidReference = referenceRows.filter((row) => !row.valid).length;
  const invalidTarget = targetRows.filter((row) => !row.valid).length;
  const misses = resultRows.filter((row) => !row.comparison.exact_match).slice(0, 5);
  return [
    "# Localpager Classifier Experiment",
    "",
    `- Repository: ${repo.fullName}`,
    `- Item type: ${opts.itemType}`,
    `- Items: ${items.length}`,
    `- Started: ${start.toISOString()}`,
    `- Finished: ${finishedAt.toISOString()}`,
    `- Output directory: ${outputDir}`,
    `- Reference model: ${opts.referenceModel}`,
    `- Target model: ${opts.targetModel}`,
    "",
    "## Metrics",
    "",
    `- Exact topic-set match: ${formatRate(totals.exact, items.length)} (${totals.exact}/${items.length})`,
    `- Micro precision: ${precision.toFixed(3)}`,
    `- Micro recall: ${recall.toFixed(3)}`,
    `- Micro F1: ${f1.toFixed(3)}`,
    `- Invalid reference outputs: ${invalidReference}`,
    `- Invalid target outputs: ${invalidTarget}`,
    "",
    "## Files",
    "",
    "- `config.json`: run configuration with credentials omitted.",
    "- `schema.runtime.json`: schema after topic enum injection.",
    "- `items.jsonl`: fetched item summaries.",
    "- `reference-outputs.jsonl`: reference model outputs.",
    "- `target-outputs.jsonl`: target model outputs.",
    "- `per-row-results.jsonl`: row-level comparison records.",
    "- `contexts/`: rendered GitHub context blocks.",
    "- `prompts/`: final prompts sent to the model.",
    "",
    "## First Mismatches",
    "",
    misses.length === 0
      ? "No mismatches."
      : misses
          .map((row) => `- ${row.item.url}: precision ${row.comparison.precision.toFixed(3)}, recall ${row.comparison.recall.toFixed(3)}`)
          .join("\n"),
  ].join("\n");
}

function formatRate(count, total) {
  return total === 0 ? "0.000" : (count / total).toFixed(3);
}

function loadTopics(filePath) {
  const body = JSON.parse(readFileSync(filePath, "utf8"));
  const rawTopics = rawTopicEntries(body);
  if (!Array.isArray(rawTopics)) {
    throw new Error(`topic taxonomy does not contain topics or topics_of_interest enum: ${filePath}`);
  }
  const topics = rawTopics.map(normalizeTopic);
  const ids = topics.map((topic) => topic.id);
  if (ids.length === 0) {
    throw new Error(`topic taxonomy is empty: ${filePath}`);
  }
  if (ids.some((id) => !/^[a-z][a-z0-9_]{0,63}$/u.test(id))) {
    throw new Error(`topic taxonomy contains invalid topic id: ${filePath}`);
  }
  if (new Set(ids).size !== ids.length) {
    throw new Error(`topic taxonomy contains duplicate topic ids: ${filePath}`);
  }
  return topics;
}

function rawTopicEntries(body) {
  if (Array.isArray(body.topics)) {
    return body.topics;
  }
  if (body.topics && typeof body.topics === "object") {
    return Object.entries(body.topics).map(([id, entry]) => ({ id, ...entry }));
  }
  return body?.properties?.topics_of_interest?.items?.enum;
}

function normalizeTopic(topic) {
  if (typeof topic === "string") {
    return { id: topic, description: "", keywords: [] };
  }
  if (topic && typeof topic === "object" && typeof topic.id === "string") {
    return {
      id: topic.id,
      description: typeof topic.description === "string" ? topic.description : "",
      keywords: Array.isArray(topic.keywords)
        ? topic.keywords.filter((keyword) => typeof keyword === "string").slice(0, 1)
        : [],
    };
  }
  throw new Error(`invalid topic entry: ${JSON.stringify(topic)}`);
}

function renderSchema(schema, topics) {
  if (topics.length === 0) {
    return schema;
  }
  const rendered = structuredClone(schema);
  const topicsProperty = rendered?.properties?.topics_of_interest;
  if (!topicsProperty || topicsProperty.type !== "array") {
    throw new Error("schema must contain array property topics_of_interest");
  }
  topicsProperty.uniqueItems = true;
  topicsProperty.items = {
    type: "string",
    enum: topics.map((topic) => topic.id),
  };
  return rendered;
}

function renderPrompt(template, target, topics, githubContext) {
  const allowedTopicsJSON =
    topics.length > 0
      ? JSON.stringify(topics.map((topic) => topic.id), null, 2)
      : JSON.stringify("No configured taxonomy; any schema-valid snake_case topic is allowed.");
  const descriptions =
    topics.length > 0
      ? topics
          .map((topic) => {
            const details = [];
            if (topic.description) {
              details.push(truncateInline(topic.description, 80));
            }
            if (topic.keywords.length > 0) {
              details.push(`cues: ${topic.keywords.join(", ")}`);
            }
            return details.length > 0 ? `- \`${topic.id}\`: ${details.join(" ")}` : `- \`${topic.id}\``;
          })
          .join("\n")
      : "No topic taxonomy configured.";
  return template
    .replaceAll("__TARGET__", target)
    .replaceAll("__GITHUB_CONTEXT__", githubContext.trim())
    .replaceAll("__ALLOWED_TOPICS_JSON__", allowedTopicsJSON)
    .replaceAll("__TOPIC_TAXONOMY_JSON__", JSON.stringify({ topics }, null, 2))
    .replaceAll("__TOPIC_DESCRIPTIONS__", descriptions);
}

function truncateInline(value, maxLength) {
  return value.length <= maxLength ? value : `${value.slice(0, maxLength - 3)}...`;
}

function validateOutput(output, topics) {
  const errors = [];
  const allowed = new Set(topics.map((topic) => topic.id));
  if (!output || typeof output !== "object" || Array.isArray(output)) {
    return { valid: false, errors: ["output must be an object"] };
  }
  for (const key of Object.keys(output)) {
    if (!["topics_of_interest", "description", "caveats"].includes(key)) {
      errors.push(`unexpected field: ${key}`);
    }
  }
  if (!Array.isArray(output.topics_of_interest)) {
    errors.push("topics_of_interest must be an array");
  } else {
    if (output.topics_of_interest.length > 5) {
      errors.push("topics_of_interest must contain at most 5 topics");
    }
    if (new Set(output.topics_of_interest).size !== output.topics_of_interest.length) {
      errors.push("topics_of_interest must be unique");
    }
    for (const topic of output.topics_of_interest) {
      if (typeof topic !== "string") {
        errors.push("topics_of_interest entries must be strings");
      } else if (allowed.size > 0 && !allowed.has(topic)) {
        errors.push(`topic is not allowed: ${topic}`);
      }
    }
  }
  if (typeof output.description !== "string" || output.description.length === 0 || output.description.length > 500) {
    errors.push("description must be a non-empty string up to 500 chars");
  }
  if (!Array.isArray(output.caveats)) {
    errors.push("caveats must be an array");
  } else if (output.caveats.length > 5 || output.caveats.some((entry) => typeof entry !== "string" || entry.length > 240)) {
    errors.push("caveats entries must be strings up to 240 chars, max 5 entries");
  }
  return { valid: errors.length === 0, errors };
}

function itemSummary(item) {
  return {
    repo: item.repo,
    kind: item.kind,
    number: item.number,
    title: item.title,
    url: item.url,
    state: item.state,
    author: item.author,
    labels: item.labels,
    updated_at: item.updated_at,
  };
}

function writeJsonl(filePath, rows) {
  writeFileSync(filePath, rows.map((row) => JSON.stringify(row)).join("\n") + "\n");
}

function setEquals(left, right) {
  if (left.size !== right.size) {
    return false;
  }
  for (const value of left) {
    if (!right.has(value)) {
      return false;
    }
  }
  return true;
}

function redactedConfig(opts, outputDir) {
  return {
    ...opts,
    github_token_env: opts.githubTokenEnv,
    github_token_present: Boolean(opts.githubTokenEnv && process.env[opts.githubTokenEnv]),
    output_dir: outputDir,
  };
}

function resolvePath(filePath) {
  return path.resolve(process.cwd(), filePath);
}

function resolveCommand(command) {
  return command.includes("/") ? resolvePath(command) : command;
}

function timestamp(date) {
  return date.toISOString().replaceAll(":", "").replace(/\.\d{3}Z$/u, "Z");
}
