#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../.."

export PYTHONPATH=prompt-optimizer/src
python3 -m unittest discover -s prompt-optimizer/tests

if [[ -f /home/bob/repos/openclaw-git-labels/data/splits/feedback300.jsonl \
  && -f /home/bob/repos/openclaw-git-labels/data/splits/pareto60.jsonl \
  && -f /home/bob/repos/openclaw-git-labels/data/splits/bench78.jsonl ]]; then
  python3 -m prompt_optimizer.cli summary >/dev/null
fi
