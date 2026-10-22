# Agent infrastructure

Status **2026-10-22**: Codex Plus runner installed on `runner.brokebyte.me`;
The initial infrastructure PR has been merged into `main`. The extension in
`globalarray/chore/improve_agent_workflows` (optional Issue, full discussion,
editable progress report and descriptive runner branches) is not yet published. GBrain is deferred. Fadwix containers, config, networks, ports,
Docker and kernel settings were not changed; no reboot was performed.

## Responsibility boundaries

| Directory | Owns | Entrypoints |
| --- | --- | --- |
| `bootstrap/` | Checksummed runtime installation and versions | `bootstrap.py`, `versions.json` |
| `runner/` | Restricted SSH, resource limits, sandbox, shared inference and transport | `install-workflow.py`, `review_dispatch.py`, `job.py`, `github_api.py`, `prepare-review.py` |
| `review/` | PR context, skill routing, report validation and advisory publication | `github_review.py`, `ssh_review.py`, `review_policy.py`, `smoke-review.py` |
| `planning/` | Issue assessment and plan-only branch/PR lifecycle | `github_plan.py`, `ssh_plan.py`, `plan_policy.py` |
| `skills/` | Pinned upstream instruction sources and installation | `install-skills.py`, `skills.json` |
| `gbrain/` | Deferred memory service, credentials, backup and timer | `configure.py`, `backup.py`, `hackrnd-gbrain*` units |
| `tests/` | Safety and protocol regression checks | `test_*.py` |

GitHub events and job permissions belong in `.github/workflows/`; human/agent
policy belongs in `.agent/workflows/` and accepted decisions. Runtime adapters
are deployed with the same `runner/`, `review/`, `planning/` boundaries under
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
  Exact `/create-plan` in the Issue creates `runner/fix/<short_description>/issue_N` with only
  `.agent/plans/issue-N-fix.md` and a draft PR. `/discuss-plan <changes>` in that
  managed PR revises only the plan. Both commands require a human with write,
  maintain or admin permission. No implementation, approval, merge or deployment.
  Plan PRs use `Refs #N`, avoiding automatic Issue closure when a plan merges.
  Plan commits use `chore:` under `.agent/workflows/git-conventions.md`.
- `infra-checks.yml`: run stdlib safety tests for infrastructure changes on
  pushes/PRs. Application Go/Python/frontend/Buf CI awaits actual project code.

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
ssh root@runner.brokebyte.me 'python3 /root/hackrnd-bootstrap/infra/runner/install-workflow.py /root/hackrnd-bootstrap/review-key.pub'
```

The installer updates root-owned adapters, scoped sudoers, key restriction,
workflow profile and the slice unit. It preserves auth and rejects different
existing authorized keys. It reloads systemd units without restarting Fadwix.
Store the private key in GitHub `AGENT_REVIEW_SSH_KEY`, and a host key verified
through a trusted channel in `AGENT_REVIEW_KNOWN_HOSTS`. Never commit private
keys, tokens or OAuth state. Never use an unverified keyscan as the trust source.

## Pins and source references

`bootstrap/versions.json` pins release URLs/archive SHA-256; `skills/skills.json`
pins upstream commits/licenses. No upstream installer or hooks are executed.

| Component | Pin/source |
| --- | --- |
| GitHub Actions | [checkout v7.0.1](https://github.com/actions/checkout/releases/tag/v7.0.1), [upload-artifact v7.0.2](https://github.com/actions/upload-artifact/releases/tag/v7.0.2), [download-artifact v8.0.2](https://github.com/actions/download-artifact/releases/tag/v8.0.2); full commit pins in workflows, native Node.js 24 runtime verified in each `action.yml` |
| Codex + companions | [OpenAI 0.163.0](https://github.com/openai/codex/releases/tag/rust-v0.163.0) |
| Bun (deferred path) | [1.4.3](https://github.com/oven-sh/bun/releases/tag/bun-v1.4.3) |
| GBrain | [garrytan/gbrain 0.60.366.0](https://github.com/garrytan/gbrain/releases/tag/v0.60.366.0), `c94425368cd5b9eb321a724cfd462e6b52bdd4d2` |
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

## Deferred memory and recovery

On compatible hardware only: `bootstrap/bootstrap.py --with-gbrain`, then
`gbrain/configure.py`. GBrain uses pinned source/frozen dependencies with lifecycle
scripts disabled. On this VM, diagnostic Bun/GBrain artifacts exist but have
never constituted an operational memory service. Initialize authenticated
localhost MCP before explicitly enabling it; keep `required=false`.

Future MCP binds `127.0.0.1:3131/mcp`. Grant developers separate identities/private
network access; never distribute root. Reviewer token must be server-enforced
read-only. Keep root admin/writer tokens under `/etc/hackrnd-agent/` (`0700`) and
reviewer token private (`0600`). Token loading is not enabled in the current jobs.

Before claiming memory ready, verify unauthenticated denial, writer/read access,
reviewer write denial, persistence, and two-client retrieval with provenance.
Native PGLite backup stops only `hackrnd-gbrain.service` and restarts on failure;
restore only into a new private directory. Backup may contain secrets: establish
private encrypted off-host retention. Neither backup/restore nor MCP has passed
its rehearsal. Prepared GBrain unit/timer are inactive.

## Extension rollout

The new planner report adds a validated `branch_description`; its publisher builds
`runner/fix/<short_description>/issue_N`. Update the GitHub orchestration and
root-owned runner adapters together. Until the extension is approved/published,
the active runner retains its initial protocol. Do not deploy only one side and
claim the new flow operational. After merge, copy the updated infra directory to
private staging and run `runner/install-workflow.py` with the existing public key,
then rehearse scope, plan creation, discussion and review on test objects.

## Checks, remaining activation and rollback

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

Remaining: open/review the rollout PR; merge workflows to default branch;
real Issue → scope → plan PR → discussion → explicit review → changed-SHA review
rehearsal; required CI/human branch rules; Plus renewal/quota/revocation and sustained
operation. Do not equate local or SSH fixture tests with deployed GitHub behavior.
No auto merge/deploy is included.

Rollback: disable only the two model workflows; revoke the dedicated SSH key and
remove only `/etc/sudoers.d/hackrnd-agent-review` to stop new jobs. Stop this
installation's `hackrnd-review-*` transient units if necessary. Preserve state.
Log out Codex as the review identity if revoking the session. Remove only its
AppArmor profile with `apparmor_parser --remove` before deleting that file.
If memory was activated later, stop/disable only `hackrnd-gbrain.service` and
`hackrnd-gbrain-backup.timer`. Never disable AppArmor globally, restart Docker,
reboot or operate Fadwix as part of rollback.
