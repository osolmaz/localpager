#!/usr/bin/env bash
set -euo pipefail

minimum_total_coverage="85.0"
coverfile="$(mktemp)"
core_coverfile="$(mktemp)"

cleanup() {
  rm -f "$coverfile"
  rm -f "$core_coverfile"
}
trap cleanup EXIT

coverage_total() {
  go tool cover -func="$1" |
    awk '/^total:/ {print substr($3, 1, length($3)-1)}'
}

go test ./internal/... -coverprofile="$coverfile"

{
  head -n 1 "$coverfile"
  grep -E '/internal/(config/config|sources/(kind|stats)|notifier/policy)\.go:' "$coverfile"
} >"$core_coverfile"

total="$(coverage_total "$core_coverfile")"
printf 'Core decision/config coverage: %s%%\n' "$total"
awk -v total="$total" -v minimum="$minimum_total_coverage" \
  'BEGIN { exit !(total + 0 >= minimum + 0) }'
