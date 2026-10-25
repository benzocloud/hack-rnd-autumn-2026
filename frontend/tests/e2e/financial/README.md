# Browser scenarios — financial checkpoint

Owned by agent C. Do not put financial mocks here.

## Intent

Short Playwright (or compatible) walkthrough:

1. register / verify / login (demo email only if server allowlisted)
2. company select/create
3. import CSV → preview → commit
4. transactions / forecast 30 points
5. payment lifecycle when #48 candidate is in the integrated stack
6. screenshots without secrets into `scripts/financial-e2e/artifacts/browser/`

## Shared manifests

`frontend/package.json` / lockfile / CI scripts are owned by agent B.
If a direct `@playwright/test` dependency and `test:e2e:financial` script are
required, publish the exact diff in
`.agent/handoffs/checkpoint-financial-e2e.md` and wait for B — or run via
standalone `npx` from harness without editing shared manifests.

## Status

HTTP P0 + R/N green on isolated stack; PAY green on #48 integration stack
(except PAY-01 CURRENT-calendar points — ordinary CSV blocker).

Browser: still checklist-only — shared `@playwright/test` / `test:e2e:financial`
not present in `frontend/package.json` (agent B). See
`smoke.checklist.md` and `docs/verification/financial-checkpoint-2026-10-25.md`.
