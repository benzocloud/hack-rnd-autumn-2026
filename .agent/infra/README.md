# Agent infrastructure

Status **2026-10-22**: Codex Plus runner installed on `runner.brokebyte.me`.
Conversation/staged-plan support from
[PR #7](https://github.com/benzocloud/hack-rnd-autumn-2026/pull/7) is installed.
Current adapters and profile match [PR #12](https://github.com/benzocloud/hack-rnd-autumn-2026/pull/12)
merge `12b8ff93b091e55fddc452dcfbba845f92092795`; Russian review messages were
verified through GitHub in PR #11. All agent workflows are enabled; live checks and remaining limits
are recorded below. Update **2026-10-23**: shared GBrain is deployed on the separate
`gbrain.brokebyte.me` host; [setup, current pin and verification](gbrain/README.md).
Existing runner MCP remains disabled until separately connected. Fadwix containers, config, networks, ports,
Docker and kernel settings were not changed; no reboot was performed.

## Responsibility boundaries

| Directory | Owns | Entrypoints |
| --- | --- | --- |
| `bootstrap/` | Checksummed runtime installation and versions | `bootstrap.py`, `versions.json` |
| `runner/` | Restricted SSH, resource limits, sandbox, shared inference and transport | `install-workflow.py`, `review_dispatch.py`, `job.py`, `github_api.py`, `prepare-review.py` |
| `review/` | PR context, skill routing, report validation and advisory publication | `github_review.py`, `ssh_review.py`, `review_policy.py`, `smoke-review.py` |
| `planning/` | Issue assessment and plan-only branch/PR lifecycle | `github_plan.py`, `ssh_plan.py`, `plan_policy.py` |
| `conversation/` | Read-only Issue/PR questions, context and replies | `github_conversation.py`, `ssh_conversation.py`, `conversation_policy.py` |
| `skills/` | Pinned upstream instruction sources and installation | `install-skills.py`, `skills.json` |
| `gbrain/` | Dedicated shared HTTPS memory, local MCP setup, credentials and backups | `deploy.py`, `setup-local.py`, `backup.py`, one `hackrnd-gbrain.service` and backup units |
| `tests/` | Safety and protocol regression checks | `test_*.py` |

GitHub events and job permissions belong in `.github/workflows/`; human/agent
policy belongs in `.agent/workflows/` and accepted decisions. Runtime adapters
are deployed with the same `runner/`, `review/`, `planning/`, `conversation/` boundaries under
`/opt/hackrnd-agent/`. Binaries/releases and pinned skill sources remain under
`bin/`, `releases/`, `skill-sources/`. Mutable state belongs only in private
`/var/lib/hackrnd-review/` and future `/var/lib/hackrnd-gbrain/`.

## GitHub behavior

- `agent-review.yml`: only a new human PR comment with exact body `/agent-review`.
  Verify write/maintain/admin permission before execution/cancellation. Resolve
  current head/base SHA, linked Issues when present (`Refs`, `Closes`, `Fixes`, `Resolves`),
  full conversation/review/inline discussion, diff and CI metadata. Without an
  Issue, review from PR context and disclose missing requirements. Same-repository PRs only. No automatic review
  on PR opening, pushes, readiness or edited comments.
- Literal `INFRA/ML:` requires Go skills. Standalone `ML:` forbids them; Go
  changes under that title are a scope conflict. For `INFRA:`/other titles,
  actual Go source/module/workspace changes determine Go routing. All scopes
  use `caveman-review` and `ponytail-review`.
- `issue-planning.yml`: every new Issue receives a static fix-boundary report.
  An announcer first posts one scope-analysis status comment. Its publisher edits
  that comment with the report or a failure/cancellation status and Actions link,
  including when analysis or artifact retrieval fails.
  Exact `/create-plan` in the Issue creates `runner/fix/<short_description>/issue_N` with only
  `.agent/plans/issue-N-fix.md` and a draft PR. `/discuss Поправь план: <changes>` in that
  managed PR revises only the plan and edits one progress comment with its outcome. Both commands require a human with write,
  maintain or admin permission. No implementation, approval, merge or deployment.
  Plan PRs use `Refs #N`, avoiding automatic Issue closure when a plan merges.
  Plan commits and draft PR titles use `feat:` with the Issue number and a short
  English fix description under `.agent/workflows/git-conventions.md`. Commit
  bodies identify the requester/command and link directly to the originating
  Issue/PR comment via `Request:`; the same link guards against duplicate commits.
- `infra-checks.yml`: run stdlib safety tests for infrastructure changes on
  pushes/PRs. Application Go/Python/frontend/Buf CI awaits actual project code.
- `agent-conversation.yml`: `/question <question>` and ordinary `/discuss <message>`
  in open Issues/same-repository PRs. Read descriptions, all conversation/review/
  inline comments/replies and directly linked Issues, with complete pagination.
  Recheck source and discussion before editing the original progress comment.
  `/question` selects `gpt-6-luna`/low and supplies bounded context/source in the
  prompt with shell tools disabled; tool-use events invalidate the answer.
  `/discuss` selects `gpt-6.1-sol`/low for read-only source inspection. Its explicit
  managed-plan amendments route exclusively through `issue-planning.yml`.
  The old plan-specific slash command is removed. Replies include SHA-bound
  source links and measured CLI token usage when present. Both use the common
  Plus Codex/ChatGPT Work quota; no separate free ChatGPT Chat mode is claimed.
  Conversation publishers have only contents read and comment write permissions.

Installed with PR #12: `/question` supplies the complete visible text of
a managed plan without its encoded state footer, after checking that the state
reproduces the original Markdown exactly. The omission is marked in the source
excerpt and disclosed in the published reply. Visible line numbers and the
original file are preserved; legacy plans stay unchanged. Invalid/missing state,
a plan beyond the existing 128 KiB file bound, visible content over 24,000
characters or total selected source over 80,000 JSON characters still fail with
`/discuss` suggested. No tools, model fallback, protocol fields or write authority
are added. The worker and publisher were updated together. All 90 infrastructure
tests also passed on the server, and the installed worker passed a projection
check as the unprivileged reviewer without model inference. A real GitHub
question about a large managed plan has not yet been repeated after this fix.

Local ADR-021 stages 1–3, not deployed: questions, ordinary discussion, Issue
assessment, plan creation/amendment and explicit PR review rebuild the Issue
dialogue with related same-repository PRs, including closed history.
Only formal references in a PR description or its managed-plan marker define
its root; comment mentions remain supplementary context. Multiple distinct roots
fail visibly. Candidates come from the complete Issue timeline and are checked
against their current descriptions and head repositories. More than ten related
PR candidates or existing pagination/context limits fail without silent omission.
Authors, timestamps, review states and inline reply locations are retained.
The request's code SHA and reply location stay unchanged; related history is
rechecked before publication, with links to the root Issue and included PRs.
Plan amendments can read a closed Issue through its open managed PR; new commands
in the closed Issue or closed PR remain rejected. Planning and review publication
recheck the full history as well as existing source/destination guards. Context
is bounded at 160,000 characters; reactions and display metadata alone do not
invalidate a result. Common validation lives in `review/review_policy.py`, while
the GitHub-only collector and substantive comparison stay in `runner/github_dialogue.py`.
These stages change all analysis contexts: update GitHub orchestration, workers
and policies together before activation.
`/create-plan` now links the verified open plan-only PR before closing its Issue.
The fixed file must exist and be nonempty at the PR head; deleted plans fail.
One stable bot handoff comment is reused or repaired, with an exact clickable PR
link and a clear statement that closure is plan handoff, not a completed fix.
If the PR exists after a link/closure failure, a new authorized command can retry
without another model call or Git write. Closed, ambiguous or implementation PRs
leave the Issue open. A branch created before a failed PR creation is preserved;
without a managed PR it still requires manual recovery, never an overwrite.
The publish job now runs for successful existing-plan preparation even when
inference was skipped; no result artifact is downloaded or consumed in that path.
Only the existing planning publisher with `issues: write` closes the Issue.
Local verification: all 124 infrastructure tests, Ruff lint/format, actionlint
and whitespace checks passed, with model/SSH/GitHub analysis and publication
calls mocked.

GitHub-hosted context jobs use read tokens. The fixed SSH invocation receives
no GitHub token. A separate GitHub-hosted publisher has only the write permissions
needed for its action. Review publication edits the original bot progress comment with its report or
failure/cancellation/stale status; it never approves a PR. A cancelled entire
workflow or publisher/API failure may prevent the final status update.
Stale head/base/title or Issue/source changes abort publication. Branch revisions
are fast-forward only, never force-pushed. Model output cannot select the plan
write path. Duplicate requests are guarded; an existing plan PR is not replaced.

Repository secrets `AGENT_REVIEW_SSH_KEY` and `AGENT_REVIEW_KNOWN_HOSTS` have been
provisioned. Actions PR creation is enabled at the repository after the owner
lifted the organization restriction; default token permissions remain read.
The workflows must land on the default branch before Issue/comment events work.
GitHub does not trigger other workflows from `GITHUB_TOKEN`-created PRs: CI on a
plan PR is not automatically proven. Human approval and required CI still govern
merge. Branch protection is pending configuration/plan availability.

## Installed runner and verified boundaries

- Checksummed Codex CLI `0.163.0`, official code-mode host and bubblewrap.
  The owner's ChatGPT Plus login through Brave and actual noninteractive inference
  succeeded; no OpenAI API billing/key is configured.
- Dedicated `hackrnd-review` and `hackrnd-gbrain` identities, private `0700`
  state. Neither belongs to Docker/sudo groups. Reviewer has one exact sudo rule:
  root-owned `/usr/bin/python3 -Es /opt/hackrnd-agent/runner/review_dispatch.py`.
  It accepts only four fixed review/planning SSH commands, then runs a transient
  service as the unprivileged reviewer. Arbitrary SSH commands are rejected.
- `hackrnd-review.slice`: 75% of one CPU, 512 MiB MemoryHigh, 1 GiB MemoryMax,
  128 MiB swap, 128 tasks. Transient jobs have an 18-minute runtime limit and
  group-wide cancellation. One active model job; overlapping work fails visibly
  rather than exceeding the small test host budget. Submit a fresh command after
  the previous job completes; monitor Actions failures for simultaneous Issues.
- Private temporary job directories are removed on normal completion/failure.
  Archives are bounded and reject traversal, links, devices and executable modes.
  Model starts in a trusted policy directory; PR configs stay below `source/`.
  Commit rules and tracked Go reference guidance are included in trusted context;
  ignored local `references/` checkouts are not shipped to the runner.
  PR build/install/test scripts and hooks must not be executed during analysis.
- Named Codex `workflow` permissions deny the root filesystem except the job,
  minimal runtime and pinned skill files; deny `.codex`, `/run` and `/proc` to
  model commands. Command network is disabled. Probes confirmed workspace reads
  and auth-file denial without exposing its contents. ML profile additionally
  denies the Go skill source tree and disables Go skill catalog entries.
- Root-owned bubblewrap has a dedicated AppArmor user-namespace profile; no
  global AppArmor/sysctl changes. The model's own authentication/network remain
  outside the command sandbox. This is a constrained owner-account pilot,
  not a claim of complete hostile-code isolation on a shared host.
- Required pinned SKILL.md content is supplied explicitly to the model. Loading
  receipts record its SHA-256; the report lists applied entrypoints. Receipts
  prove instructions were supplied, not compliance with every recommendation.
  A synthetic `INFRA/ML:` review found the intended `api/first.go:4` panic,
  reporting all three required entrypoints. The standalone ML fixture found the
  expected divide-by-zero without Go skills. These are synthetic SSH checks,
  not a real GitHub PR rehearsal. The synthetic Issue scope adapter also
  returned the intended single-file fix boundary and acceptance criteria. Go/gopls are absent; no source tests/builds ran.

GBrain is optional/disabled (`enabled=false`, `required=false`). The guest reports
QEMU CPU 2.5+ without `sse4_2`, which Bun needs on x64. Physical AMD model/SMT does
not establish guest feature exposure. GBrain has no active service, DB, identities
or backup timer. This does not block the Codex-only runner (ADR-010). Any future
VM CPU change or restart needs separate scheduling with Fadwix's owner.

## Reproduce installation

Requires root on Ubuntu x86_64, Python 3, Git, systemd and `apparmor_parser` on
Ubuntu with restricted namespaces. Execute local commands from the repo root.
Scripts operate only this installation's `hackrnd-*` namespace.

```sh
ssh root@runner.brokebyte.me 'install -d -m 700 /root/hackrnd-bootstrap'
scp -r .agent/infra root@runner.brokebyte.me:/root/hackrnd-bootstrap/
ssh root@runner.brokebyte.me 'python3 /root/hackrnd-bootstrap/infra/bootstrap/bootstrap.py'
ssh root@runner.brokebyte.me 'python3 /root/hackrnd-bootstrap/infra/runner/prepare-review.py'
ssh root@runner.brokebyte.me 'python3 /root/hackrnd-bootstrap/infra/skills/install-skills.py'
```

Authenticate as `hackrnd-review`, not root. Device login first requires account
security settings to allow device sign-in; browser OAuth can use an SSH forward
from local port 1455 to runner port 1455, closed after login. Always change into
the review jobs directory before running Codex as that identity.

```sh
ssh root@runner.brokebyte.me 'cd /var/lib/hackrnd-review/jobs && runuser -u hackrnd-review -- /opt/hackrnd-agent/bin/codex login status'
```

Create a dedicated Ed25519 key locally; copy **only its public key** to a private
root staging directory on the runner. Pass that public key to the installer:

```sh
ssh root@runner.brokebyte.me 'python3 /root/hackrnd-question-12b8ff93b091e55fddc452dcfbba845f92092795/candidate/.agent/infra/runner/install-workflow.py /root/hackrnd-bootstrap/review-key.pub'
```

The installer updates root-owned adapters, scoped sudoers, key restriction,
workflow profile and the slice unit. It preserves auth and rejects different
existing authorized keys. It reloads systemd units without restarting Fadwix.
Store the private key in GitHub `AGENT_REVIEW_SSH_KEY`, and a host key verified
through a trusted channel in `AGENT_REVIEW_KNOWN_HOSTS`. Never commit private
keys, tokens or OAuth state. Never use an unverified keyscan as the trust source.

For the conversation rollout, install `conversation/`, updated `runner/job.py`,
`runner/review_dispatch.py`, the permission profile and both planning adapters
together, before enabling the new default-branch workflows. Use the existing
public key; hold `/var/lib/hackrnd-review/review.lock` while updating adapters,
and back up the prior root-owned adapters/profile for rollback. The new dispatcher
allowlist has six fixed commands; the four-command pilot was the
previous state. Verify selected model access, shell-tool exclusion and source
read/auth denial as the unprivileged user before publishing test commands.
The versioned staging path above is the verified PR #12 bundle, not a moving
checkout. Stage future versions separately and coordinate both protocol sides.
The live rollout below used the existing public key and preserved Plus auth.

## Pins and source references

`bootstrap/versions.json` pins release URLs/archive SHA-256; `skills/skills.json`
pins upstream commits/licenses. No upstream installer or hooks are executed.

| Component | Pin/source |
| --- | --- |
| GitHub Actions | [checkout v7.0.1](https://github.com/actions/checkout/releases/tag/v7.0.1), [upload-artifact v7.0.2](https://github.com/actions/upload-artifact/releases/tag/v7.0.2), [download-artifact v8.0.2](https://github.com/actions/download-artifact/releases/tag/v8.0.2); full commit pins in workflows, native Node.js 24 runtime verified in each `action.yml` |
| Codex + companions | [OpenAI 0.163.0](https://github.com/openai/codex/releases/tag/rust-v0.163.0) |
| Bun (deferred path) | [1.4.3](https://github.com/oven-sh/bun/releases/tag/bun-v1.4.3) |
| GBrain (dedicated host) | [garrytan/gbrain 0.60.409.0](https://github.com/garrytan/gbrain/releases/tag/v0.60.409.0), `3deb0563bb6215c83613d903702f378c2c9be798`; artifact SHA-256 in `gbrain/versions.json`. Older runner bootstrap artifacts are historical, inactive |
| caveman-review | JuliusBrussee/caveman, Apache-2.0; exact SHA in `skills/skills.json` |
| ponytail-review | DietrichGebert/ponytail, MIT; exact SHA in `skills/skills.json` |
| Go suite | samber/cc-skills-golang, MIT; `golang-how-to` plus applicable secondary skills |

Plan shorthand `cc-golang` resolves to that Go suite, not a skill of that name.
Context7 was used for GBrain/Codex/Bun and GitHub API documentation; pins were
checked against official source/releases. References: [Bun CPU requirements](https://bun.com/docs/installation#cpu-requirements),
[GBrain install](https://github.com/garrytan/gbrain/blob/c94425368cd5b9eb321a724cfd462e6b52bdd4d2/docs/INSTALL.md),
[MCP deployment](https://github.com/garrytan/gbrain/blob/c94425368cd5b9eb321a724cfd462e6b52bdd4d2/docs/mcp/DEPLOY.md),
[Git refs, force=false](https://docs.github.com/en/rest/git/refs),
[Actions organization policy](https://docs.github.com/en/organizations/managing-organization-settings/disabling-or-limiting-github-actions-for-your-organization).

## Shared memory and recovery

Canonical deployment: [gbrain/README.md](gbrain/README.md),
`https://gbrain.brokebyte.me/mcp` on `193.233.244.17`, independent of the runner.
Only the dedicated host was provisioned; no Fadwix services were changed.
There is one server unit and one native backup job/timer, without a parallel
`standalone/` configuration or legacy `configure.py`.

GBrain is a supplemental service with `required=false` in local Codex.
The runner's old diagnostic artifacts and disabled MCP configuration are not
this deployment; do not run the old `bootstrap.py --with-gbrain` path to enable it.
Developer and reviewer credentials are separate; see the runbook for private
file locations, actual acceptance checks and recovery limitations.

## Extension rollout

On 2026-10-22 the runner's `planning/plan_policy.py` and `planning/ssh_plan.py`
were synchronized after Issue #3 exposed an old report schema without
`branch_description`. Only those two root-owned adapters were replaced under
the review lock; originals are backed up under
`/root/hackrnd-plan-fix.9peB65eE/backup`. No service restart was needed.
A read-only replay of Issue #3 at source
`59c7aea5a2051e836f8bcdde2b36ac7bc19ce4c7` through the fixed unprivileged
dispatcher returned a report with `branch_description` and passed local
source/Issue/schema validation. This was an SSH replay, separate from the
original failed GitHub run.
Issue scope progress comments are enabled by the default-branch version of
`issue-planning.yml`; the later PR #7 rehearsal verified them on Issue #8.

The new planner report adds a validated `branch_description`; its publisher builds
`runner/fix/<short_description>/issue_N`. Update the GitHub orchestration and
root-owned runner adapters together. Do not deploy only one side and
claim the new flow operational. After merge, copy the updated infra directory to
private staging and run `runner/install-workflow.py` with the existing public key,
then rehearse scope, plan creation, discussion and review on test objects.

## Checks, verified activation and rollback

### Staged-plan protocol: verification and coordinated rollout

Scope assessments keep the compact `steps` report. Plan creation and amendments
require `stages` and `review_mode`, with numbered stages, dependencies, affected
files, proposed actions/checks and completion criteria. New stages are unstarted;
the default requires developer review after each implementation stage. Checks in
generated plans are proposals, never evidence that tests ran.

The plan Markdown includes a versioned structured-state footer. Amendments read
it from the fixed plan path at the authorized PR head, preserve stage history
and review mode, and reject model-invented status changes. The publisher fetches
the same Git file again before writing; a substituted artifact cannot authorize
different history. Rendered text and state must agree. A manual prose-only edit
does not update structured state and stops the next automatic amendment; repair
both together rather than removing metadata to bypass validation. A separate
format marker and stage headings distinguish structured plans from legacy text:
losing the state footer stops revision rather than resetting accepted stages.
Model-provided text is escaped, multiline list items remain inside their parent
item, and stage titles/results must be single lines. Source references stay
clickable; model text cannot introduce checked tasks or additional headings.
Legacy plans
without structured state migrate with unstarted stages and per-stage review.
New status approvals and review-mode changes are not inferred from comments;
only previously recorded Git state is carried forward. This is not an automated
implementation or developer-approval system.

The report remains bounded at 45,000 JSON characters; the fixed Markdown file
limit is 128 KiB to accommodate rendered text and structured state. No second
repository file or external state database is added.

Local adapter-cycle tests cover creation, revision, preservation of a previously
recorded final-review mode and accepted stage, duplicate plan detection without
another model invocation, and rejection of changed source before Git writes.
GitHub HTTP, SSH transport, model inference and the worker workspace are mocked;
these tests do not prove Linux sandboxing, installed adapters, model access or
live GitHub publication. Real HTTP and subprocess calls are blocked in the cycle
tests. Ordinary infrastructure CI on push runs only unit tests and lint, without
model tokens. The separate manually run model fixture does consume usage.

Future protocol activations require a separate approved rollout:

1. Suspend the three agent workflows during the protocol transition; ordinary
   infrastructure CI can continue. Record current publisher/runner revisions and
   back up the root-owned adapters and profile.
2. Update `planning/plan_policy.py` and `planning/ssh_plan.py` on the runner under
   the existing review lock, together with the conversation adapters/profile
   required above. The GitHub side needs matching `planning/github_plan.py` and
   `planning/plan_policy.py`. Old and new plan response formats are intentionally
   incompatible; do not leave one side active with the other version.
3. Before re-enabling commands, verify the synchronized revisions, fixed command
   routing and existing auth/sandbox boundaries as the unprivileged reviewer.
   Any real inference checks consume model quota and remain manual.
4. Rehearse a disposable Issue, plan creation, a plan amendment and explicit PR
   review. Confirm stage formatting, the same plan path/branch, one amended status
   comment and exact source links. Verify a push does not start model analysis.
   Re-enable normal requests only after the real results are inspected.

### Verified PR #7 rollout, 2026-10-22

The owner explicitly approved merge and installation. The three agent workflows
were suspended during the transition; ordinary infrastructure CI remained active.
The versioned candidate was transferred from a Git archive, its checksum verified,
and all 27 Python sources parsed on the server before installation. The installer
ran under the existing review lock with the existing public key. All eight
installed adapter hashes and the workflow profile match the candidate revision.
Backup, file ownership/modes and activation provenance are retained privately in
`/root/hackrnd-rollout-24af4e55ea097496bef3cea71350eb344c4a7867/`.
OAuth credentials were not included in that backup. No Fadwix container was
recreated or restarted; container IDs and uptime remained unchanged.

Both selected models answered trusted fixtures through the actual unprivileged
dispatcher: `gpt-6-luna` for questions without tool events and `gpt-6.1-sol` for
discussion. The live unit used `hackrnd-review`, no capabilities,
`NoNewPrivileges=yes`, strict system protection and the scoped writable directory.
Plus login remained valid; the review user has no supplementary groups.

Live GitHub checks used disposable [Issue #8](https://github.com/benzocloud/hack-rnd-autumn-2026/issues/8)
and [PR #9](https://github.com/benzocloud/hack-rnd-autumn-2026/pull/9):

- Automatic scope assessment edited the original progress comment into its report.
- `/question` answered from Issue context and edited the same status comment.
- `/create-plan` created one draft PR on `runner/fix/document_agent_commands/issue_8`,
  containing only `.agent/plans/issue-8-fix.md`, with one unstarted stage and
  per-stage developer review. The Issue received the PR link.
- An explicit `/discuss` amendment updated the same branch/file, preserved the
  stage status/review mode, used a `feat:` commit and a request-comment permalink,
  and edited the same progress comment. Stored state validated after both writes.
- Ordinary `/discuss` in the PR recalled markers from its linked Issue and prior
  PR discussion without changing the plan SHA. All 11 source-link ranges in the
  amended plan and the answer's line reference were checked against source.
- Only the explicit `/agent-review` request ran review. Its original status comment
  became a SHA-bound, complete advisory report applying `caveman-review` and
  `ponytail-review`, with no Go skills. No findings were reported. This was not
  approval: the bot-created plan PR had no CI checks, and no repository tests ran
  in the static reviewer. The ordinary discussion's review jobs were skipped.

Both test objects were closed manually without merging the plan; its branch is
retained as evidence. The merge push ran only infrastructure CI, successfully,
without model analysis. All three agent workflows were re-enabled. These manual
inference checks consumed subscription usage; the 85 local/CI unit tests did not.

The subsequent PR #10 language update replaced only `review/ssh_review.py` and
`planning/plan_policy.py` under the same lock, with atomic file replacement and
backups in `/root/hackrnd-language-75ba614ba204ef6524a3ac5b719d28949db87fac/`.
The remaining adapters/profile stayed unchanged; all installed hashes match that
revision. [Test PR #11](https://github.com/benzocloud/hack-rnd-autumn-2026/pull/11#issuecomment-6350051001)
verified Russian start/final labels and model-written checks/limitations in the
same edited comment. Its review workflow passed; the report was static and the
test PR had no CI checks. The PR was closed without merge and its disposable
branch deleted. No service restart or Fadwix changes were needed.

The PR #12 fix replaced only `conversation/ssh_conversation.py` atomically under
the review lock. All eight installed adapters and the workflow profile match
merge `12b8ff93b091e55fddc452dcfbba845f92092795`. The previous worker and hash,
ownership/mode, container and verification receipts are retained under
`/root/hackrnd-question-12b8ff93b091e55fddc452dcfbba845f92092795/`.
All 90 unit tests passed on the server with mocked external/model calls. The
installed worker ran as `hackrnd-review`: a 39,324-character fixture became
6,236 characters, preserving visible lines, the source file and one canonical
path for `./` and `/./` aliases. No inference was called. All nine Fadwix
containers retained their IDs, start times and restart counts; no restart or
host-wide configuration change was performed. Main's merge CI also passed.

Observed limits still require follow-up: the initial model report cited worker
`context.json` as a repository file, producing an invalid link; a requested plan
amendment removed it, but the formatter does not verify source existence. Also,
the initial one-stage plan occupied 35,989 characters including encoded state,
exceeding `/question`'s 24,000-character per-file bound. That guard was reproduced
without inference; PR #12 addresses this metadata overhead and is installed.
A live GitHub question about a large plan still needs a manual quota-consuming
rehearsal; visible text above the unchanged bound still requires `/discuss`.
The live static reviewer also disclosed denied `rg` execution and the absence of
the trusted root copy of `local-development.md`; it used read-only inspection.

ADR-021's shared dialogue and Issue handoff are implemented locally in
`.agent/plans/shared-issue-dialogue.md`; final developer review and a coordinated
rollout are still pending. None of that follow-up is installed yet. Fadwix,
GBrain, Docker and host-wide settings are outside this rollout.

For the approved ADR-021 rollout, suspend only the three agent workflows before
merging the updated orchestration. Back up installed adapters/profile and compare
their hashes with the pinned candidate. Under the existing review lock, install
the matching review/planning/conversation workers and policies; the shared
`review_policy.py` validates all three contexts. Keep `github_dialogue.py` on the
trusted GitHub side, outside model jobs. Verify the existing auth/sandbox and the
new context schema before re-enabling workflows. Rehearse a disposable Issue →
plan PR → linked/closed Issue → discussion/amendment/review from that PR. A
retry check can simulate link/closure failure with mocks; any live model checks
are manual and consume quota. On failure, restore matching GitHub and runner
versions together. This is an installation plan, not evidence of deployment.

If activation fails, leave agent workflows suspended and restore both the prior
GitHub orchestration and the corresponding runner adapters/profile together.
Keep already generated plans and diagnose protocol/state mismatches; an older
adapter does not automatically understand new structured state. Do not delete
stage history to make a rollback appear successful.

Install the pinned development tools in a local virtual environment:

```sh
python3 -m venv .venv
.venv/bin/python -m pip install -r .agent/infra/requirements-dev.txt
```

Ruff checks Python syntax, unused/undefined names, imports, bug-prone patterns
and simplifications (including SIM117) using root `ruff.toml`. The same pinned
version checks formatting locally and in CI. The Python target remains 3.9;
reference checkouts and generated protobuf bindings are excluded. Python edits
anywhere in the repository and lint configuration changes trigger infra checks.

```sh
.venv/bin/ruff check .
.venv/bin/ruff format --check .
python3 -B -m unittest discover -s .agent/infra/tests -p 'test_*.py' -v
actionlint .github/workflows/*.yml
git diff --check
```

To apply safe lint fixes and formatting locally, run `.venv/bin/ruff check --fix .`
then `.venv/bin/ruff format .` and inspect the diff before publication. CI checks
only; it does not rewrite files.

`review/smoke-review.py` is a separate trusted fixture diagnostic for default
Codex config and unavailable-memory disclosure. It never publishes to GitHub.
Private diagnostics live in `/var/lib/hackrnd-review/checks/`, not Git/artifacts.
Validate report content, source/SHA/skill scope and expected findings; CLI exit 0
alone is insufficient.

Remaining: final ADR-021 review, publication and coordinated rollout;
live changed-SHA re-review and
large-plan question rehearsals; source-link validation; required CI/human branch
rules; Plus renewal/quota/revocation and sustained operation. Do not equate local
or SSH fixture tests with deployed GitHub behavior.
No auto merge/deploy is included.

Rollback: disable only the three agent workflows (`agent-review.yml`,
`issue-planning.yml`, `agent-conversation.yml`); revoke the dedicated SSH key and
remove only `/etc/sudoers.d/hackrnd-agent-review` to stop new jobs. Stop this
installation's `hackrnd-review-*` transient units if necessary. Preserve state.
Log out Codex as the review identity if revoking the session. Remove only its
AppArmor profile with `apparmor_parser --remove` before deleting that file.
If memory was activated later, stop/disable only `hackrnd-gbrain.service` and
`hackrnd-gbrain-backup.timer`. Never disable AppArmor globally, restart Docker,
reboot or operate Fadwix as part of rollback.
