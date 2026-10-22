# Local development

1. Read `AGENTS.md`, `.agent/instructions.md`, `.agent/decisions.md`, architecture docs.
2. Fetch GitHub Issue (scope, acceptance criteria, comments, dependencies, linked PRs).
3. Query GBrain for relevant findings when configured/available; verify against current code and accepted decisions. If disabled/unavailable, disclose that and continue without shared memory (ADR-010).
4. Check existing branches/PRs and contract dependencies. For Go services, consult `.agent/references/go-services.md` and name the applicable reference/pattern in the plan; disclose unavailable local reference checkouts.
5. Present a plan: scope, touched files, API/proto changes, tests, risks, unresolved questions. Wait for developer approval unless autonomy explicitly requested.
6. Implement in dedicated branch/worktree; run relevant tests, lint and Buf checks.
7. Record useful verified findings with Issue/PR/commit provenance if permitted.
8. Follow `.agent/workflows/git-conventions.md` for commit messages and publication. Developer decides when to push and open PR. After review, validate and fix findings; push updated SHA.

Prompt: `.agent/prompts/local-plan.md`.
