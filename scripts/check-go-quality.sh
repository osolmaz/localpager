#!/usr/bin/env bash
set -euo pipefail

cd .

packages="$(./scripts/go-packages.sh)"
slophammer_go="${SLOPHAMMER_GO:-$(go env GOPATH)/bin/slophammer-go}"
golangci_lint="${GOLANGCI_LINT:-$(command -v golangci-lint || true)}"
if [[ -z "$golangci_lint" ]]; then
  golangci_lint="$(go env GOPATH)/bin/golangci-lint"
fi

go vet $packages
"$golangci_lint" run
go test $packages
./scripts/check-go-coverage.sh
make build

"$slophammer_go" dry . --max-candidates 0
"$slophammer_go" crap .
"$slophammer_go" mutate . --scan
(
  cd reposhell
  "$golangci_lint" run
)
"$slophammer_go" check .
