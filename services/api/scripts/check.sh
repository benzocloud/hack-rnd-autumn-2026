#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
python_bin="${PYTHON:-python3}"
bash scripts/generate.sh --check
go build ./...
go test ./...
go test -race -coverpkg=./... -covermode=atomic -coverprofile=coverage.out ./...
"$python_bin" scripts/coverage.py coverage.out
go vet ./...
"${GOLANGCI_LINT:-golangci-lint}" fmt --diff
"${GOLANGCI_LINT:-golangci-lint}" run ./...
"$python_bin" -m ruff check scripts
"$python_bin" -m ruff format --check scripts
"$python_bin" -m unittest discover -s scripts -p test_data_contracts.py
"$python_bin" scripts/smoke.py --output artifacts/smoke.json
API_FINANCIAL_REPORT="$PWD/artifacts/financial.json" go test -count=1 ./internal/handler/http -run '^TestFinancialContractResponses$'
"$python_bin" scripts/contract.py artifacts/smoke.json artifacts/financial.json
