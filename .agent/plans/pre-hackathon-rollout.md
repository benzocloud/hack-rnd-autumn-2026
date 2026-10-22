# Pre-hackathon infrastructure rollout plan

**Status:** Planned; not deployed. **Owner:** Tech Lead. **Deadline:** Complete before hackathon starts. **Objective:** local Codex planning with shared context and independent server-side PR review working end-to-end.

## Phase 0 — Decisions and access (blocking)
- [ ] Confirm GitHub organization/repo, maintainers, PR conventions and human reviewers.
- [ ] Choose dedicated agent machine separate from production VPS; define threat model for trusted/untrusted PRs.
- [ ] Verify current Codex CLI support for ChatGPT Plus sign-in, noninteractive `codex exec`, terms, limits and session renewal. Decide Plus pilot vs separately billed API fallback; never share a personal account among developers.
- [ ] Decide trusted PR scope, draft PR review policy, expected concurrency and failure mode for missing GBrain/skills.
- [ ] Assign Issue owners for infrastructure work and document acceptance criteria.
**Exit:** access/security plan approved; no secret in repo.

## Phase 1 — Repo policies and GitHub
- [ ] Commit root `AGENTS.md`, `.agent/` policies, ADRs and prompts.
- [ ] Configure protected `main`: no direct push, required CI, human approval, no self-approval.
- [ ] Define Issue and PR templates; PR must explicitly link Issue and include acceptance criteria/test evidence.
- [ ] Provision least-privilege GitHub App/token for PR metadata/read and separate review publication.
**Exit:** sample Issue + PR can be retrieved with correct permissions.

## Phase 2 — GBrain shared memory
- [ ] Install and pin an audited GBrain release on persistent agent infrastructure; verify actual commands and MCP endpoint from that version's docs.
- [ ] Restrict access to private authenticated network (e.g. Tailscale plus TLS), not public unauthenticated MCP.
- [ ] Configure durable storage, backups, restore test, service health and restart policy.
- [ ] Create individual developer identities/grants if supported; reviewer credential must be **server-enforced read-only**.
- [ ] Test two local Codex clients can retrieve the same verified memory; test reviewer write denial.
- [ ] Establish provenance fields and no-secrets retention policy.
**Exit:** persistence/recovery, shared read, controlled write and review read-only verified.

## Phase 3 — Local developer Codex
- [ ] Install compatible Codex CLI for each developer; configure GitHub MCP and GBrain MCP without committing tokens.
- [ ] Ensure Codex reads `AGENTS.md` and `.agent/` from repository root.
- [ ] Smoke test `.agent/prompts/local-plan.md`: real Issue → GBrain search → proposed plan → wait for approval.
- [ ] Test graceful disclosure when GitHub/GBrain is unavailable.
**Exit:** at least two developers independently complete plan-only dry runs.

## Phase 4 — Skills and isolated review runner
- [ ] Identify exact upstream sources, licenses, instructions and revisions for `caveman-review`, `ponytail-review`, `cc-golang`.
- [ ] Audit and pin skills; install into a Codex-recognized skill location; prove each is callable in noninteractive review.
- [ ] Create dedicated non-root review identity, isolated ephemeral job workspace and network/secret restrictions.
- [ ] Authenticate Codex using the selected supported method; secure and rotate/revoke credentials as needed.
- [ ] Review runner gets read-only source, read-only GBrain, no production secrets or deployment privileges.
- [ ] Separate review analysis from GitHub comment publishing; validate output schema and SHA.
**Exit:** controlled test review executes all applicable skills and cannot write to GBrain or production.

## Phase 5 — GitHub Actions orchestration
- [ ] Implement CI: Go tests/lint, Python tests, frontend build/tests, Buf lint/breaking as applicable.
- [ ] Implement PR workflow: receive event → resolve current head/base SHA → fetch linked Issue/comments, PR/diff and CI status → checkout exact head SHA → validate required `.agent/` files → run isolated Codex review → publish summary/comments.
- [ ] Runner passes trusted metadata/context file paths and `.agent/prompts/pr-review.md` to `codex exec` (prefer stdin); model reads repo policies itself.
- [ ] Bind findings to reviewed SHA; cancel superseded jobs; deduplicate comments; limit concurrency/cost.
- [ ] Restrict `GITHUB_TOKEN` permissions; do not run untrusted PR scripts in privileged self-hosted environment. For forks, avoid secret-bearing PR jobs and use a safe trusted orchestration model.
- [ ] Fail visibly if required skill, credentials or mandatory context are missing; never publish false PASS.
**Exit:** synthetic PR produces SHA-specific feedback with evidence and status.

## Phase 6 — End-to-end rehearsal
- [ ] Open synthetic Issue with acceptance criteria and intentional bug.
- [ ] Local Codex reads ADRs, Issue, GBrain and proposes plan; developer approves.
- [ ] Push synthetic PR; verify runner receives Issue, PR, diff, CI, ADRs and GBrain context.
- [ ] Verify `caveman-review`, `ponytail-review`, `cc-golang` (Go PR) actually run; record pinned versions.
- [ ] Verify actionable finding with file/line and SHA; reviewer cannot edit, merge or write memory.
- [ ] Push fix; verify re-review at new SHA, previous finding resolved without duplicate spam.
- [ ] Simulate unavailable GBrain, missing skill, expired login and superseded SHA; check failure modes.
- [ ] Confirm human approval + CI gates before merge and no review-to-production path.
**Exit:** all acceptance tests green; capture screenshots/logs without secrets; freeze config for hackathon.

## Day-0 runbook
- [ ] Check runner service, disk, memory and queue.
- [ ] Check Codex auth status and remaining quota/limit where available.
- [ ] Check GBrain health, read-only reviewer credential and last successful backup.
- [ ] Check GitHub token/App access and Actions permissions.
- [ ] Check pinned skill discovery and a no-op review invocation.
- [ ] Confirm human fallback if automated review fails or Plus limits are exhausted.

## Deliverables
- `.agent/` policies, prompts and ADRs in repo.
- Working authenticated GBrain MCP with persistence/backup and read-only reviewer.
- Local Codex + GitHub/GBrain integrations.
- Isolated Codex review runner with audited/pinned skills.
- GitHub CI and SHA-bound review workflow, branch protection, documented fallback.
- Successful synthetic Issue → plan → PR → review → fix → re-review demo.

## Explicit non-goals before hackathon
- Automatic code implementation by central agent.
- Autonomous merge or deployment.
- Full production observability platform.
- Building optional Telegram bot ahead of MVP.
