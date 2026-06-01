---
title: Classifier Benchmark Metrics
author: Bob <dutifulbob@gmail.com>
date: 2026-06-01
---

# Classifier Benchmark Metrics

Localpager classifier benchmarks compare topic sets. The benchmark does not
score `description` style except for schema validity.

## Main Numbers

- Exact topic-set match: target topics exactly equal reference topics.
- Micro precision: of all topics the target emitted, how many were also in the
  reference.
- Micro recall: of all reference topics, how many the target found.
- Micro F1: one combined precision/recall score.
- Invalid outputs: outputs that failed the runtime schema or local validator.

For this project, precision matters a lot. A false extra topic can page the
wrong maintainer domain. Recall also matters, but a missed notification is
usually easier to notice and repair than a noisy notification stream.

## How To Read Results

High precision and low recall means the prompt is too cautious.

High recall and low precision means the prompt is adding unnecessary labels.
This was Gemma 4's main failure mode during earlier local testing.

Low exact match with decent precision/recall often means the model is usually in
the right area but disagrees on secondary labels. Inspect `per-row-results.jsonl`
and the saved prompt for those rows.

Invalid outputs mean the model or endpoint is not respecting the schema. Fix the
model runner, schema settings, or prompt profile before treating the benchmark
score as meaningful.

## Reporting Format

Use this compact format in docs, PRs, and release notes:

```text
Rows: 100
Exact topic-set match: 0.820 (82/100)
Micro precision: 0.910
Micro recall: 0.780
Micro F1: 0.839
Invalid target outputs: 0
Main miss: extra secondary topics on broad infrastructure items
```

Always include the model names, prompt profile, topic taxonomy, schema path, and
dataset or repo sample used. Without those, the score is not reproducible.
