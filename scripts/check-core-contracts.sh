#!/usr/bin/env bash
# Generate/compile each consumer's local bindings without writing to its checkout.
set -euo pipefail
cd "$(dirname "$0")/.."

buf_bin="${BUF:-buf}"
if [[ "$("$buf_bin" --version)" != "1.75.0" ]]; then
  printf '%s\n' 'Buf 1.75.0 is required' >&2
  exit 1
fi
"$buf_bin" lint
"$buf_bin" format --diff --exit-code
"$buf_bin" build
"$buf_bin" breaking --against ".git#branch=${CONTRACT_BASE_REF:-main}"

task_dir="$(mktemp -d "${TMPDIR:-/tmp}/saldo-core-contracts.XXXXXXXX")"
trap 'rm -rf -- "$task_dir"' EXIT

"$buf_bin" generate --template buf.core.gen.yaml --output "$task_dir/core"
diff -ru services/core/internal/gen "$task_dir/core/services/core/internal/gen"

for consumer in api auth; do
  module="github.com/benzocloud/hack-rnd-autumn-2026/services/$consumer"
  target="$task_dir/$consumer"
  mkdir -p "$target"
  # Directory maps to the actual workspace; plugin mappings rewrite every Core
  # file and its Auth import, avoiding cross-module internal imports.
  python3 - "$target" "$module" > "$task_dir/generate.json" <<'PY'
import json
import sys

output, module = sys.argv[1:]
options = ["paths=source_relative"] + [
    f"Mcore/v1/{name}.proto={module}/internal/gen/core/v1"
    for name in ("common", "company", "ledger", "payments", "errors")
] + [f"Mauth/v1/auth.proto={module}/internal/gen/auth/v1"]
print(json.dumps({
    "version": "v2",
    "inputs": [{"directory": ".", "paths": ["proto/core/v1", "proto/auth/v1"]}],
    "plugins": [
        {"local": ["go", "run", plugin], "out": output + "/internal/gen", "opt": options}
        for plugin in (
            "google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12",
            "google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.2",
        )
    ],
}))
PY
  "$buf_bin" generate --template "$task_dir/generate.json"
  cat > "$target/go.mod" <<EOF_MODULE
module $module

go 1.27.3

require (
 google.golang.org/grpc v1.84.0
 google.golang.org/protobuf v1.36.12
)
EOF_MODULE
  (
    cd "$target"
    go mod tidy
    go test ./...
  )
done
