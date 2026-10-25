# Финансовая приёмка — контрольная точка 25.10.2026

Черновик отчёта агента C. Колонки: implemented / automated contract /
real HTTP / real browser / restart / blocked.

## Идентичность прогона

| Поле | Значение |
| --- | --- |
| Дата (МСК) | 2026-10-25 |
| Test PR branch | `globalarray/test/verify_financial_user_journey` |
| Test PR | https://github.com/benzocloud/hack-rnd-autumn-2026/pull/50 |
| Base main SHA | `71f2a86ffbcf218d7fff517830b7aa41f0a97c95` |
| Harness commit | `971a1b79dca53ef87122b7637283961f2d3ed163` |
| Integrated candidate SHAs | main + #48 + #49 (+ опц. #45); уточнить перед прогоном |
| API origin | isolated only; не `saldo.brokebyte.me` |
| Auth profile | demo allowlist только если server-enabled; пометить |
| Fixture source | `dataset/statements/C001.csv` SHA-256 `945fe67e8418882bf7bd5e9bdcda570c613fc7777113d55424ae34b052dfa92a` |
| Ordinary slice | см. `scripts/financial-e2e/fixtures/manifest.json` после `prepare_fixtures.py` |

## Сводка сценариев

| ID | implemented | automated contract | real HTTP | real browser | restart | blocked |
| --- | --- | --- | --- | --- | --- | --- |
| P0-01 register | harness | — | pending | pending | — | isolated stack |
| P0-02 verify | harness | — | pending | pending | — | |
| P0-03 login | harness | — | pending | pending | — | |
| P0-04 company | harness | — | pending | pending | — | |
| P0-05 upload | harness | — | pending | pending | — | |
| P0-06 preview/ack | harness | — | pending | pending | — | |
| P0-07 commit | harness | — | pending | pending | — | |
| P0-08 ledger | harness | — | pending | pending | — | |
| P0-09 forecast 30 | harness | — | pending | pending | — | |
| P0-10 reload | harness | — | pending | pending | pending | |
| R-* manual/CSV | planned | — | pending | pending | pending | |
| PAY-* lifecycle | planned | — | pending | pending | pending | needs #48 candidate |
| N-* negatives | planned | — | pending | pending | — | after P0 green |

## Честные ограничения

- D2.4 runtime categorization/regularity не закрыт этим PR.
- `not_calibrated` / probability quality остаются открытыми (D3.1).
- Double Auth/predictor и financial mock не засчитываются.
- Успех covered fixture ≠ поддержка ordinary CSV + calendar.
- C001 не имеет EOD на `2026-09-30`; slice якорится на последний день с EOD.

## Blockers (минимальное воспроизведение)

1. **Isolated candidate stack** — локальные images `86c1374` старше main;
   нужен build/run SHA кандидата. Команда/ошибка будет дописана при попытке.
2. **gh auth** — `git push` доступен; `gh` CLI без login (PR create может
   требовать `gh auth login`).
3. **PR #48 open** — payment lifecycle на чистом main blocked.

## Демо-резерв (запись не заменяет тест)

Последовательность: login → company → upload ordinary CSV → preview/ack →
commit → forecast 30 → показать limitations/`not_calibrated` → (если #48)
payment edit → новый forecast. Файл: `ordinary_90d_c001.csv`.
Expected: 30 точек, без zero-fill unknown days, без fabricated 0% probability.
Резерв: заранее записанный walkthrough + этот отчёт на зафиксированных SHA.
