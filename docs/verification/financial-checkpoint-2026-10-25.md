# Финансовая приёмка — контрольная точка 25.10.2026

Отчёт агента C. Колонки: implemented / automated contract /
real HTTP / real browser / restart / blocked.

## Идентичность прогона

| Поле | Значение |
| --- | --- |
| Дата (МСК) | 2026-10-25 ~11:30 |
| Test PR | https://github.com/benzocloud/hack-rnd-autumn-2026/pull/50 |
| Test PR branch | `globalarray/test/verify_financial_user_journey` |
| Base main SHA | `71f2a86ffbcf218d7fff517830b7aa41f0a97c95` |
| P0 stack binaries | `e95e98eefd8e897e4d4cc2f339695f004cebb98f` (suffix `p0ml`) |
| PAY stack binaries | `0826fa6118f29bf3609577c668af5e0cd291bf1d` (= `origin/pr-48`, suffix `pay48`) |
| Harness tip | обновляется commit’ом с этим отчётом |
| API origin | isolated `https://127.0.0.1:<ephemeral>` — не production |
| Auth profile | **demo allowlist** `AUTH_DEMO_EMAIL_VERIFICATION=true`, exact `owner-<suffix>-NN@financial-e2e.test` |
| Fixture source | `dataset/statements/C001.csv` SHA-256 `945fe67e8418882bf7bd5e9bdcda570c613fc7777113d55424ae34b052dfa92a` |
| Ordinary slice | `ordinary_90d_c001.csv` SHA-256 `8780e7806a705f1c2898acf234c301ff6c1391f88fbb99bd27bb55f7c4e4bf85` (2026-07-02..2026-09-29) |
| EOD conflict fixture | `eod_conflict.csv` SHA-256 `e5f8d0aebf64ca6fef8a82792ea4d3bb605f32b96c43a219f9fcfb1b06a9aa99` (intra-day distinct `balance_eod`) |
| #48 payments tip | `0826fa6118f29bf3609577c668af5e0cd291bf1d` |
| #49 frontend tip | `14639d524601b26511de207e6addd67433d657d9` |
| #45 ML train tip | open; не закрывает calibration |

## Сводка сценариев

| ID | implemented | automated contract | real HTTP | real browser | restart | blocked |
| --- | --- | --- | --- | --- | --- | --- |
| P0-01..P0-10 | harness | — | **pass** | pending | pending | — |
| R-01 re-upload | harness | — | **pass** | skip | — | — |
| R-02 import idempotency | harness | — | **pass** | skip | — | — |
| R-03 manual after import | harness | — | **pass** | pending | — | — |
| R-04 manual replay | harness | — | **pass** | skip | — | — |
| R-05 exact import as manual | harness | — | **pass** | skip | — | — |
| R-08 forecast stale | harness | — | **pass** | skip | — | — |
| N-01 invalid upload | harness | — | **pass** | skip | — | — |
| N-02 dup key different body | harness | — | **pass** | skip | — | — |
| N-04 short history | harness | — | **pass** | skip | — | — |
| N-05 EOD conflict | harness | — | **pass** | skip | — | — |
| N-08 CSRF invalid | harness | — | **pass** | skip | — | — |
| N-09 foreign company | harness | — | **pass** | skip | — | — |
| PAY-01 create→forecast | harness | — | **blocked:ordinary_csv_current_calendar** | pending | — | cutoff 2026-09-29 ≠ today; in-horizon payment → `UNSUPPORTED_MODEL_CONTEXT` (fail-closed) |
| PAY-02 amount/date edit | harness | — | **pass** (`pay48`/#48) | pending | — | — |
| PAY-03 partial match cycle | harness | — | **pass** (`pay48`/#48) | pending | — | — |
| PAY-04 cancel→history→forecast | harness | — | **pass** (`pay48`/#48) | pending | — | — |
| PAY-05 forbidden/stale | harness | — | **pass** (`pay48`/#48) | pending | — | — |
| Browser smoke | checklist | — | — | **blocked:no_playwright_manifest** | — | shared `@playwright/test` / `test:e2e:financial` — зона агента B |

Команды:

```sh
# P0 + R/N (stack p0ml, Homebrew Python / TLS 1.3)
/opt/homebrew/bin/python3 scripts/financial-e2e/up_stack.py --keep --suffix p0ml
. scripts/financial-e2e/artifacts/stack-p0ml/env.sh
/opt/homebrew/bin/python3 scripts/financial-e2e/run_http.py
/opt/homebrew/bin/python3 scripts/financial-e2e/run_regression.py

# PAY (#48 sources via --repo-root)
git fetch origin refs/pull/48/head:refs/remotes/origin/pr-48
git worktree add --detach /tmp/saldo-pay48 origin/pr-48   # or sibling path
/opt/homebrew/bin/python3 scripts/financial-e2e/up_stack.py --keep --suffix pay48 \
  --repo-root /Users/globalarray/hack-rnd-autumn-2026-pay48
. scripts/financial-e2e/artifacts/stack-pay48/env.sh
/opt/homebrew/bin/python3 scripts/financial-e2e/run_payments.py
```

## PAY-01 — честный blocker

Ordinary CSV якорится на cutoff `2026-09-29`. CURRENT-calendar в #48
(`activateCalendar`) требует `cutoff == today` или `cutoff+1 == today`, плюс
active reconciliation group. Wall-clock today (`2026-10-25`) не совпадает →
in-horizon payment даёт явный `422 UNSUPPORTED_MODEL_CONTEXT`, не silent
zero-risk. Covered-history CURRENT proof живёт в integration-тестах #48 и
**не** заменяет ordinary CSV (см. PAY-06 / handoff #48).

После cancel in-horizon obligation ordinary forecast снова **succeeded**.

## Честные ограничения

- D2.4 runtime categorization/regularity не закрыт этим PR.
- `not_calibrated` / probability quality остаются открытыми (D3.1).
- Double Auth/predictor и financial mock не засчитывались.
- Успех covered fixture ≠ поддержка ordinary CSV + calendar (здесь ordinary CSV).
- C001 не имеет EOD на `2026-09-30`; slice якорится на `2026-09-29`.
- Browser walkthrough и restart-persistence ещё не выполнены.
- CLT Python/LibreSSL не говорит TLS 1.3 — harness требует OpenSSL 3 Python.
- Empty `counterparty_inn: ""` → API `422 VALIDATION_ERROR`; harness omits field.

## Blockers (минимальное воспроизведение)

1. **Browser** — checklist only; нет `@playwright/test` в shared `frontend/package.json`
   (зона B). Isolated UI + self-signed API TLS/same-origin proxy не закрыты.
2. **PAY-01 calendar points** — ordinary CSV + wall-clock today; needs covered
   CURRENT fixture или clock/cutoff alignment (product/candidate #48), не harness.
3. **ML bindings** — локально `scripts/financial-e2e/artifacts/ml-gen/ml` (gitignore).

## Демо-резерв (запись не заменяет тест)

Последовательность: login → company → upload ordinary CSV → preview → commit →
forecast 30 → limitations/`not_calibrated` → (#48) payment CRUD/match/cancel →
forecast after cancel. Файл: `ordinary_90d_c001.csv`. Expected: 30
`daily_points`, без zero-fill unknown days, без fabricated 0% probability.
Резерв: этот отчёт + HTTP P0/R/N на `p0ml` + PAY на `pay48`/#48 SHA выше.
