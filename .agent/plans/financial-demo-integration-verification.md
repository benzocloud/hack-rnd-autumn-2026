# Financial demo: integration verification

Owner requested frontend validation and merging compatible Core/API/Auth work
into main. Date: 2026-10-24. Isolated worktree:
globalarray/feat/financial_demo_integration from main cc6d034. Includes Core
0c88a25 and Auth PR34 687101e; API changes copied without editing its worktree.

## Stages

1. Integration: Core/Auth merge and API snapshot apply without conflicts — done.
2. Validation: actual HTTPS frontend/API/Auth/Core/PostgreSQL/Python ML — done.
3. Publication: separate Core PR, integration PR, CI and main merge — in progress.

Local only: separate saldo_full_core_test/saldo_full_auth_test databases in the
existing dedicated test PostgreSQL container. Ephemeral CA/service certificates
under /private/tmp, loopback listeners, financial mocks disabled. No server
deployment, Fadwix/GBrain changes or participant checkout/branch modifications.

Real registration/demo verification/no auto-login, password login, company/account
read passed. Membership was explicitly prepared by operator for the synthetic
company: public company onboarding is not implemented. Browser login then
created a forecast through HTTP202/polling; dashboard and detail page displayed
the stored result and explicit limitations. Reload reused the single succeeded
job. API→Auth/Core and Core→ML used mTLS with pinned service identities. HTTPS
used an ephemeral local CA and an explicit browser exception for localhost.

Frontend: 22 tests, TypeScript, ESLint, production build passed. Dev HMR proxy
was replaced by a production build for browser validation. No frontend source
edits beyond Auth PR34 are needed. Core full task check passed. Auth full
unit/PostgreSQL integration/race and lint passed. API full check script passed:
canonical bindings, unit/race/vet/lint, 89.83% statement coverage, HTTPS/mTLS
process smoke and 36 real response OpenAPI checks. Initial API sandbox bind
denial was rerun with local networking; protobuf mismatch was only local
generator v1.36.11 versus pinned v1.36.12 and passed with pinned tools.

Independent compatibility review checked tenant/session/CSRF/deadlines/mTLS,
SQL parameters/context/errors and pool/transaction ownership: no confirmed bug.
Earlier Core Caveman/Ponytail reviews are recorded in core-api-forecast-flow.md.

Limits: 30-day horizon, observed cutoff EOD plus explicit certified 90-day
history coverage required. Calendar unavailable, probability not_calibrated;
recommendations/scenarios/imports/invitations/user onboarding remain incomplete.
No claim of full product MVP or deployed real backend.
