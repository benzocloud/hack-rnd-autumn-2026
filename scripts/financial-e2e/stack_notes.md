# Isolated stack notes (no deploy/** edits)

## Reference surfaces (read-only)

- `deploy/stack/compose.yaml` — production-shaped stack (external frontend network,
  immutable image digests, private PKI under `/opt/saldo-stack`)
- `services/core/compose.integration.yml` — disposable Postgres for Core tests
- `frontend/compose.yaml` — frontend image only
- Service READMEs / `.env.example` — Auth demo flags, addresses

## Required properties for agent C stack

1. Unique `COMPOSE_PROJECT_NAME` / `SALDO_E2E_PROJECT`
2. Loopback-only published ports; conflict check before bind
3. Disposable DB volumes; never production volume
4. Own ephemeral PKI under harness-controlled temp dir (not Fadwix/GBrain/Grafana)
5. Real Auth/API/Core/ML binaries or images built from the **candidate SHAs**
6. Staff migrations applied explicitly; runtime start does not migrate silently
7. `AUTH_DEMO_EMAIL_VERIFICATION` only with `AUTH_ENV=local` + exact `.test` allowlist
8. Teardown only after marker/identity check that project name matches ours

## Current machine snapshot (25.10.2026 ~10:53 МСК)

- Docker Desktop available
- Images present: `saldo-{api,auth,core,ml,frontend-real}:86c1374` (older than main `6c9edd9`)
- Foreign containers present (`saldo-onboarding-integration-*`, codex agent) — do not reuse
- Public `saldo.brokebyte.me` — out of scope for proof

## Blocker template

If stack cannot start, record in handoff:

- exact command
- exit code / first actionable error
- which image/binary SHA was requested vs available
- ports attempted
- whether PKI/env generation failed

Then continue preparing/running whatever can run offline (fixtures, matrix, dry docs).
