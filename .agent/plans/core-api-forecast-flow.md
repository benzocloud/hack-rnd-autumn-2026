# Core: финансовый проход текущего API worktree

Дата: 2026-10-24. Основание: владелец поручил проверить Go API worktree и
покрыть его сценарий gRPC. Работа в существующем Core worktree/ветке
globalarray/feat/core_company_bootstrap от 0e46f13; API/frontend/alites не менять.
Референсы: текущие Core feature services/app-owned sqlx Tx, ML client и C3–C5.

## Сверенный сценарий и границы

API worktree api-financial-flow, ветка globalarray/feat/api_financial_flow,
база 01ecfc0 и локальные изменения, план .agent/plans/go-api-hour-flow.md.
Исходники financial.go/router.go вызывают именно:

| REST | RPC | Core сейчас |
| --- | --- | --- |
| GET /companies | Auth.ListMemberships → Core.ReadCompany | Core готов; Auth отдельно |
| GET /companies/{id} | Core.ReadCompany | Готов |
| GET /companies/{id}/account | Core.GetDefaultAccount | Готов |
| POST /companies/{id}/forecasts | Core.CreateForecast | Реализован локально, на ревью |
| GET /companies/{id}/forecasts | Core.ListForecasts | Реализован локально, на ревью |
| GET /companies/{id}/forecasts/{forecast_id} | Core.GetForecast | Реализован локально, на ревью |

Путь: вход → подготовленная компания/история/EOD → 30-дневный прогноз →
polling/график → повторное чтение. API scope account получает только из Core.
Создание компании/импорт/платежи/список операций/сценарии в этот API-срез не входят.
Наличие HTTP handlers/bindings не доказывает browser E2E: real Auth/frontend
проверяются отдельно. HTTP API → Core TCP mTLS → PostgreSQL → Python ML проверен
интеграционно с явным Auth double. Контракт protobuf/REST не менять.

## C6.1 — финансовый ML adapter

Статус: На ревью. До этого подэтапа клиент обрабатывал только history baseline,
хотя Python Predict уже поддерживает mean_90_components_v1. Продуктовый pipeline
должен явно выбирать финансовую версию даже с недоступным календарём (ADR-029).

- Добавить transport-neutral application input/result для финансового расчёта;
  private typed Go ML adapter использует тот же Predict, deadline/2 MiB и mTLS.
- Передать 90 подтверждённых дней, неизменяемые операции, model configuration
  и явную calendar availability. Вход без доказанной coverage отклонить;
  не считать отсутствие операции доказательством нулевого дня.
- Первый adapter соответствует существующему FinancialSnapshot: календарь
  исторически недоступен, это явная limitation той же финансовой модели.
  Подтверждённый current calendar — отдельное расширение snapshot, не фикция.
- Проверить весь echo, 30 последовательных дат/точную арифметику, probability
  not_calibrated и отсутствие baseline fallback. Не рекламировать residual
  components как подтверждённые recurring/sales или сценарные targets.
- Проверки: unit/race/full lint и настоящий Python Predict по mTLS. Затем отчёт
  и остановка на ревью; C6.1 сам по себе не закрывает три отсутствующих RPC.

## C6.2 — durable jobs и подготовка входа

Статус: Реализован локально, ожидает ревью владельца. После C6.1.
Создать сохраняемую coverage provenance и snapshot/history/calendar view,
заморозить financial model context, idempotent enqueue/accepted response.
Claim/lease/recovery при рестарте, bounded worker, ML вне SQL Tx, только worker
меняет queued/running на succeeded/failed. Ошибки/результаты сохраняются;
повреждённый ответ не становится succeeded. Неполная история → честный отказ.
Нет внешнего брокера, прямого SQL из service или переприсвоения исторических EOD.

## C6.3 — три RPC и проверка границы API

Статус: Реализован локально, ожидает ревью владельца. После C6.2.
Create/Get/ListForecast: Auth-before-SQL/replay, FINANCIAL_WRITE/CSRF для create,
FINANCIAL_READ для чтений, default account/tenant scope, API-only mTLS, cursor
actor/company/filter-bound, stable sort, immutable input/result и current stale.
Accepted всегда queued/poll_after_ms≥1000, включая replay после завершения.
Get не запускает ML. Typed states/money/null/errors соответствуют API validators.
Проверить PostgreSQL/race/restart/concurrent idempotency/worker failure + реальный
Core→ML и HTTP adapter с Core runtime, не объявляя Auth double настоящим Auth.

Уточнение владельца «так добей core»: ревью разработчиком отложено до завершения
всего согласованного API forecast flow C6.1–C6.3; промежуточные проверки и
агентские ревью сохраняются. Commit/push, новый PR, merge и deployment отдельно
не разрешены. Это не расширяет срез на остальные пока не используемые API RPC.

## Результат C6.1 — 2026-10-24

В Core добавлены transport-neutral FinancialModelInput/Result, Coverage и
проекции component/evidence. Financial input принимает существующий immutable
snapshot identity/history, явное coverage provenance и scoped domain операции.
Клиент использует существующий ForecastService/Predict с закреплёнными
mean_90_components_v1/calendar_replacement_v1/configuration. Внутренний Go-метод
PredictFinancial — адаптер этого же RPC, отдельный protobuf endpoint не создаётся.

Проверяется наличие 90 последовательных покрытых дней, observed cutoff EOD,
совпадение сумм операций с дневными потоками, scope/UUID/версии/category ownership,
точный reconstructed balance и допуск ±1 только для observed EOD. Покрытие
не выводится из пустоты: будущий job producer должен сохранить его основание.
Передача historical calendar сейчас явно UNAVAILABLE с ограничениями;
изменения snapshot для current/known calendar этим подэтапом не заявляются.

