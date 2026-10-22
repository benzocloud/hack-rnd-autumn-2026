# Local development

1. Read `AGENTS.md`, `.agent/instructions.md`, `.agent/decisions.md`, architecture docs.
2. Fetch GitHub Issue (scope, acceptance criteria, comments, dependencies, linked PRs).
3. Query GBrain for relevant findings; verify against current code and accepted decisions.
4. Check existing branches/PRs and contract dependencies.
5. Present a plan: scope, touched files, API/proto changes, tests, risks, unresolved questions. Wait for developer approval unless autonomy explicitly requested.
6. Implement in dedicated branch/worktree; run relevant tests, lint and Buf checks.
7. Record useful verified findings with Issue/PR/commit provenance if permitted.
8. Developer decides when to push and open PR. After review, validate and fix findings; push updated SHA.

Prompt: `.agent/prompts/local-plan.md`.
