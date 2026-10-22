# Pre-hackathon infrastructure rollout plan

**Status:** In progress (2026-10-22). GitHub review, Issue planning and conversation workflows are active on main. Runner adapters/profile match PR #12 merge `12b8ff93b091e55fddc452dcfbba845f92092795`. Real Issue/plan/discussion/review cycles passed on disposable Issue #8 and PR #9; Russian review comments passed on PR #11. All 90 infrastructure tests passed on the server after PR #12, plus an installed-worker check without inference. GBrain is deferred and optional (ADR-010). Review requires `/agent-review` (ADR-011) and title-based skill routing (ADR-012). Main protection, full re-review/failure/auth rehearsals and ADR-021 remain open. **Owner:** Tech Lead. **Deadline:** Complete before hackathon starts. **Objective:** local Codex planning and independent server-side PR review working end-to-end, with shared memory added later.

Implementation, pins, evidence and continuation steps: `.agent/infra/README.md`.
Owner confirmed this is a test host and authorized installation while requiring
Fadwix infrastructure to remain untouched. No stop/reboot or Fadwix changes were
performed. GBrain hardware changes are deferred. Open work below is not represented as complete.

## Phase 0 — Decisions and access (blocking)
- [ ] Confirm GitHub organization/repo, maintainers, PR conventions and human reviewers.
- [ ] Choose dedicated agent machine separate from production VPS; define threat model for trusted/untrusted PRs.
- [ ] Verify current Codex CLI support for ChatGPT Plus sign-in, noninteractive `codex exec`, terms, limits and session renewal. Decide Plus pilot vs separately billed API fallback; never share a personal account among developers.
  - Plus pilot selected by owner. Browser login through Brave, login status and actual `codex exec` inference verified; terms/quotas and renewal rehearsal remain open.
- [ ] Decide trusted PR scope, draft PR review policy, expected concurrency and failure mode for missing GBrain/skills.
  - Accepted ADR-010/011/012: optional memory; explicit `/agent-review` requests; literal combined `INFRA/ML:` invokes Go skills, standalone `ML:` excludes them. Workflows are active with authorization, SHA checks and concurrency; trusted routing fixtures and ordinary live GitHub review passed. Hostile-input and sustained-operation verification remain open.
- [ ] Assign Issue owners for infrastructure work and document acceptance criteria.
**Exit:** access/security plan approved; no secret in repo.

## Phase 1 — Repo policies and GitHub
- [x] Commit root `AGENTS.md`, `.agent/` policies, ADRs and prompts to the rollout branch.
- [ ] Configure protected `main`: no direct push, required CI, human approval, no self-approval.
- [ ] Define Issue and PR templates; PR should include scope/acceptance criteria and test evidence; an Issue link is optional for review (ADR-016).
- [x] Provision scoped GitHub Actions tokens for metadata reads and separate comment/plan publication; model jobs receive no GitHub token.
**Exit:** sample Issue + PR can be retrieved with correct permissions.

## Phase 2 — GBrain shared memory (deferred follow-up; not a runner prerequisite)
- [ ] Install and pin an audited GBrain release on persistent agent infrastructure; verify actual commands and MCP endpoint from that version's docs.
  - Selected `garrytan/gbrain` 0.60.366.0, pinned source and targeted integration inspection recorded. Initialization blocked: guest reports QEMU CPU 2.5+ without `sse4_2`; no active GBrain service/database/tokens.
- [ ] Restrict access to private authenticated network (e.g. Tailscale plus TLS), not public unauthenticated MCP.
- [ ] Configure durable storage, backups, restore test, service health and restart policy.
- [ ] Create individual developer identities/grants if supported; reviewer credential must be **server-enforced read-only**.
- [ ] Test two local Codex clients can retrieve the same verified memory; test reviewer write denial.
- [ ] Establish provenance fields and no-secrets retention policy.
**Exit:** persistence/recovery, shared read, controlled write and review read-only verified.

