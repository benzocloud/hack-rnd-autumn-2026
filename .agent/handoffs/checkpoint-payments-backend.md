# Payments backend checkpoint

Branch: globalarray/feat/payments_frozen_calendar
Base: 0abae4f (fresh origin/main). Current/remote SHA: первый docs checkpoint публикуется; точный SHA доступен в Git/PR head. PR URL: создаётся сразу после push.

Последний законченный этап: source/contract inventory, На ревью; runtime не реализован.

## Матрица
| Область | Existing | Not wired / not implemented |
|---|---|---|
| Create/match | domain/service/SQL + proto | Core gRPC/API |
| List/get/history | proto/OpenAPI | application read + adapters |
| Update/cancel/unmatch | domain operations + proto | application commands/SQL/adapters |
| Calendar | snapshot domain/codec/SQL | полная проверка job capture + groups/mode/ML compatibility |

## Frontend handoff (canonical contract, ещё не runtime)
REST paths/DTO: docs/api/openapi.yaml; RPC: proto/core/v1/payments.proto; map: docs/core/rest-rpc-map.json.
ExpectedPayment: id/account_id, direction, amount/remaining_amount (RUB decimal minor strings), scheduled_date, category_id, counterparty_name, description, status, source, version (decimal string), created_at, reconciliation_group_id.
List/get/create/update/cancel, matches list/create/delete и reconciliation groups должны сохранять существующий wire; additive diff пока отсутствует. Не считать endpoints подключёнными.

Проверки: gh authenticated nickname globalarray; fetch main/#47; PR list и #47 comments/reviews/inline (пусты). Tests ещё не запускались. RTK отсутствует. GBrain unavailable: MCP tools не подключены. Sandbox bwrap отсутствует, shell работает через approved escalation.
Known failures: реализации lifecycle adapters ещё нет; P0 не готов.
Dependency: https://github.com/benzocloud/hack-rnd-autumn-2026/pull/47 @7e5c0d4aff78286c13d9a8245f801e8a5b82e16c, прочитан опубликованный handoff; не переносить dirty tree.
Schema/rollback: изменений нет. Merge/deploy не выполнялись.
Следующая команда: cat services/core/internal/domain/payments/payment_state.go services/core/internal/service/ledger/read.go

## Checkpoint 10:18 МСК

Draft PR: https://github.com/benzocloud/hack-rnd-autumn-2026/pull/48.
Плановый checkpoint `34b6add27a897577e4ed3c2f5d736d4ae67e01f6` pushed и remote проверен.
Опубликованная зависимость #47 @7e5c0d4aff78286c13d9a8245f801e8a5b82e16c интегрирована обычным merge `22a5bcc`; PR является stacked относительно #47. Изменения frontend/общих docs в merge принадлежат зависимости, собственная реализация их не редактирует.
Lifecycle в работе: новые domain update/unmatch, сервисные reads/CAS mutations, исходные projections create/match. Узкий Go unit набор до последних audit-изменений прошёл. Полный блок ещё не проверен/не закоммичен.
Дополнительный wire diff: `PaymentMatch.optional google.protobuf.Timestamp removed_at = 6` — audit отменённого распределения, исключённого из active total. Старые поля/номера неизменны. Generation и schema projection ещё в работе.
Изолированный PostgreSQL18 поднимается внутри контейнера на 127.0.0.1:55488, отдельный .agent/local/pg18, DB saldo_core_test, marker saldo-core-integration. После смены модели прежний процесс отсутствовал; новый старт перепроверяется.
Следующая команда: `git status --short`; затем закончить lifecycle block и serial integration.

## Lifecycle checkpoint 10:27 МСК — На ревью

Текущий опубликованный baseline до этого блока: d7674d52abaac411e689f74d7c738c98fd60b310 (remote SHA проверен). Следующий commit содержит lifecycle, исходные projections create/match, scoped cursor reads, audit unmatch и durable group primitives. P0 forecast activation ещё в работе.

Проверено: Go domain/service unit; Core handler/server compile; API handler/app tests; marked PG18 integration `TestPaymentLifecyclePersistsAuditAndOriginalReplay|TestPaymentExternalReplayAfterMatch|TestHistoryCoverageCarries` с race — pass. ML existing RPC/mTLS suite — 45 tests, без skips после generation; API schema suite — 13 tests pass. Первый ML запуск с 45 skips был неуспешным и заменён настоящим прогоном. Full module lint preliminary обнаружил handler enum/date/format findings; исправляются, итоговый lint ещё не pass.

Реализовано: list/get/create/update/cancel/match/unmatch/history на service/SQL/Core RPC и API lifecycle adapters. Partial cancel сохраняет remaining и allocations; unmatch после cancel возвращает PAYMENT_STATE_CONFLICT. Tombstones читаются через nullable removed_at. Для create/match новых receipts сохранена исходная projection; legacy receipt codec/fingerprint tests сохранены.

Schema: 00013 audit tombstone, downgrade guard при reversed allocations; 00014 independent scoped groups и nullable payment FK. Обе миграции применены только в own disposable DB. Group/calendar end-to-end приёмка ещё не выполнена. Rollback со схемы14 требует совместимого decoder и сохранения групп, не обещается простой downgrade.
Следующая команда: `go test -p 1 -tags=integration,mlintegration -race ./internal/app -run TestPaymentHTTP` с own DB/API binary/Python env после завершения API process build. Затем закончить current calendar/Predict arithmetic, gates и независимое review.

