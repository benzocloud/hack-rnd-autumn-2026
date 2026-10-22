# Infrastructure bootstrap prompt for Codex

You are preparing the hackathon infrastructure BEFORE development begins.

Read `AGENTS.md`, `.agent/instructions.md`, `.agent/decisions.md`, `.agent/plans/pre-hackathon-rollout.md` and related workflows.

First inspect the repository and server environment; do not assume dependencies or credentials exist. Produce a phased implementation plan with explicit checkpoints and approval gates. For each phase state files/services to create, validation commands, failure/rollback plan, security boundaries and acceptance criteria.

Implement only phases explicitly approved by the operator. Do not install unknown review skills without identifying and auditing their exact upstream source and pinned revision. Do not invent GBrain CLI flags or MCP tools: consult actual installed version/docs. Do not create production deployment access for the review runner.

Use a synthetic PR to prove Issue context, ADR loading, GBrain read-only retrieval, all required skill invocations, SHA-bound review publication, re-review, and branch protection behavior.
