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