## Phase 3 — Local developer Codex
- [ ] Install compatible Codex CLI for each developer; configure GitHub MCP and GBrain MCP without committing tokens.
- [ ] Ensure Codex reads `AGENTS.md` and `.agent/` from repository root.
- [ ] Smoke test `.agent/prompts/local-plan.md`: real Issue → available memory or explicit unavailable status → proposed plan → wait for approval.
- [ ] Test graceful disclosure when GitHub/GBrain is unavailable.
**Exit:** at least two developers independently complete plan-only dry runs.

## Phase 4 — Skills and isolated review runner
- [x] Identify exact upstream sources, licenses, instructions and revisions for `caveman-review`, `ponytail-review`, `cc-golang`.
  - `skills/skills.json` pins Caveman, Ponytail and `samber/cc-skills-golang`; actual Go entrypoint is `golang-how-to`, not a literal `cc-golang` skill. Files installed under review user's `.agents/skills`.
- [ ] Audit and pin skills; install into a Codex-recognized skill location; prove each is callable in noninteractive review.
  - Targeted instruction review and installation complete. Trusted Go fixture invoked `caveman-review`, `ponytail-review`, `golang-how-to` and secondary skills; expected `first.go:4` bug found with successful tool evidence. Go/gopls and full GBrain/PR compatibility remain unverified.
- [ ] Create dedicated non-root review identity, isolated ephemeral job workspace and network/secret restrictions.
  - Dedicated non-root identity and read-only command sandbox verified. AppArmor exception is scoped to our official bubblewrap binary; named workflow sandbox denies command access to auth, non-job host files and network; restricted SSH runs fixed unprivileged transient services under bounded resources. Synthetic checks passed; hostile-code and sustained-operation guarantees remain open.
- [ ] Authenticate Codex using the selected supported method; secure and rotate/revoke credentials as needed.
  - Plus login and inference work; auth file is private. Rotation/revocation rehearsal remains open.
- [ ] Review runner gets read-only source, read-only GBrain, no production secrets or deployment privileges.
- [x] Separate review analysis from GitHub comment publishing; validate output schema and SHA.
**Exit:** controlled test review executes all applicable skills and cannot write to GBrain or production.

## Phase 5 — GitHub Actions orchestration
- [ ] Implement CI: Go tests/lint, Python tests, frontend build/tests, Buf lint/breaking as applicable.
- [x] Implement PR workflow: `issue_comment.created` with exact `/agent-review` body on a PR → verify requester write/maintain/admin permission → resolve current head/base SHA → fetch linked Issue/comments, PR/diff and CI status → checkout exact head SHA → validate required `.agent/` files and title-based skill routing → run isolated Codex review → recheck head SHA → publish summary/comments. No PR-open, push, ready-for-review or comment-edit triggers.
- [x] Runner passes trusted metadata/context file paths and `.agent/prompts/pr-review.md` to `codex exec` (prefer stdin); model reads repo policies itself.
- [x] Bind findings to reviewed SHA; cancel superseded jobs; deduplicate comments; limit concurrency/cost.
- [x] Restrict `GITHUB_TOKEN` permissions; do not run untrusted PR scripts in privileged self-hosted environment. Fork PRs are rejected in this pilot.
- [ ] Fail visibly if required skill, credentials or mandatory context are missing; never publish false PASS.
  - Initial mandatory-memory failure is historical. Default-config fixtures and real GitHub reviews succeed without GBrain and disclose its absence. Required source/SHA/CI/skills still cannot be omitted; linked Issues are fetched when present (ADR-016). Full live missing-skill/expired-auth/stale-source rehearsals remain open.
**Exit:** synthetic PR produces SHA-specific feedback with evidence and status.

