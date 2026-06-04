---
title: Classifier Prompt Provenance
author: Bob <dutifulbob@gmail.com>
date: 2026-06-04
---

# Classifier Prompt Provenance

Localpager keeps only the production classifier prompt and topic taxonomy in the
GitHub repo. Bulky dataset-generation prompts, per-row rendered DS4 prompts, and
prompt experiment metrics live in the Hugging Face dataset.

## Production Files

- Production Gemma/OpenClaw routing prompt:
  `examples/profiles/openclaw-routing-v8.prompt.md`
- Production topic taxonomy:
  `examples/profiles/openclaw-routing-topics.json`

## Dataset Provenance

The canonical dataset and prompt provenance live at:

```text
https://huggingface.co/datasets/dutifuldev/openclaw-classification-dataset
```

Important remote artifacts:

- Original DS4 generation prompt:
  `prompt-snapshots/2026-05-30-deepseek-localagent-generation-prompt.md`
- Representative original DS4 runtime prompt:
  `prompt-snapshots/2026-05-30-deepseek-localagent-runtime-prompt-0001.md`
- Original DS4 runtime template:
  `prompt-snapshots/2026-05-30-deepseek-localagent-runtime-template.md`
- Rendered DS4 runtime prompt example:
  `prompt-snapshots/2026-05-30-deepseek-localagent-runtime-example-0001.md`
- Rendered DS4 runtime prompts:
  `prompt-snapshots/2026-05-30-deepseek-localagent-runtime-prompts.jsonl`
- Gemma prompt candidates and metrics:
  `prompt-experiments/ds4-precision/`
- Final v8 prompt experiment:
  `prompt-experiments/ds4-precision/routing-intent-v8-fp-table.md`
- Full-dataset seed-vs-v7 comparison:
  `prompt-experiments/ds4-precision/full-638-20260601-093700/comparison-note.md`

## Rule

Do not copy per-row DS4 runtime prompts into Localpager. Keep Localpager focused
on the runtime configuration and link to Hugging Face for dataset provenance.
