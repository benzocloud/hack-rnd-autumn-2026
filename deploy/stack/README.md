# Integrated Saldo deployment

Public demo: `https://saldo.brokebyte.me`; application uses real API/Auth/Core/ML.
Email confirmation alone uses ADR-028 demo mode for exact allowlisted `.test`
accounts. Finance is real computation over explicitly synthetic operator data.
Invitations, public company onboarding, imports and what-if are not implemented.

## D2.2 upload candidate (not deployed)

The financial-demo branch adds upload/list/preview/issues over real Core storage.
Applying a preview remains disabled until D2.3; uploading never changes Ledger,
source binding or account revision. This section describes a candidate, not the
current public server state.

Before an authorized rollout, apply Core migration 00007 separately. Install the
updated Compose/nginx templates and add `API_IMPORT_SPOOL_DIR=/var/lib/saldo/import-spool`
to the existing private `api.env`; do not rerun initial provisioning over existing
credentials. The dedicated API volume starts with owner 65532 and mode 0700 from
the image. Keep one API process per spool directory. It reserves at most two
100 MiB uploads, creates 0600 files and removes orphans on startup. Do not point
it at the 16 MiB `/tmp` tmpfs or a shared/public directory.

Core accepts at most 100 MiB/300000 CSV rows; nginx permits a 101 MiB multipart
envelope. Each logical CSV record is bounded to 64 KiB, including multiline
quoted text. A preview with more than 1000 EOD dates is explicitly blocked and
its displayed balance list is incomplete; all staged rows remain private.
These candidate limits need review before broader historical datasets.
Nginx streams directly to API. API/Core both verify size and SHA256.
Core keeps chunks/jobs in PostgreSQL before 202, with 200 MiB/account, two
unfinished jobs/account and 1 GiB global raw reservations. These are application
quotas, not a filesystem/WAL limit: provision and monitor PostgreSQL disk space
before enabling uploads. Incomplete receiving leases expire after five minutes;
uncommitted previews expire after seven days. Raw bytes are removed 24 hours
after the first terminal state; metadata/replay and diagnostics remain durable.

The existing deployment workflow does not provision this new mount/env/nginx
configuration automatically. Live installation and deployment need separate
authorization and a restart/persistence smoke test.

## Server layout

- `/opt/saldo-stack`: root-only configuration, credentials, private CA, service
  certificates, release backups; Compose project `saldo-stack`.
- PostgreSQL 18.6: own persistent volume, separate `saldo_auth`/`saldo_core`
  roles/databases, SCRAM and verified TLS. No published database/gRPC ports.
- Only API joins `saldo-frontend_default`, under alias `saldo-api`. API HTTP
  listener uses TLS; existing Saldo nginx verifies its private CA/name.
- `nginx-api.conf.fragment` belongs before `location /` in the existing Saldo
  HTTPS config; copy only the public CA as `nginx/backend-ca.pem`. Landing stays.
- Every runtime has CPU/memory/PID limits. Images are built outside the host.
- `/opt/saldo-stack/smoke-account.json` contains a synthetic fixture credential
  root-only, never committed/logged. The fixture does not establish real company
  ownership and is not a replacement for public onboarding.

First provisioning is explicit: copy Compose/init/provision/scripts into this
directory, run provision.py once as root, load/pull images, write `.env` with
API_IMAGE/AUTH_IMAGE/CORE_IMAGE/ML_IMAGE immutable references. Apply Auth and
Core migrations separately, then start services. `seed-demo.py` is one-time
operator provisioning through real register/verify/login plus explicitly labelled
SQL fixtures; do not rerun over an existing account/database. Runtime startup
never applies migrations. Back up the previous frontend env/nginx config before
initial cutover; the frontend build must use `developerMode=dev`.

## Automatic deployment

`.github/workflows/stack-deploy.yml` runs on relevant pushes to main and manual
dispatch. PR runs verify scripts/runtimes/frontend and build images without
publishing/deploying. Main builds and publishes five images to GHCR, collects
immutable digests, then uses the existing `frontend-mock` GitHub environment's
restricted SSH credential to send one `deploy-stack` command. The legacy
environment name is retained; it does not select frontend mock mode.

User explicitly approved the additional restricted sudo entry on 2026-10-24.
Server root-owned `/usr/local/sbin/saldo-stack-deploy` accepts only a 40-hex
revision, five SHA256 digests and a registry actor; repositories/image names are
fixed. No eval, arbitrary path, shell/PTY/forwarding or Docker-group access is
granted to CI. Preserve existing frontend/landing forced commands. SSH wrapper
dispatches `deploy-stack` as split arguments with globbing disabled; root script
validates their exact count/shape before any action. Registry token arrives on
stdin and temporary Docker login is removed on exit.

Root-owned infrastructure and PKI are not overwritten by CI. Changes to Compose,
deploy scripts, certificates or nginx need an administrator installation before
the new workflow runs. CI handles immutable image releases and migrations only.
Certificates expire after 90 days; operator rotation is required before expiry.
The old frontend mock workflow remains manual-deploy only; do not dispatch it
over the integrated demo. Landing can still be deployed independently.

Deploy serializes backend/frontend image changes, snapshots image/config refs
and takes a pre-migration pg_dumpall backup. It applies migrations with explicit
error checks, restarts backend, updates frontend and verifies public TLS. The
mandatory smoke logs in, checks membership/account, creates a new forecast,
polls to succeeded with 30 points and logs out; a running Auth alone cannot
pass it. Failure restores previous images/config; database is preserved, never
automatically downgraded. Schema changes therefore must stay backward compatible.

## Checks and operations

```sh
cd /opt/saldo-stack
docker compose ps
python3 smoke.py
cat deployed-revision
```

Use root-only `releases/<revision>-<time>/` image/config backups for rollback.
Do not remove the PostgreSQL volume, use compose down -v, prune Docker or modify
Fadwix/GBrain/other services. Keep pre-migration dumps off-host encrypted under
the operator's backup policy; automated off-host backup is not implemented here.

Initial deployment verified 2026-10-24: base application main86c1374, explicit
Auth/Core migrations, real public financial smoke, frontend real-API image,
Core/ML restart and persistence. All pre-existing non-Saldo container IDs stayed
unchanged. Secrets and service keys were created only on target. CI deployment
proof is recorded separately after the workflow's first successful main run.
