# Go API: первый финансовый проход за час

Дата: 2026-10-24. Начало: 18:35 МСК; контрольная точка: 19:35 МСК.
Основание: прямое поручение владельца «фиксируй план и приступай» после
согласования часового сценария. База: main 01ecfc0 (PR #31); отдельный worktree
api-financial-flow, ветка globalarray/feat/api_financial_flow. Это детализация F1,
не принятие/перенумерация остальных этапов go-api-financial-integration.md.
Ревью отложено до конца данного согласованного часового среза; результат
передаётся владельцу. Commit/push/PR/merge/deployment отдельно не разрешены.

## Цель и границы

Вход → подготовленная компания → счёт → forecast на 30 дней → polling/график →
повторное чтение сохранённого результата. Подготовленные история/EOD должны
быть настоящими входами Core. API не владеет БД, jobs и вызовом ML.
Использовать canonical Auth/Core proto, OpenAPI и docs/core/api-adapter.md.
Не трогать frontend/alites, Auth/Core реализацию и worktree основной задачи.
Новые компания/регистрация, CSV, приглашения/сценарии и остальные 54 операции
за рамками первого часа. Уже реализованные 10 Auth endpoints сохраняются.

## Проверенное начало и зависимости

PR #30 содержит API Auth boundary/tests; PR #31 добавляет frontend/landing,
Go API не меняет. Других открытых PR/API-веток нет (проверено GitHub MCP).
Core Company/bootstrap находится в отдельной ветке 39d0f41, локальные C3/C4
добавляют чтение счёта/EOD; основная задача начала C5. В проверенных исходниках
нет Create/Get/ListForecast handlers и Auth business runtime. Поэтому реальный
browser→Auth/Core→ML E2E пока блокируется upstream. Наличие protobuf этого не
исправляет. Тестовые RPC доказывают адаптацию, не готовый финансовый runtime.

## Этапы и критерии

1. 0–10 минут: инвентаризация, готовность upstream/PKI/тестовых данных и baseline.
   Статус: Выполнено обследование, upstream-блокеры зафиксированы выше.
2. 10–30 минут: API-local Core bindings, app-owned mTLS client/config/lifecycle,
   GET /companies, GET /companies/{id}, GET /companies/{id}/account,
   GET/POST /companies/{id}/forecasts, GET /companies/{id}/forecasts/{forecast_id}.
   Статус: Реализовано и проверено локально. Company list: Auth ListMemberships → Core ReadCompany.
   Scope account только из Core GetDefaultAccount, не из тела браузера.
3. 30–40 минут: DTO/oneof/null/строковые int64, typed errors и проверки отказов,
   сохранение исходного cookie/CSRF/idempotency key, bounded deadlines без retries.
   Статус: Реализовано и проверено локально. Нет protojson напрямую в HTTP, выдуманных финансовых ответов,
   identity/role из браузера, логирования cookie/CSRF/финансовых payloads.
4. 40–50 минут: unit/race/lint и HTTP→mTLS контрактные тесты, OpenAPI schemas
   успешных/ошибочных responses, точные dates/UUID/money, pending/success/failed,
   неизвестный enum/пустой oneof → безопасная ошибка зависимости.
   Статус: Проверки пройдены. Проверены отказ Auth/Core, deadline/cancellation, unknown
   outcome POST не получает нового ключа автоматически. GET не запускает ML.
5. 50–60 минут: настоящий E2E при готовности зависимостей; иначе подробный
   upstream-blocker report плюс проверенный API adapter, без ложного успеха.
   Статус: Реальный E2E не выполнен, upstream-blocked. Реальный запуск требует provisioned mTLS/Auth/Core/ML,
   durable forecast jobs/results и компании с валидной историей/cutoff EOD.
   Runtime не разворачивать этим поручением. README/runbook должен позволять
   повторить проверку после запуска сервисов без новой реализации API.

## Файлы и проверки

services/api/internal/{client/core,handler/http,config,app,gen/core};
services/api/scripts/{generate.py,check.sh}; integration/tests;
.github/workflows/api-ci.yml (Core proto path gate); API README и этот план.
Новые пакеты/зависимости вводить только по необходимости, private implementations,
consumer-side interfaces, ручная сборка. Референс: существующий Auth API adapter
и maxhack gateway lifecycle, без переноса Kafka/чужих моделей.

Проверки: generation drift, go build/test/race/vet, полный pinned golangci-lint,
Ruff/check/format, process smoke/OpenAPI, existing coverage gate и govulncheck.
Caveman и Ponytail последовательно, без subagents в этом боковом обсуждении;
исправления и повтор соответствующих проверок. Секреты/PKI только временные.
Статусы результата: implemented/tested/upstream-blocked; не merged/deployed.

## Результат продолжения — 2026-10-24

Финансовый HTTP adapter реализован; новый код остаётся в этом worktree, без
commit/push. Сверены исходники, а не исторические статусы старого плана:
Core Company/GetDefaultAccount и ручные writes C3–C5 опубликованы в отдельной
ветке `globalarray/feat/core_company_bootstrap` (`0e46f13` при исходной сверке).
Наличие этих методов не доказывает готовность прогнозных jobs/worker или Auth.
Прогнозные handlers в другом worktree могут развиваться параллельно; здесь они
не изменялись. Реальный E2E требует отдельного запуска актуальных сервисов.

Выполнены последовательные локальные Caveman/Ponytail review, без подагентов.
Исправлен обнаруженный маршрутный дефект: financialRoute больше не обслуживает
`/companies` без `/api/v1/`; regression tests проверяют 404 без upstream вызовов.
Дополнительных абстракций/зависимостей не добавлено.

Добавлен TestFinancialHTTPThroughCoreMTLS: реальная Core mTLS-сеть и HTTP handler
проверяют company/account/list, два POST с исходным ключом и write context,
queued → succeeded → одинаковое повторное чтение и отказ Core. RPC-server здесь
явно synthetic fixture; он не доказывает durable job/replay или ML inference.

Полный scripts/check.sh — exit 0: Auth/Core generation drift, build, unit/race,
vet, fmt/full lint (0 issues), Ruff, 3 data-contract tests, отдельные HTTPS/mTLS
Auth процессы и 36 HTTP-ответов по OpenAPI (20 Auth process responses и 16
financial HTTP handler responses). Coverage: statements 989/1102 = 89.75%,
executable block lines 1028/1137 = 90.41%. govulncheck — No vulnerabilities found;
go mod verify — all modules verified. Реальный browser/Auth/Core/ML E2E,
deployment и опубликованный PR этим результатом не заявляются.

Результат ожидает ревью владельца. Следующий шаг интеграции — provisioned запуск
настоящих зависимостей и проход POST → polling → сохранённый финансовый результат.

### Исправления Caveman review — 2026-10-24

По поручению владельца исправлены два P2. Succeeded с readiness=insufficient
теперь отвергается как некорректный upstream ответ (503); честный failed с
INSUFFICIENT_HISTORY остаётся доступен для чтения. Метрики сверяются с дневными
балансами: minimum/closing, max(0,-minimum), дата и сумма первого дефицита и
отсутствие first_gap при неотрицательных точках. Первый дефицит не обязан
совпадать с максимальной нехваткой; MinInt64 отклоняется без переполнения.
API не подменяет данные Core исправленными цифрами.

Добавлены HTTP regression tests для обоих P2, неправильных даты/суммы first_gap,
скрытого/выдуманного дефицита, minimum/closing/max, int64 границы и валидных
ready/limited/failed состояний. Старый presence fixture исправлен: его first_gap
теперь соответствует настоящей отрицательной дневной точке.
Повторный полный scripts/check.sh — exit 0, 0 lint issues, race/mTLS и 36
OpenAPI responses — PASS. Coverage: 1003/1117 statements (89.79%),
1042/1152 executable block lines (90.45%). Правки локальные, без публикации.
