# Финансовая приёмка — контрольная точка 25.10.2026

Отчёт агента C. Колонки: implemented / automated contract /
real HTTP / real browser / restart / blocked.

## Идентичность прогона

| Поле | Значение |
| --- | --- |
| Дата (МСК) | 2026-10-25 ~11:11 |
| Test PR | https://github.com/benzocloud/hack-rnd-autumn-2026/pull/50 |
| Test PR branch | `globalarray/test/verify_financial_user_journey` |
| Base main SHA | `71f2a86ffbcf218d7fff517830b7aa41f0a97c95` |
| Stack binaries built from | `e95e98eefd8e897e4d4cc2f339695f004cebb98f` (worktree at up_stack) |
| Harness tip | обновляется commit’ом с этим отчётом |
| API origin | isolated `https://127.0.0.1:<ephemeral>` — не production |
| Auth profile | **demo allowlist** `AUTH_DEMO_EMAIL_VERIFICATION=true`, exact `owner-p0ml-NN@financial-e2e.test` |
| Fixture source | `dataset/statements/C001.csv` SHA-256 `945fe67e8418882bf7bd5e9bdcda570c613fc7777113d55424ae34b052dfa92a` |
| Ordinary slice | `ordinary_90d_c001.csv` SHA-256 `8780e7806a705f1c2898acf234c301ff6c1391f88fbb99bd27bb55f7c4e4bf85` (2026-07-02..2026-09-29) |
| #48 payments tip | `1f10b6e615296a01d64957730ac2ce38a2e2b11c` (не в этом прогоне) |
| #49 frontend tip | `7d6d749c4dcd0547e11d48162527c38021dbb003` (browser ещё не) |
| #45 ML train tip | `462344f9a11ae563c7d8d104cd6844270f64983d` (не закрывает calibration) |

## Сводка сценариев

| ID | implemented | automated contract | real HTTP | real browser | restart | blocked |
| --- | --- | --- | --- | --- | --- | --- |
| P0-01 register | harness | — | **pass** | pending | — | — |
| P0-02 verify (no autologin) | harness | — | **pass** | pending | — | — |
| P0-03 login | harness | — | **pass** | pending | — | — |
| P0-04 create company | harness | — | **pass** | pending | — | — |
| P0-05 upload CSV | harness | — | **pass** | pending | — | — |
| P0-06 preview | harness | — | **pass** | pending | — | — |
| P0-07 commit | harness | — | **pass** | pending | — | — |
| P0-08 ledger | harness | — | **pass** (page=100) | pending | — | — |
| P0-09 forecast 30 | harness | — | **pass** (`daily_points=30`) | pending | — | — |
| P0-10 reload | harness | — | **pass** | pending | pending | — |
| R-* CSV/manual | planned | — | pending | pending | pending | — |
| PAY-* lifecycle | planned | — | pending | pending | pending | needs #48 |
| N-* negatives | planned | — | pending | pending | — | after more coverage |

Команды успешного P0:

```sh
/opt/homebrew/bin/python3 scripts/financial-e2e/up_stack.py --keep --suffix p0ml
. scripts/financial-e2e/artifacts/stack-p0ml/env.sh
/opt/homebrew/bin/python3 scripts/financial-e2e/run_http.py
# exit 0; P0-01..P0-10 pass
```

## Честные ограничения

- D2.4 runtime categorization/regularity не закрыт этим PR.
- `not_calibrated` / probability quality остаются открытыми (D3.1).
- Double Auth/predictor и financial mock не засчитывались.
- Успех covered fixture ≠ поддержка ordinary CSV + calendar (здесь ordinary CSV).
- C001 не имеет EOD на `2026-09-30`; slice якорится на `2026-09-29`.
- Browser walkthrough и restart-persistence ещё не выполнены.
- CLT Python/LibreSSL не говорит TLS 1.3 — harness требует OpenSSL 3 Python.

## Blockers (минимальное воспроизведение)

1. **Browser** — `frontend/tests/e2e/financial/smoke.checklist.md` только; shared
   Playwright manifests у агента B.
2. **PR #48 open** — payment lifecycle на чистом main blocked.
3. **ML bindings** — нужны локально в `scripts/financial-e2e/artifacts/ml-gen/ml`
   (gitignore); без них `up_stack` без `--skip-ml` падает.

## Демо-резерв (запись не заменяет тест)

Последовательность: login → company → upload ordinary CSV → preview → commit →
forecast 30 → limitations/`not_calibrated` → (если #48) payment → новый forecast.
Файл: `ordinary_90d_c001.csv`. Expected: 30 `daily_points`, без zero-fill unknown
days, без fabricated 0% probability. Резерв: этот отчёт + успешный HTTP P0 на
зафиксированных SHA.