Ответ проходит независимую проверку identity/model/calendar echo, 30 дат,
округлённых средних/сумм и signed int64 balance arithmetic, first gap, probability
not_calibrated и обязательных предупреждений. Evidence ссылается только на
исходные операции; component source/category/regularity и суммы по дням проверены.
Residual компоненты не превращаются в подтверждённые recurring/sales или targets
сценария. Empirical bands проверяются как uncalibrated metadata и не выдаются
публичными confidence intervals. При отказе финансовой модели baseline не вызывается.

Проверки — PASS:

- Полный Core task check: build, fmt, unit/race, vet, lint, 0 issues.
- Полный lint с mlintegration-тегами: 0 issues.
- Настоящий Python Predict по mTLS в отдельном локальном процессе: финансовый
  профиль с обычной и reconstructed history, подтверждённая нулевая история
  с observed zero cutoff EOD. Старый явный baseline также проходит.
- Негативные unit/race: нет coverage/неполные потоки, дубли операций, неверные
  даты/model/horizon/coverage, corrupted echo/amounts/components/evidence/bands/
  probability, отмена/deadline и отсутствие fallback после FailedPrecondition.
- Python bindings сгенерированы из canonical proto только в /private/tmp;
  первое падение старта из-за отсутствующих bindings устранено тестовой
  подготовкой, ML checkout/зависимости/контракты не менялись.
- git diff --check — PASS. БД/schema/SQL в C6.1 не менялись; PostgreSQL suite
  этого подэтапа повторно не запускался.

Независимый Caveman-review: подтверждённых дефектов нет; review-агент тесты не
запускал. Локальный Ponytail-review: используется тот же connection/RPC/errors,
новых библиотек и общего framework не добавлено, обязательные проверки сохранены.
При локальной сверке исправлен reconstructed-day rounding и добавлен regression.

C6.1 на ревью владельца вместе с реализованными C6.2/C6.3 ниже.
Принятые C3–C5 уже опубликованы; весь C6 остаётся локальным без
commit/push/merge/deploy.

## Результат C6.2/C6.3 — 2026-10-24

Добавлены migration 00006, domain job/coverage DTO, Tx-bound forecastjobs SQL,
feature-local orchestration/receipt/codec и три Forecasting RPC. App владеет
sqlx Tx/pool и ML connection; потребительские интерфейсы узкие, service без SQL.
Очередь сохраняется в PostgreSQL, внешний брокер не нужен. Claim использует
SKIP LOCKED и lease 60 секунд; finish проверяет token/expiry и берёт время БД.
ML вызывается вне Tx с deadline; shutdown отменяет worker до закрытия pool.

Create: Auth FINANCIAL_WRITE/CSRF → default account → reserve key → account lock
→ exact-revision coverage + observed cutoff EOD → immutable snapshot/model
→ queued job + receipt → commit. Replay повторно проверяет доступ и возвращает
исходный accepted ID/status/poll interval. Нет coverage — отказ, без фиктивных
нулевых дней. Неподдерживаемый horizon отклоняется, а не обрезается.

Get/List: FINANCIAL_READ, API-only mTLS, tenant/default-account scope, stable
keyset/actor/filter-bound cursor. Get читает сохранённый результат, не ML;
current data version определяет stale, исходный snapshot/result не меняются.
Результат версионирован; повреждённая схема, арифметика или probability reason
не выдаются как успешный прогноз. List не загружает тяжёлый result payload.

Проверки — PASS:

- Полный Core build/fmt/unit/race/vet/lint; lint с integration,mlintegration — 0 issues.
- Полный PostgreSQL suite с race/shuffle: 8 concurrent enqueue → 1 job/receipt,
  replay после завершения/reconnect, stale после записи, CSRF/revoked/scope,
  safe failed result, coverage revision/range, lease/reclaim/stale token,
  stable pagination и отсутствие result payload в summary.
- Настоящий Python ML по mTLS: durable worker → 30 точек → сохранение →
  чтение новым экземпляром Core/list; worker корректно отменяется.
- Отдельный бинарник API из актуального локального API worktree: HTTP company,
  account, forecast202 → queued → succeeded → list/replay; Core по TCP mTLS,
  настоящие PostgreSQL/Python ML. API checkout не изменялся.
- Regression: повреждённые schema version, balance arithmetic, probability
  reason отклоняются при чтении; cursor другого actor/scope/filter отклоняется.

Ограничения: требуется доверенная подготовка history coverage; writer/import
в этот срез не входит. Historical calendar UNAVAILABLE, readiness limited,
probability not_calibrated. Public recommendations/components/scenario targets
пока недоступны; payment/what-if/import RPC вне текущего API-сценария. Auth double
не доказывает интеграцию с реальным Auth или frontend/browser E2E.

Независимый Caveman-review C6: подтверждённых correctness/security дефектов нет.
Ponytail-review: убран повторный SQL read immutable snapshot перед сохранением
ML-результата; encoder стал чистой функцией, validation сохранена. Review-агенты
тесты не запускали. Финальные проверки выполняет основной агент.

ML PR #33 проверен отдельно и по явному согласию владельца слит в main,
merge cc6d034bba5befbd90e583b2ca0394d488836003. Там только тесты/docs; модель и
protobuf не менялись. Core worktree пока не интегрирован с этим новым main:
перед будущей публикацией потребуется обычная синхронизация ветки.