## Phase 5a — Issue assessment and plan automation (ADR-013)
- [x] Implement every-Issue static fix-boundary analysis and Issue report, without QA filters.
- [x] Implement authorized `/create-plan`: plan-only branch and draft PR linked with `Refs #N`.
- [x] Implement `/discuss <explicit plan amendment>` updates in the same managed plan PR; reject PRs containing implementation changes.
- [x] Separate read-only model analysis from GitHub write publisher; derive plan path from Issue number and reject stale source/Issue context.
- [x] Organize infrastructure by responsibility (ADR-014); update installers, workflow paths and safety checks.
- [x] Provision restricted SSH secrets; owner enabled organization policy and repository Actions PR creation was verified enabled, with default read token.
- [x] Activate on default branch and rehearse real Issue → scope report → draft plan PR → discussion (Issue #8 / PR #9).
- [ ] Implement ADR-021's shared Issue/PR context and close the Issue after publishing the plan PR link. Stages and review gates: `.agent/plans/shared-issue-dialogue.md`.

## Phase 6 — End-to-end rehearsal
- [ ] Open synthetic Issue with acceptance criteria and intentional bug.
- [ ] Local Codex reads ADRs, Issue and available GBrain context (or discloses absence), then proposes plan; developer approves.
- [ ] Push synthetic PR and verify it does not start agent review. Add `/agent-review` as an authorized PR comment; verify the runner receives Issue, PR, diff, CI, ADRs and available-memory status.
- [ ] Verify `caveman-review`, `ponytail-review`, `cc-golang` (Go PR) actually run; record pinned versions.
- [ ] Verify actionable finding with file/line and SHA; reviewer cannot edit, merge or write memory.
- [ ] Push fix; verify no automatic agent review. Add a new `/agent-review` comment; verify re-review at the new SHA, previous finding resolved without duplicate spam.
- [ ] Verify routing: `ML:` never invokes Go skills; literal `INFRA/ML:` requires them; `INFRA:` invokes them only for Go source/dependency changes. A standalone `ML:` with Go changes must report a scope conflict.
- [ ] Simulate unavailable GBrain, missing skill, expired login and superseded SHA; check failure modes.
- [ ] Confirm human approval + CI gates before merge and no review-to-production path.
**Exit:** all acceptance tests green; capture screenshots/logs without secrets; freeze config for hackathon.

## Day-0 runbook
- [ ] Check runner service, disk, memory and queue.
- [ ] Check Codex auth status and remaining quota/limit where available.
- [ ] If GBrain is enabled later, check health, read-only reviewer credential and last successful backup; otherwise report shared memory unavailable and continue.
- [ ] Check GitHub token/App access and Actions permissions.
- [ ] Check pinned skill discovery and a no-op review invocation.
- [ ] Confirm human fallback if automated review fails or Plus limits are exhausted.

## Deliverables
- `.agent/` policies, prompts and ADRs in repo.
- Deferred follow-up: authenticated GBrain MCP with persistence/backup and read-only reviewer.
- Local Codex + GitHub integration; GBrain is optional and deferred.
- Isolated Codex review runner with audited/pinned skills.
- GitHub CI and SHA-bound review workflow, branch protection, documented fallback.
- Successful synthetic Issue → plan → PR → review → fix → re-review demo.

## Explicit non-goals before hackathon
- Automatic code implementation by central agent.
- Autonomous merge or deployment.
- Full production observability platform.
- Building optional Telegram bot ahead of MVP.

## Current verification boundary

Installed infrastructure corresponds to main merge `12b8ff9`; GitHub and server
unit checks passed. Real Issue scope, plan creation/amendment, question,
discussion and explicit review were verified in disposable threads. The latest
large-plan fix passed an installed-worker check, not a new live model call.
Restricted SSH fixtures cover standalone ML and combined INFRA/ML routing;
Go/gopls, hostile input, full changed-SHA/failure/auth rehearsals and sustained
operation remain unverified. Main is not protected (GitHub API checked
2026-10-22). ADR-021 stages 1–2 are local work, not deployed; stage 3 remains
unstarted. See `.agent/infra/README.md` for exact evidence and limits.
