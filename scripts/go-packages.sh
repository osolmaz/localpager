#!/usr/bin/env bash
set -euo pipefail

go list ./... | grep -v '/localpager-agent/'
