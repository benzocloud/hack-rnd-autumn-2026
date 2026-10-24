#!/usr/bin/env bash
# Generate/compile each consumer's local bindings without writing to its checkout.
set -euo pipefail
cd "$(dirname "$0")/.."

buf_bin="${BUF:-buf}"
contract_python="${CONTRACT_PYTHON:-python3}"
if [[ "$("$buf_bin" --version)" != "1.75.0" ]]; then
  printf '%s\n' 'Buf 1.75.0 is required' >&2
  exit 1
fi
"$buf_bin" lint
"$buf_bin" format --diff --exit-code
"$buf_bin" breaking --against ".git#branch=${CONTRACT_BASE_REF:-main}"

task_dir="$(mktemp -d "${TMPDIR:-/tmp}/saldo-core-contracts.XXXXXXXX")"
trap 'rm -rf -- "$task_dir"' EXIT
"$buf_bin" build --as-file-descriptor-set --output "$task_dir/contracts.binpb"
"$contract_python" scripts/check-contract-coverage.py "$task_dir/contracts.binpb"

"$buf_bin" generate --template buf.core.gen.yaml --output "$task_dir/core"
diff -ru services/core/internal/gen "$task_dir/core/services/core/internal/gen"

for consumer in api auth; do
  module="github.com/benzocloud/hack-rnd-autumn-2026/services/$consumer"
  target="$task_dir/$consumer/services/$consumer"
  "$buf_bin" generate --template "buf.$consumer-core.gen.yaml" --output "$task_dir/$consumer"
  cat > "$target/go.mod" <<EOF_MODULE
module $module

go 1.27.3

require (
 google.golang.org/grpc v1.84.0
 google.golang.org/protobuf v1.36.12
 golang.org/x/net v0.60.0
)
EOF_MODULE
  (
    cd "$target"
    go mod tidy
    go test ./...
  )
done
