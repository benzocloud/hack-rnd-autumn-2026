# Checkpoint: verify the real financial user journey

Дата: 25.10.2026, Europe/Moscow. Агент C (QA/integration).
Режим: промежуточное ревью владельца отложено до общего результата (явное
исключение ADR-020 из поручения). Этапы после завершения — «На ревью».
Merge, ready-for-review, public deployment и изменение работающего сервера
запрещены. Разрешены commit/push и один draft PR тестового harness.

## Цель

Воспроизводимая сквозная приёмка на реальных сервисах:

Frontend → Go API → Auth/Core → PostgreSQL → Python ML.

Результат: runnable harness, browser scenarios, отчёт по конкретным SHA,
точные blockers с минимальным воспроизведением. Без повторной реализации
продукта и без переделки инфраструктуры.

## Текущее состояние (проверено 25.10 ~10:53 МСК)

- Worktree: `/Users/globalarray/hack-rnd-autumn-2026-financial-e2e`
- Branch: `globalarray/test/verify_financial_user_journey`
- Base `origin/main`: `71f2a86ffbcf218d7fff517830b7aa41f0a97c95`
- GBrain MCP: недоступен в этой Cursor-сессии (нет namespace/tools recall);
  продолжаем по Git/ADR/PR.
- Чужой dirty tree `hack-rnd-autumn-2026` на
  `globalarray/feat/import_source_preview` не трогали.
- Dataset (gitignored): `/Users/globalarray/hack-rnd-autumn-2026/dataset`
- Локальные Docker images `saldo-{api,auth,core,ml,frontend-real}:86c1374`
  есть; это старее текущего main — для приёмки нужны образы/binaries с
  проверяемого кандидата, не слепое переиспользование 86c1374.
- Public `https://saldo.brokebyte.me` не используем как proof (агент B:
  CSRF 503); свой isolated stack.

### Зависимости (SHA на момент старта; перепроверить перед прогоном)

