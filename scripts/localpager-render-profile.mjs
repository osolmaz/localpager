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

const topics = options.topicTaxonomy ? loadTopics(options.topicTaxonomy) : [];
const schema = JSON.parse(readFileSync(options.schema, "utf8"));
const promptTemplate = readFileSync(options.promptTemplate, "utf8");

mkdirSync(path.dirname(options.outputSchema), { recursive: true });
mkdirSync(path.dirname(options.outputPrompt), { recursive: true });
writeFileSync(options.outputSchema, JSON.stringify(renderSchema(schema, topics), null, 2) + "\n");
writeFileSync(options.outputPrompt, renderPrompt(promptTemplate, options.target, topics));

function parseArgs(args) {
  const parsed = {
    help: false,
    target: "",
    schema: "",
    promptTemplate: "",
    topicTaxonomy: "",
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
      "       --output-schema PATH --output-prompt PATH [--topic-taxonomy PATH]",
      "",
    ].join("\n"),
  );
  process.exit(status);
}

function loadTopics(filePath) {
  const body = JSON.parse(readFileSync(filePath, "utf8"));
  const rawTopics = Array.isArray(body.topics) ? body.topics : body?.properties?.topics_of_interest?.items?.enum;
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

function normalizeTopic(topic) {
  if (typeof topic === "string") {
    return { id: topic, description: "" };
  }
  if (topic && typeof topic === "object" && typeof topic.id === "string") {
    return { id: topic.id, description: typeof topic.description === "string" ? topic.description : "" };
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

function renderPrompt(template, target, topics) {
  const allowedTopicsJSON =
    topics.length > 0
      ? JSON.stringify(topics.map((topic) => topic.id), null, 2)
      : JSON.stringify("No configured taxonomy; any schema-valid snake_case topic is allowed.");
  const taxonomyJSON = JSON.stringify({ topics }, null, 2);
  const descriptions =
    topics.length > 0
      ? topics
          .map((topic) => (topic.description ? `- \`${topic.id}\`: ${topic.description}` : `- \`${topic.id}\``))
          .join("\n")
      : "No topic taxonomy configured.";
  return template
    .replaceAll("__TARGET__", target)
    .replaceAll("__ALLOWED_TOPICS_JSON__", allowedTopicsJSON)
    .replaceAll("__TOPIC_TAXONOMY_JSON__", taxonomyJSON)
    .replaceAll("__TOPIC_DESCRIPTIONS__", descriptions);
}
