---
title: Classifier Prompt Provenance
author: Bob <dutifulbob@gmail.com>
date: 2026-06-04
---

# Classifier Prompt Provenance

Localpager keeps the production classifier prompt, output schema, and topic
taxonomy in the GitHub repo. Bulky dataset-generation prompts, per-row rendered
DS4 prompts, and prompt experiment metrics live in the Hugging Face dataset.

## Production Files

- Production OpenClaw routing prompt:
  `examples/profiles/openclaw-routing.prompt.hbs`
- Production output schema:
  `examples/profiles/openclaw-routing.schema.json`
- Production topic taxonomy:
  `examples/profiles/openclaw-routing-topics.json`
- These files are the self-contained v10/evalstate benchmark profile now used
  by Localpager.

## Dataset Provenance

The canonical dataset and prompt provenance live at:

```text
https://huggingface.co/datasets/dutifuldev/openclaw-classification-dataset
```

Important remote artifacts:

- Cutover prompt folder:
  `prompts/`
- Prompt folder README:
  `prompts/README.md`
- Original DS4 generation prompt:
  `prompts/2026-05-30-ds4-generation-prompt.md`
- Representative original DS4 runtime prompt:
  `prompts/2026-05-30-ds4-runtime-rendered-row-0001.md`
- DS4 runtime template rendered by the original generator from a placeholder seed row:
  `prompts/2026-05-30-ds4-runtime-template-placeholder.md`
- Rendered DS4 runtime prompt example:
  `prompts/2026-05-30-ds4-runtime-example-row-0001.md`
- Rendered DS4 runtime prompts:
  `prompts/2026-05-30-ds4-runtime-rendered-prompts.jsonl`
- DS4 generator script:
  `scripts/generate_deepseek_localagent_dataset.mjs`
- Gemma prompt candidates and metrics:
  `prompts/` for prompt files and `prompt-experiments/ds4-precision/` for
  metrics.
- Final v8 prompt experiment:
  `prompts/gemma-routing-intent-v8-fp-table.md`
- Final Localpager/Gemma production prompt:
  `prompts/localpager-openclaw-routing-v8-production.prompt.md`
- DS4 precision prompt experiments:
  `prompt-experiments/ds4-precision/`

## Rule

Do not copy per-row DS4 runtime prompts into Localpager. Keep Localpager focused
on the runtime configuration and link to Hugging Face for dataset provenance.
