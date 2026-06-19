#!/usr/bin/env node
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";

const options = parseArgs(process.argv.slice(2));

if (options.help) {
  usage(0);
}
if (!options.target || !options.schema || !options.promptTemplate || !options.outputSchema || !options.outputPrompt) {
  usage(2);
}

const schema = JSON.parse(readFileSync(options.schema, "utf8"));
const loadedTopics = options.topicTaxonomy ? loadTopics(options.topicTaxonomy) : [];
const topics = orderTopicsForSchema(loadedTopics, schema);
const promptTemplate = readFileSync(options.promptTemplate, "utf8");
const githubContext = options.githubContextFile
  ? readFileSync(options.githubContextFile, "utf8")
  : "GitHub context unavailable. Classify only from the target text and say so in caveats.";

mkdirSync(path.dirname(options.outputSchema), { recursive: true });
mkdirSync(path.dirname(options.outputPrompt), { recursive: true });
writeFileSync(options.outputSchema, JSON.stringify(renderSchema(schema, topics), null, 2) + "\n");
writeFileSync(options.outputPrompt, renderPrompt(promptTemplate, options.target, topics, githubContext));

function parseArgs(args) {
  const parsed = {
    help: false,
    target: "",
    schema: "",
    promptTemplate: "",
    topicTaxonomy: "",
    githubContextFile: "",
    outputSchema: "",
    outputPrompt: "",
  };
  for (let index = 0; index < args.length; index += 1) {
    const arg = args[index];
    if (arg === "--help" || arg === "-h") {
      parsed.help = true;
      continue;
    }
    if (arg === "--target") {
      parsed.target = requiredValue(args, ++index, arg);
      continue;
    }
    if (arg === "--schema") {
      parsed.schema = requiredValue(args, ++index, arg);
      continue;
    }
    if (arg === "--prompt-template") {
      parsed.promptTemplate = requiredValue(args, ++index, arg);
      continue;
    }
    if (arg === "--topic-taxonomy") {
      parsed.topicTaxonomy = requiredValue(args, ++index, arg);
      continue;
    }
    if (arg === "--github-context-file") {
      parsed.githubContextFile = requiredValue(args, ++index, arg);
      continue;
    }
    if (arg === "--output-schema") {
      parsed.outputSchema = requiredValue(args, ++index, arg);
      continue;
    }
    if (arg === "--output-prompt") {
      parsed.outputPrompt = requiredValue(args, ++index, arg);
      continue;
    }
    throw new Error(`unsupported argument: ${arg}`);
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

function usage(status) {
  const output = status === 0 ? process.stdout : process.stderr;
  output.write(
    [
      "usage: localpager-render-profile --target TARGET --schema PATH --prompt-template PATH",
      "       --output-schema PATH --output-prompt PATH [--topic-taxonomy PATH] [--github-context-file PATH]",
      "",
    ].join("\n"),
  );
  process.exit(status);
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
        ? topic.keywords.filter((keyword) => typeof keyword === "string")
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
  const taxonomyJSON = JSON.stringify({ topics }, null, 2);
  const descriptions =
    topics.length > 0
      ? topics
          .map((topic) => {
            const cues = topic.keywords.length > 0 ? ` Cues: ${topic.keywords.join(", ")}.` : "";
            return `- ${topic.id}: ${topic.description}${cues}`;
          })
          .join("\n")
      : "No topic taxonomy configured.";
  return template
    .replaceAll("__TARGET__", target)
    .replaceAll("__GITHUB_CONTEXT__", githubContext.trim())
    .replaceAll("__ALLOWED_TOPICS_JSON__", allowedTopicsJSON)
    .replaceAll("__TOPIC_TAXONOMY_JSON__", taxonomyJSON)
    .replaceAll("__TOPIC_DESCRIPTIONS__", descriptions)
    .replace(handlebarsVarPattern("target", true), target)
    .replace(handlebarsVarPattern("github_context", true), githubContext.trim())
    .replace(handlebarsVarPattern("allowed_topics_json", true), allowedTopicsJSON)
    .replace(handlebarsVarPattern("topic_taxonomy_json", true), taxonomyJSON)
    .replace(handlebarsVarPattern("topic_descriptions", true), descriptions)
    .replace(handlebarsVarPattern("target", false), target)
    .replace(handlebarsVarPattern("github_context", false), githubContext.trim())
    .replace(handlebarsVarPattern("allowed_topics_json", false), allowedTopicsJSON)
    .replace(handlebarsVarPattern("topic_taxonomy_json", false), taxonomyJSON)
    .replace(handlebarsVarPattern("topic_descriptions", false), descriptions);
}

function handlebarsVarPattern(name, raw) {
  const open = raw ? "\\{\\{\\{" : "\\{\\{";
  const close = raw ? "\\}\\}\\}" : "\\}\\}";
  return new RegExp(`${open}\\s*${escapeRegExp(name)}\\s*${close}`, "gu");
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/gu, "\\$&");
}

function orderTopicsForSchema(topics, schema) {
  if (topics.length === 0) {
    return topics;
  }
  const enumValues = schema?.properties?.topics_of_interest?.items?.enum;
  if (!Array.isArray(enumValues) || enumValues.some((value) => typeof value !== "string")) {
    return topics;
  }
  const topicById = new Map(topics.map((topic) => [topic.id, topic]));
  if (enumValues.length !== topics.length || enumValues.some((id) => !topicById.has(id))) {
    return topics;
  }
  return enumValues.map((id) => topicById.get(id));
}