| Работа | Ветка / PR | SHA / статус |
| --- | --- | --- |
| main base | `origin/main` | `6c9edd9` |
| CSV eligibility + manual supplements | PR #47 | merged `f686ca8` (в main) |
| Payments / frozen calendar | `globalarray/feat/payments_frozen_calendar` PR #48 | head ~`ac90fa3` (open; не в main) |
| Financial frontend | `globalarray/feat/financial_user_journey` PR #49 | head ~`7d6d749` (open) |
| Company onboarding | `prosteyshiyyy/feat/company_onboarding` PR #41 | 0 commits ahead of main (уже в main) |
| ML training | `god4k1ta/feat/ml_trained_forecast` PR #45 | head ~`462344f` (open; не закрывает not_calibrated) |
| Import source preview (локальный) | `globalarray/feat/import_source_preview` | `7e5c0d4` (не путать с #47) |

D2.4 runtime categorization/regularity этим поручением не закрывается.
`not_calibrated` остаётся открытым требованием продукта.

## Принятые решения

ADR-002/003/020/022/024–032, особенно ADR-028 (demo email только server-enabled
allowlist), ADR-030 (fixture ≠ bank certified), ADR-031 (обычный CSV 90 дней,
cutoff EOD, unknown days, отказ при reconciliation gaps), ADR-032 (manual после
импорта). Git conventions ADR-015/018.

## Scope

В скоупе (только свои пути):

- `scripts/financial-e2e/**`
- `frontend/tests/e2e/financial/**`
- `docs/verification/financial-checkpoint-2026-10-25.md`
- `.agent/plans/checkpoint-financial-e2e.md`
- `.agent/handoffs/checkpoint-financial-e2e.md`

Вне скоупа: `services/**`, canonical proto/OpenAPI, `frontend/src/**`,
Auth/onboarding реализация, `deploy/**`, общие CI workflows, чужие планы,
дублирование `deploy/stack/smoke.py`, product bugfix параллельно владельцу.

Нужный diff общих frontend manifests/config — сначала в handoff (владелец: агент B).

## Источник тестовых CSV

| Fixture | Источник | Назначение |
| --- | --- | --- |
| `ordinary_90d_c001.csv` | slice `dataset/statements/C001.csv`, окно 90 календарных дней с cutoff EOD на последнем наблюдаемом дне | основной пользовательский CSV-путь (ADR-031) |
| `short_history.csv` | укороченный slice | негатив: короткая история |
| `eod_conflict.csv` | тот же день, разные EOD | негатив: EOD conflict |
| Covered fixture mode | только если сервер явно включает dataset fixture assumption | отдельно от ordinary CSV; успех fixture ≠ CSV calendar |

Проверенные hashes исходника (полный C001, не slice):

- `C001.csv` SHA-256: `945fe67e8418882bf7bd5e9bdcda570c613fc7777113d55424ae34b052dfa92a`
- Каталог dataset gitignored; harness читает `SALDO_DATASET_DIR` или
  соседний `../hack-rnd-autumn-2026/dataset`.
- Наблюдение: у C001 нет строк на `2026-09-30`; последний EOD —
  `2026-09-29` (`1723624.49`). Slice должен якорить cutoff на день с EOD,
  иначе ожидаем контрактный отказ missing cutoff anchor — это не «чинить»
  модель ради зелёного теста.

## Окружение

| Требование | Значение |
| --- | --- |
| Compose project | `saldo-financial-e2e-<suffix>` (уникальный) |
| Ports | свободные loopback; не production |
| DB/volumes | disposable; marker/identity перед teardown |
| Auth demo | `AUTH_DEMO_EMAIL_VERIFICATION=true` только local + allowlisted `*.test` |
| Secrets | генерируются локально; не коммитятся |
| Запрещено | production DB, чужие containers/networks/PKI, Fadwix/GBrain/Grafana, privileged sockets, ослабление TLS/CSRF |

Изолированный запуск: переиспользовать штатные migrations/config сервисов и
существующий `deploy/stack` / service compose как **reference**, не меняя
`deploy/**`. Harness-обёртка живёт в `scripts/financial-e2e/` (project name,
ports, env names, teardown). Локальные images `86c1374` недостаточны без
rebuild/merge кандидата.

## Этапы

### 1. Матрица и ранняя публикация

Статус: **На ревью**

Результат: draft PR с матрицей, CSV source, env, dependency SHA, планом запуска.

- [x] plan/handoff
- [x] skeleton harness + scenario matrix
- [x] commit/push/draft PR #50
- [x] remote SHA verification

### 2. Изолированное окружение

Статус: **На ревью**

Результат: поднимается disposable stack реальных сервисов либо точный blocker.

### 3. Основной пользовательский сценарий

Статус: **На ревью** (real HTTP P0 pass 11:11 МСК)

register → verify → login → create company → upload → preview/issues/ack →
commit → ledger → forecast → polling → 30 точек → saved reload.
Без SQL financial facts/membership/coverage. До полного onboarding-пути
допустима штатно созданная test company, но first-user flow остаётся открытым
если create company недоступен на кандидате.

### 4. CSV/manual regression

Статус: **Не начат**

Idempotency, manual provenance, duplicate rejection, ack на CSV∩manual,
persistence, stale forecast.

### 5. Платёжный lifecycle

Статус: **Не начат**

create → forecast → amount/date edit → partial match → unmatch → partial →
cancel → новый forecast → reload/restart. Covered fixture и ordinary CSV
отдельно. Зависимость: PR #48.

### 6. Негативные сценарии

Статус: **Не начат**

После зелёного happy-path.

### 7. Persistence, browser walkthrough, ревью harness

Статус: **Не начат**

Restart своих сервисов; browser screenshots без секретов; caveman/ponytail
своего harness/report.

## Матрица сценариев (колонки отчёта)

`implemented` / `automated contract` / `real HTTP` / `real browser` /
`restart` / `blocked`

См. `scripts/financial-e2e/scenarios.md` и итоговый
`docs/verification/financial-checkpoint-2026-10-25.md`.

## Риски

1. Кандидат для интеграции = main + #48 + #49 (+ опционально #45); чужие
   коммиты не входят в тестовый PR.
2. Ordinary CSV без cutoff EOD на выбранной дате — ожидаемый отказ.
3. D2.4 / probability не закрыты.
4. `gh` API auth в этой сессии может быть недоступен при живом `git push`.

## Открытые вопросы владельцу

Нет блокирующих вопросов на старте harness; blockers фиксируются по факту прогона.
