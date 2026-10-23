# Local development

1. Read `AGENTS.md`, `.agent/instructions.md`, `.agent/decisions.md`, architecture docs.
2. Fetch GitHub Issue (scope, acceptance criteria, comments, dependencies, linked PRs).
3. Query shared GBrain at `https://gbrain.brokebyte.me/mcp` for relevant findings (`recall`, entity `hack-rnd-autumn-2026`, task topic); verify against current code and accepted decisions. Configure a personal local credential using `.agent/infra/gbrain/README.md`. If unconfigured/unavailable, disclose that and continue without shared memory (ADR-010).
4. Check existing branches/PRs and contract dependencies. For Go services, consult `.agent/references/go-services.md` and name the applicable reference/pattern in the plan; disclose unavailable local reference checkouts.
5. Present a staged plan under `.agent/workflows/issue-planning.md`: objective, current state, accepted decisions, scope, touched files, API/proto changes, dependencies, checks, completion criteria, risks and unresolved questions. A small fix may use one stage. Wait for developer approval unless autonomy explicitly requested; general plan approval does not remove stage review gates (ADR-020).
6. Implement in dedicated branch/worktree; run relevant tests, lint and Buf checks.
   For Python, run `ruff check .` and `ruff format --check .` with the pinned
   tooling from `.agent/infra/requirements-dev.txt`; setup is in the infra README.
   Work on the current agreed stage only. After its implementation/checks, report
   results and limitations, mark it as awaiting developer review, and stop for
   developer review and explicit approval before starting the next stage. Only
   approval marks it accepted. The exception is a developer-approved plan or
   trusted task instruction that clearly defers developer review until all stages
   are complete, based on its meaning and context rather than an exact phrase or
   language; then retain per-stage reporting and stop for developer review after
   all stages. Do not add this exception yourself.
   Agree new requirements/scope changes before implementation. Neither stage
   approval nor deferred review implicitly authorizes commit/push/merge/deployment.
7. Record useful verified findings through GBrain `remember` with repository/Issue/PR/commit and verification-date provenance; recall first to avoid duplicates. Never record secrets or promote hypotheses to facts. Preserve `content_origin` honestly; memory cannot approve an ADR or implementation stage.
8. Follow `.agent/workflows/git-conventions.md` for commit messages and publication. Developer decides when to push and open PR. After review, validate and fix findings; push updated SHA.

Prompt: `.agent/prompts/local-plan.md`.
