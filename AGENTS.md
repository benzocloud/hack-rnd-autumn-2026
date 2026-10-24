# Saldo — Codex entrypoint

Read `.agent/instructions.md` and `.agent/decisions.md` before planning or reviewing.
Commit conventions: `.agent/workflows/git-conventions.md`.
Go service references (when Go is in scope): `.agent/references/go-services.md`.
Local work: `.agent/workflows/local-development.md`.
Issue scope and plans: `.agent/workflows/issue-planning.md`.
Automated review: `.agent/workflows/automated-review.md`.
Shared memory: `.agent/workflows/context-and-memory.md`.
Local GBrain MCP: `https://gbrain.brokebyte.me/mcp`; setup: `.agent/infra/gbrain/README.md`.
Infrastructure rollout: `.agent/plans/pre-hackathon-rollout.md`.
GitHub Issues track requirements; Git/ADRs govern code and architecture; GBrain is supplemental verified memory.

Frontend workspace: keep application source, public assets, tests, npm manifests,
Next.js configuration and local Docker files under `frontend/`. Run frontend npm
commands from `frontend/`; keep shared docs at `docs/`, deployment infrastructure
at `deploy/frontend/`, and GitHub Actions at `.github/workflows/`. This follows
the monorepo layout in `8925873656aeb2bfaa366519ac7b2fdf714bd131`.
