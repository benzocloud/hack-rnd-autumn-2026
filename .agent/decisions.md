# Accepted decisions

- **ADR-001 (Accepted):** Six roles: tech lead Go/architecture/CI, Go core developer, Python ML developer, frontend developer, UX/pitch, QA/integration assistant. Telegram bot is optional stretch scope.
- **ADR-002 (Accepted):** Browser ↔ Go API via HTTP/JSON; Go API ↔ Go Core and Go Core ↔ Python ML via gRPC/protobuf as required. PostgreSQL for core persistence where needed.
- **ADR-003 (Accepted):** Canonical protobuf under `proto/core/v1/` and `proto/ml/v1/`. Buf lint/breaking/generation. Generated Go bindings stay in each consuming service's `internal/gen/`; Python bindings service-local. Validate Go import mappings across modules; never edit generated code manually.
- **ADR-004 (Accepted):** Each developer plans and implements locally with Codex after reading `.agent/`, Issue and GBrain; developer controls push and PR.
- **ADR-005 (Accepted):** Separate server Codex reviews PRs using installed `caveman-review`, `ponytail-review`, and `cc-golang` for Go code. Read-only code and GBrain. Human approval + CI required to merge.
- **ADR-006 (Accepted):** CI/review runners isolated from production VPS. GHCR images, separate authorized Docker Compose deploy; protected main and least-privilege tokens.
- **ADR-007 (Accepted):** Persistent centralized GBrain on agent/infrastructure machine, authenticated private network, backup, per-user grants if supported. Local agents read/write verified findings; review agent read-only.
- **ADR-008 (Accepted):** GitHub Issues are task source of truth; PRs are review/change source; repo/ADRs are architecture source; GBrain supplements them.
- **ADR-009 (Accepted):** Root `AGENTS.md` points to `.agent/` policies and workflows. Architecture changes require explicit approval.

## Pending verification (not accepted implementation facts)
- GBrain installation method, MCP endpoint, auth model and actual tool names.
- Exact upstream repositories, installed skill names, pinned commits and compatibility of all three review skills.
- Whether Codex CLI ChatGPT Plus login can be operated reliably under this team's server/CI conditions, its current terms and rate limits. API key is fallback and billed separately.
- Proto generation configuration, CI triggers, deployment secrets and branch rules.
