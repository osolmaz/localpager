#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../.."

export PYTHONPATH=prompt-optimizer/src
python3 -m unittest discover -s prompt-optimizer/tests

if [[ -f /home/bob/oc/openclaw-classification-dataset/ds4.jsonl \
  && -f /home/bob/scratch/shaun-openclaw-data-rows/gepa-good-60.rows.jsonl \
  && -f /home/bob/oc/openclaw-classification-dataset/prompts/localpager-openclaw-routing-v9.1-monologue-cap.hbs ]]; then
  python3 -m prompt_optimizer.cli summary >/dev/null
fi
