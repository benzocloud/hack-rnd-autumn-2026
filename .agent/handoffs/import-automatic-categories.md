# Immediate categories for imported operations

Owner request, 2026-10-25: automatically show categories immediately after CSV import; use a targeted manual production update without full CI builds.

`ListTransactionViews` now returns an effective category from the existing company catalogue: explicit manual category first, otherwise deterministic payment-purpose rules for imported operations. Category filtering uses the same effective value. Existing imports gain categories on their next read; no forecast or 90-day history is required. The six existing catalogue groups are sales, operating expenses, taxes, financing, own transfers and other. Unrecognized text falls back to other; these are heuristic groups, not measured model accuracy or the finer forecast analysis labels.

The projection does not write bank facts, category overrides, operation versions, account revisions or frozen forecasts. Raw reads for category commands and ML inputs are unchanged, so saving a manual override still persists it and clearing it restores the automatic list category. No migration, protobuf/API schema or frontend build is needed.

Validation: PostgreSQL integration test covers immediate categories before any forecast, tenant catalogue IDs, category filters, unchanged stored facts/versions/revision, manual override and clearing. Related CSV forecast, imports HTTP and manual-supplement integration checks pass with the race detector (11 tests including subtests). Core Linux/amd64 binary builds; Go vet and module-wide golangci-lint v2.15.0 with integration tags pass. The older Homebrew v2.13.2 cannot decode Go 1.27.3 export data; the pinned project version was used instead.

Deployment: only replace Core's static `/app/server` in a thin image based on the current immutable Core runtime, update only `CORE_IMAGE`, and recreate only Core with `--no-deps --no-build --pull never`. Preserve the previous image and private environment backup for rollback. Verify the existing public C900 company has 323 categorized operations, working category filters and the unchanged saved 30-day forecast. Full CI/release builds are intentionally skipped under the owner's urgent instruction.
