# Saldo public integrated deployment

Owner requested deployment of the locally tested frontend/API/Auth/Core/ML
flow after main integration. Base: main 86c1374, 2026-10-24. Worktree:
globalarray/feat/full_stack_deployment. Authorization covers preparing and
executing this Saldo deployment; preserve Fadwix, GBrain and participant work.

## Scope and stages

1. Inspect host and prepare isolated images/Compose/secrets/migrations — done.
2. Start backend privately, apply migrations, verify real Auth/Core/ML flow — done.
3. Back up existing Saldo frontend/nginx configuration; switch the frontend
   image to real API mode and add a verified HTTPS upstream for /api/v1 — done.
4. Public HTTPS/browser smoke and restart/persistence check — passed; rollback
   failure regression passed with isolated fake tools, not a live schema downgrade.
5. Publish and verify automatic main deployment — PR37, CI in progress.

Host verified read-only: brokebyte.me:8321, x86_64, about 2.2 GiB available RAM
and 140 GiB disk. Existing Saldo uses /opt/saldo-frontend and /opt/saldo-landing;
other named containers must remain untouched. Build outside the host. New
backend under /opt/saldo-stack, own PostgreSQL volume/databases/roles, private
network, per-service memory/CPU limits. Only API joins existing Saldo network;
gRPC and PostgreSQL have no published host ports. Existing landing stays intact.

Preserve mTLS API→Auth/Core and Core→ML; verify TLS for PostgreSQL and nginx→API.
Generate independent secrets and CA on the target with restrictive permissions;
never commit or log them. Back up previous frontend image and nginx config
before cutover. Migrations are explicit and never run at application startup.

This is an isolated public demo, not a claim of production-ready registration:
email confirmation remains server-enabled demo only for exact synthetic .test
accounts. Real passwords/session/CSRF/membership checks stay enabled. Financial
pipeline has no mocks. Seed clearly labelled synthetic company/history via
operator preparation only; no invented user ownership or real financial data.
The current app supports a 30-day forecast; imports/invitations/what-if and
public company onboarding remain outside this deployment.

Checks: Compose validation, image builds, shell/Python checks as applicable,
private readiness and real TLS/SQL/forecast result, public register/login/
company/account/create/poll/detail/reload/logout, persistence after service
restart, unrelated-container identity comparison. Record actual results below.

## Verified initial deployment

Application image sources: main86c1374. Built five linux/amd64 images locally,
transferred archive and loaded on target; images selected by immutable IDs.
Separate Saldo PostgreSQL roles/databases migrated (Auth2, Core6), no public
database/gRPC ports. Initial PKI permissions and tmpfs YAML issues were fixed
before frontend switch. New PostgreSQL initialization was completed explicitly;
no existing database or service was removed.

Public HTTPS API registration/demo verify/login, membership/account/forecast
create+poll succeeded through actual Auth/Core/PythonML. Synthetic account secret
stored /opt/saldo-stack/smoke-account.json root-only. Frontend switched to real
API image with previous image/config preserved in first-cutover-backup. Public
login page and server CSRF/demo mode verified in browser. Core/ML restarted;
stored forecasts survived and a new financial smoke passed. Three succeeded
jobs at that check. All pre-existing non-Saldo container IDs preserved. Landing
HTTPS still available. Resource usage remained below configured service caps.

Owner separately approved adding only deploy-stack to existing restricted
saldo-deploy sudo/SSH dispatch; installed and malformed command rejected.
No shell, Docker group or arbitrary root path granted. Automated approval
initially rejected permission expansion; the explicit owner confirmation was
obtained before installation.

Checks: five image builds, Compose, migrations, actual HTTPS financial smoke,
restart/persistence and unrelated-container checks passed. Ruff/format,
ShellCheck, actionlint and failed-migration rollback regression passed.
Caveman review found/fixed directory umask, Bash conditional errexit and missing
full financial readiness. Recheck found no new defects. Ponytail replaced manual
readiness polling with bounded native curl retries; regression passed again.
Reviewer agents did not deploy; runtime verification done by primary agent.
First complete GitHub main-triggered deployment is pending PR37 checks/merge.
