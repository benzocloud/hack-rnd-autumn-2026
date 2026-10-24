# Saldo public integrated deployment

Owner requested deployment of the locally tested frontend/API/Auth/Core/ML
flow after main integration. Base: main 86c1374, 2026-10-24. Worktree:
globalarray/feat/full_stack_deployment. Authorization covers preparing and
executing this Saldo deployment; preserve Fadwix, GBrain and participant work.

## Scope and stages

1. Inspect host and prepare isolated images/Compose/secrets/migrations — in progress.
2. Start backend privately, apply migrations, verify real Auth/Core/ML flow.
3. Back up existing Saldo frontend/nginx configuration; switch the frontend
   image to real API mode and add a verified HTTPS upstream for /api/v1.
4. Public HTTPS/browser smoke, restart/persistence check and rollback verification.

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
