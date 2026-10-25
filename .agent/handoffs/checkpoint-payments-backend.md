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