Wire diff перед следующим блоком: для явного различия available-empty и unavailable добавляются `ForecastInput.calendar_knowledge_mode = 18` (новый typed `ForecastCalendarKnowledgeMode`, значения mirror ML: unspecified0/current1/historical_as_of_certified2/unavailable3/synthetic_scenario4), `calendar_as_of_date = 19` string, `no_post_cutoff_movements = 20` bool. Поля 1–17 неизменны. Legacy unspecified не превращается в historical/current knowledge. REST read projection — optional mode/date/bool; request calendar_mode остаётся прежним выбором available/synthetic, а не assertion знания. Current context не выдаётся за synthetic historical; synthetic пока explicit unsupported.

## Проверка публикации и исправление checkpoint

Independent caveman review f344200 обнаружил пропущенный untracked API `payment_groups.go`: опубликованный API checkpoint не собирался. Исправление включается отдельным fix commit немедленно; этот неуспешный checkpoint не объявляется passing. Review также обнаружил external reimport/linkage comparison — исключение изменяемого reconciliation_group_id из external identity исправлено с regression.

Real HTTP tests текущего дерева с race: `TestPaymentForecastHTTPWithPostgresPythonAndRestart` и `TestPaymentHTTPAPILifecyclePersistsAudit` — PASS, package4.577s; весь API→Core→PG→Python mTLS путь, семь прогнозов, 30 exact points, audit/stale/immutable bytes/restart. Auth double и covered CURRENT fixture обозначены в тестах, обычный CSV и настоящий Auth не заявляются.

## Frozen calendar / Predict checkpoint 10:38 МСК — На ревью

Lifecycle publication fix pushed `f4627f1cfcca5fa2ad01ec7e52a618f8e608d4ea`, remote совпал. CURRENT block публикуется следующим commit; Git/PR head — точный current SHA (self-referential SHA не встраивается в собственный commit).

Работает: frozen snapshot payload2 содержит payment remaining/status/version/provenance/group и независимые groups; legacy payload1 читается. Covered current jobs schema2 сохраняют прежний model context mean_90_components_v1/calendar_replacement_v1. Один Predict и independent Go residual/replacement/remaining/components/sourceIDs/overlap/overflow validation. Current пустой календарь отличается от unavailable; public read fields18–20 и known components/effective occurrences объясняют remaining.

Проверки: real Go→Python mTLS current lifecycle; `TestCurrentCalendarForecastFreezesLifecycleAndReplacement` (фактическое имя см. source) real PG/job/worker/reopen, PASS7.083s; combined real HTTP tests PASS4.577s (семь новых forecasts, все30points). Codec current clone/remaining/group regression и mode/overdue/unknown tests прошли узко. Existing ML45 без skips; API schema13 — первый новый запуск нашёл устаревшую copied ForecastInput definition, она синхронизирована и rerun в работе. Full Core serial integration+mlintegration race, full tagged lint/vet запущены; не объявлены passing до завершения.

Ограничения: mean_90_csv_v1 не принимает calendar — любой persisted payment/group даёт UNSUPPORTED_MODEL_CONTEXT (conservative explicit blocker). Historical nonempty calendar/synthetic mode не активируются; synthetic selection явно unsupported, не CURRENT. Недоказанный replacement/overdue/incompatible horizon отклоняются, даты не сдвигаются. Covered CURRENT тесты не доказывают ordinary CSV/calendar поддержку. Probability not_calibrated; training/default не менялись.

Schema/rollback: 00013 tombstones, 00014 groups/link, 00015 разрешает snapshot payload2. Downgrade15 не пройдёт при существующих payload2; rollback обязан сохранять decoder. Старый Core остановить до запуска schema2 producer; работающий сервер не менялся. Старый бинарник нельзя смешивать с новыми jobs. Все миграции выполнялись только в собственном marked PG18.
Следующая команда: проверить `.agent/local/core-full.json`/`core-full.err`, `core-final-lint.log` и `core-final-vet.log`; независимое review следующего опубликованного SHA, затем исправления и финальные CI/main checks.

## Проверки и независимое review 10:44 МСК

Published current calendar SHA:2723d0a7552abb892d27c297d5772f3b0b01b031, remote совпал. Независимый агент применил caveman-review и ponytail-review к собственному diff d7674d5..2723d0a: все4 earlier findings исправлены, новых подтверждённых нет; ponytail net:-0. Review read-only, без тестов/апрува. Проверка более поздних fixes будет отдельной.

Full serial Core race integration+mlintegration:746 passed tests/subtests,2 failed,1 opt-in large-import skip;29 test packages passed,1 failed,9 packages без tests. Два failures — timestamp microsecond roundtrip external payment replay и []/nil roundtrip пустых groups. Исправлены normalization; affected packages rerun нашёл дополнительное UTC-location различие revoke receipt, исправлено в scanGroup. Все3 проблемных test scenarios затем прошли с race1.545s. Старый immutable snapshot test теперь меняет valid timestamp вместо заведомо несогласованного ExcludedPayments. Не выдавать исходный suite за полностью зелёный.

API final full race:298 tests/subtests /8 test packages pass (4 без tests); vet/full lint0/fmt clean. Core tagged full lint0/vet pass; последний fmt fix calendar_mode_test сделан с module config. Buf lint/build/breaking и scripts/check-core-contracts.sh — pass (58 REST maps, generated drift, consumer compilation). API generation --check — pass после генерации настоящим pinned grpc_tools protoc: Buf unknown version header ранее вызывал CI false drift, сохраняется реальный version header. API schema13 pass; ML45 без skips; pinned Ruff0/check+format14files clean.

Свежий origin/main сейчас6c9edd969b3a694bccabaf6980b86531e2722490; включает #47/onboarding/training. Следующий шаг — обычный merge в нашу ветку, минимальные conflicts, релевантные checks и actual CI. Main не изменялся нами.
