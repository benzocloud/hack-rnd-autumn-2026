#!/usr/bin/env bash
# Generate/check only the canonical Auth + Company/Errors consumer bindings.
set -euo pipefail
cd "$(dirname "$0")/../../.."
buf_bin="${BUF:-buf}"
lint_bin="${GOLANGCI_LINT:-golangci-lint}"
export GOTOOLCHAIN=go1.27.3
if [[ "$("$buf_bin" --version)" != "1.75.0" ]]; then
  printf '%s\n' 'Buf 1.75.0 is required' >&2
  exit 1
fi
case "$("$lint_bin" --version)" in
  *"version 2.15.0 "*) ;;
  *) printf '%s\n' 'golangci-lint 2.15.0 is required' >&2; exit 1 ;;
esac
if [[ "$#" -gt 1 || ("$#" -eq 1 && "${1:-}" != "--check") ]]; then
  printf '%s\n' 'Usage: generate.sh [--check]' >&2
  exit 1
fi
task_dir="$(mktemp -d "${TMPDIR:-/tmp}/saldo-auth-proto.XXXXXXXX")"
trap 'rm -rf -- "$task_dir"' EXIT
"$buf_bin" generate --template buf.auth-core.gen.yaml \
  --path proto/auth/v1/auth.proto --path proto/core/v1/company.proto \
  --path proto/core/v1/errors.proto --output "$task_dir"
"$lint_bin" fmt --config services/auth/.golangci.yml "$task_dir/services/auth/internal/gen"
if [[ "${1:-}" == "--check" ]]; then
  diff -ru services/auth/internal/gen "$task_dir/services/auth/internal/gen"
else
  cp -R "$task_dir/services/auth/internal/gen/." services/auth/internal/gen/
fi
