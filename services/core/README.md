# Go Core

Основа модульного DDD-монолита. Подэтап 2.1 содержит env-конфигурацию и
локальный `pkg/log`; подэтап 2.2 добавляет доменные модели денег, Ledger и Payments.
Подэтап 2.3 включён в main: sqlx pool,
миграции, атомарные операции Ledger/Payments и persistent idempotency.
Подэтап 2.4 принят разработчиком 2026-10-24: FinancialSnapshot.
Прикладная сборка находится в `internal/app`; HTTP/gRPC ещё не реализованы.
Сервер остаётся в следующих подэтапах
[плана](../../.agent/plans/go-core-foundation.md).

## Доменные модели

`internal/domain` разделён на пакеты по предметной ответственности;
сущности, связанные методы и их тесты сгруппированы по файлам:

```text
money/     money.go, parse.go, arithmetic.go, errors.go
ledger/    account.go, transaction.go, balance.go
           scope.go, date.go, direction.go, source.go, category.go, errors.go
payments/  payment.go, match.go, errors.go
forecasting/ snapshot.go, errors.go
```

Конструкторы проверяют инварианты; состояние Transaction и ExpectedPayment
закрыто, изменения возвращают новые значения. BankAccount владеет привязками
источников и меняется через методы. Внутренние ID типизированы `uuid.UUID`; новые ID создаются через UUIDv7 в
прикладном слое, SQL хранит их как `uuid`. Внешние ID и Auth actor ID остаются
строками (ADR-024). Модели используют stdlib, google/uuid и
доменные пакеты; в них нет транспорта, SQL или логирования.

Money хранит точные `int64` копейки в RUB. `Parse`, `ParsePositive`,
`ParseNonnegative` проверяют десятичную запись и диапазон;
`Add`/`Sub` отвергают переполнение. Ограничения положительности проверяются
при создании операций/ожиданий, отрицательные остатки допустимы.

Ledger отделяет company/account scope, внешнюю компанию источника, бизнес-дату
и фактическое время записи/импорта. Привязка дополнительного источника ожиданий
не выбирает источник фактов. Повтор импорта сравнивается по внешней идентичности
и содержимому; повторные EOD не складываются. Версия ресурса и `data_version`
счёта разделены; write-транзакция сохраняет их атомарно. Внешний EOD и факты
закрепляют один factual namespace; ручной EOD и ожидания его не выбирают.

Payments ограничивает match остатком ожидания и нераспределённой суммой факта,
проверяет scope/направление и возвращает новое состояние вместе с PaymentMatch.
Просрочка — признак для заданной даты, без автоматического перехода в факт.
Отмена сохраняет историю распределений. Переданный в match общий объём уже
распределённых средств должен быть прочитан для точного scoped transaction
под блокировкой счёта в PostgreSQL-транзакции. Проверка модели не заменяет
защиту от конкурентных записей, storage uniqueness и идемпотентность команды.


## Прикладные команды и результаты

В `internal/service/ledger` и `internal/service/payments` команды и результаты
расположены рядом с операциями. Общего `service/command` больше нет (ADR-026).

| Операция | Результат | Значимые поля |
| --- | --- | --- |
| `ledger.CreateTransaction` | `CreateTransactionResult` | `TransactionID`, `TransactionVersion`, `DataVersion` |
| `ledger.RecordBalance` | `RecordBalanceResult` | `Date`, `DataVersion`; у наблюдения нет отдельного UUID |
| `payments.CreatePayment` | `CreatePaymentResult` | `PaymentID`, `PaymentVersion`, `DataVersion` |
| `payments.MatchPayment` | `MatchPaymentResult` | `MatchID`, `PaymentVersion`, `DataVersion` |

Каждая команда явно содержит `ActorID` и `IdempotencyKey`. Сервис предполагает
ActorID из доверенного входного адаптера; сам адаптер ещё не реализован.
Ручные операции требуют оба поля, импорт допускает отсутствие обоих, но не
одного из пары. Проверка данных/генерация UUID и приватные consumer-side ports
сохранены. Это внутренние application DTO без JSON-тегов; HTTP/protobuf mapper
и сериализация сохранённых результатов разделены: private persistence codecs в
`service/{ledger,payments}/*_receipt.go`; будущий HTTP/protobuf mapper
появится у транспорта. REST-контракт не менялся.

## Атомарная запись

`app.New(ctx, cfg)` открывает pool и передаёт сервисам явно собранные функции
`withinTx`; приложение владеет `Close`. Конструктор не применяет
миграции. Создание компаний/счёта будет подключено после контракта онбординга Auth;
fixture создаёт тестовый счёт отдельно.

Сервисы координируют четыре финансовых сценария: читают состояние через private
consumer-side ports, проверяют версии, вызывают доменные методы и формируют
operation-specific Result. SQL-репозитории возвращают доменные модели, версии
и признак наличия записи; service command/result DTO они не принимают и не возвращают.

В Ledger и Payments определено по одному публичному callback-типу `Work`.
Private `store` и `commandStore` задают только нужные feature возможности.
Функция из app вызывает общий `app/postgres.WithinTx` и создаёт repository
wrappers для одного Tx. `WithinTx` управляет deadline/Begin/Commit/Rollback;
он не знает команд, моделей, repositories или receipt. Сервис возвращает
результат только после успешного завершения `withinTx`.

Service проверяет persistent replay до account lock и stale guards, вызывает
доменный сценарий только для новой команды и сохраняет private receipt.
`repository/postgres/commands` хранит opaque payload и digest; service DTO и
codecs ему неизвестны. SQL ошибки классифицируются на repository boundary,
ошибки lifecycle — в helper, feature mapping выполняется рядом с app wiring.
SQLx/Tx в service и в context values не передаются. Центрального реестра
repositories, `service/transaction` и operation-specific транзакционных адаптеров нет.

```text
service → withinTx (функция из app) → app/postgres.WithinTx
             Work ← repositories, привязанные к одному Tx
service: replay → domain decisions → persistence → receipt
             Commit → result; любая ошибка → zero result
```

Все четыре прикладные операции используют короткую READ COMMITTED транзакцию
с `CORE_POSTGRES_QUERY_TIMEOUT`. Порядок блокировок: ключ команды → scoped account
`FOR UPDATE` → ресурсы. Под блокировкой счёта проверяются текущие версии,
источник/namespace и доступная сумма распределения. Новые данные, ресурсная версия,
`data_version` и результат команды коммитятся вместе. Ошибка откатывает и
финансовые изменения, и reservation. Неожиданные SQL/driver errors не выдаются
наружу; порт возвращает безопасные application errors либо конкретные domain errors.
Цепочка причин и SQLSTATE сохраняются через errors.Is/As; Error() не печатает
приватные детали driver. Не выводить unwrapped cause в публичный ответ или лог с данными.

Persistent scope: actor + company/account + операция/resource + IdempotencyKey.
Digest считается по каноническому исходному запросу, с явными копейками/валютой,
до подстановки ID/time. Явно переданные значения остаются частью digest.
Результат проверяется до stale-version guard и возвращается после переподключения.
Хранение минимум 24 часа: expiry записывается в БД; очистка пока не запускается,
поэтому сохранённые ключи продолжают воспроизводиться и после этой отметки.
Импорт без ключа защищён отдельно по external identity; повтор сохраняет текущие
resource ID/version и первоначальную created data revision без повторной записи.
Balance хранит собственную created_data_version, чтобы повтор EOD возвращал
исходную revision. Импорт ожидания после match сохраняет remaining/status.

Это внутренние операции для доверенного адаптера. Авторизация и проверка прав
на компанию должны выполняться будущим транспортом при каждом запросе, включая replay.
Очередь, importer и сервер этим этапом не добавлены.

## Финансовый снимок (2.4)

`app.Forecasting.CaptureSnapshot(ctx, {Scope, Cutoff})` сохраняет внутренний
FinancialSnapshot с UUIDv7, account data_version, EOD ровно на Cutoff и фактами
с business date <= Cutoff. Даты трактуются как бизнес-даты Europe/Moscow,
а временные метки сохраняются UTC. Без точного EOD возвращается
`forecasting.ErrMissingBalance`; предыдущий остаток не переносится автоматически.
`GetSnapshot(ctx, scope, id)` читает сохранённый снимок по company/account/ID.
Создание и чтение снимка не меняют финансовую revision счёта.

Все scoped read-проекции и INSERT снимка проходят в одной REPEATABLE READ
read-write транзакции на попытку. `app/postgres.WithinRepeatableRead` повторяет
только SQLSTATE 40001: максимум три попытки в общем QueryTimeout. Это нужно,
в частности, при конкурентной записи account и проверке FK. Callback должен
быть replayable без сетевых вызовов/внешних эффектов. Сервис возвращает результат
после commit. Обычные команды Ledger/Payments используют прежний WithinTx.

Таблица `core_financial_snapshots` добавляется миграцией 00003. Repository имеет
только INSERT/SELECT; повтор ID даёт конфликт. Private JSONB codec версии 1
сохраняет точные int64 суммы, provenance и версии фактов/остатка; доменные модели
не сериализуются напрямую. Снимок владеет копиями срезов; изменение счёта или
новый capture не перезаписывают существующий снимок.

Текущая схема не позволяет восстановить исторические remaining/status платежей.
Поэтому календарь явно исключён с `historical_calendar_unavailable`;
`ExcludedPayments` — количество видимых исключённых строк на момент capture,
не доказанное количество на cutoff. Флаг `historical_availability_unverified`
дополнительно сообщает, что факты по бизнес-дате не доказывают доступность
информации в прошлом. Реальные imported_at/recorded_at сохранены. Пустой календарь
не означает отсутствия обязательств. Снимок ещё не является ForecastInput,
ModelContext или результатом прогноза; адаптер ML и запуск job остаются далее.

## Проверки

Go **1.27.3**, golangci-lint **2.15.0**, [Task](https://taskfile.dev/) **3.48+**
(проверено на 3.48.0). `Taskfile.yml` задаёт точный `GOTOOLCHAIN` в Go-командах,
включая случай другого значения в окружении. Go при необходимости скачивает
этот toolchain. Закреплены google/uuid 1.6.0, sqlx 1.4.0 и pgx 5.11.0; `go.sum` отслеживается.
pgx/stdlib используется как database/sql driver, без native pgxpool.
Pool создаёт `internal/app/postgres`, закрывает вызывающий app; repository
получает готовый DB/Tx. Goose 3.29.0 запускается отдельно через `task migrate-up`.
Установить CLI: `GOTOOLCHAIN=go1.27.3 go install github.com/pressly/goose/v3/cmd/goose@v3.29.0`.
Передать `GOOSE_DBSTRING` через защищённое окружение для целевой БД;
для нестандартного пути использовать `GOOSE=/path/to/goose`. Startup миграции не запускает.

```sh
cd services/core
GOTOOLCHAIN=go1.27.3 go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
task check
```

`check` выполняет build, форматирование, unit/race tests, vet и lint.
`task` или `task --list` показывает доступные команды и их описания.
Из корня репозитория: `task -d services/core check`.
`task fmt` применяет gofmt и goimports; `fmt-check` проверяет оба без записи; `task test`, `task race`, `task vet`, `task lint`
доступны отдельно. База данных для этих проверок не нужна.
Для нестандартного расположения линтера: `task lint GOLANGCI_LINT=/path/to/golangci-lint`.
`task integration` запускает `go test -tags=integration -race -count=1 ./...`.
Integration-проверки покрывают UUID/source CHECK, четыре операции записи,
replay после reconnect, разные digest/scopes, откат после FK/SQL-ошибки,
конкурентный одинаковый ключ и factual namespaces, два match разных ожиданий
на один факт, stale versions, cross-tenant ID и отмену context без утечки pool.
Для снимка проверяются MVCC consistency при конкурентном факте/ожидании/match,
serialization retry без частичных записей, неизменяемость и reconnect roundtrip,
точные границы int64, exact-cutoff EOD, исключение будущего факта и tenant scope.

Использовать только одноразовую БД из `compose.integration.yml`: запустить
`docker compose -f compose.integration.yml up -d --wait postgres`, узнать
случайный порт через `docker compose -f compose.integration.yml port postgres 5432`.
Передать его и тестовые настройки из Compose через `CORE_POSTGRES_*` (host
`127.0.0.1`, database `saldo_core_test`, SSL mode `disable`); отдельно применить
миграции через `task migrate-up` с закреплённым goose CLI 3.29.0, затем выполнить `task integration`.
Перед записью тесты требуют server marker `saldo.test_database=saldo-core-integration`;
без него завершаются ошибкой. Low-level тесты откатывают транзакции;
прикладные тесты коммитят для проверки reconnect/concurrency и удаляют только
свои company-scoped fixtures при завершении. Не прерывать их на постоянной БД.
После проверки: `docker compose -f compose.integration.yml down`.
Реальные/dev БД для этих тестов не подходят.

## CI и контейнер

[Go Core CI](../../.github/workflows/go-core-ci.yml) запускается при изменениях
Core, этого workflow или LICENSE в PR и при push в `main`; доступен ручной запуск.
Четыре последовательных job: `lint` (format/vet/lint) → `test` (race tests
со случайным порядком) → `build` (проверка `go mod tidy`, сборка пакетов,
govulncheck 1.9.0 и Docker build) → `publish` (публикация в GHCR).
Actions закреплены SHA, инструменты — точными версиями; используются
GitHub-hosted `ubuntu-24.04`. PR не получают право публикации.

Dockerfile адаптирует multi-stage/scratch подход из
`references/hack-rnd-2026-spring/services/engine-go/Dockerfile` и
`references/auth-service/Dockerfile`: отдельная компиляция, CA и tzdata в runtime,
статический бинарник, запуск с UID/GID 65532. Go base закреплён digest;
чужие порты, сертификаты и настройки не переносятся.
Контекст — **корень репозитория**; `Dockerfile.dockerignore` допускает только
Core и LICENSE и исключает `.env*`, dataset и локальные референсы.

Сейчас `cmd/core` отсутствует: CI проверяет target `build` и явно пропускает
публикацию. Проверка пакетов в контейнере из корня:

```sh
docker build --target build -f services/core/Dockerfile .
```

После реализации `services/core/cmd/core/main.go` CI автоматически проверит
полный target `runtime`. Если компиляция не пройдёт, публикации не будет.
Локальная сборка будущего runtime:

```sh
docker build --target runtime -f services/core/Dockerfile -t core:local .
```

После успешных проверок push/ручной запуск **на `main`** публикует
`ghcr.io/benzocloud/hack-rnd-autumn-2026/core:sha-<полный commit SHA>` и `:latest`.
Используется встроенный `GITHUB_TOKEN` с `packages: write` только у publish job;
отдельный PAT не нужен. Политика организации должна разрешать этому репозиторию
публикацию пакетов; для ранее созданного package нужно дать репозиторию write-доступ
в его Actions access. Реестр не запускает приложение: deploy на сервер остаётся
отдельным авторизуемым действием.

## Конфигурация

`config.Load()` применяет defaults → env → validation, не читает файлы и не
открывает БД/сеть. `.env.example` описывает переменные; `.env` автоматически
не загружается. Установленная пустая переменная считается значением и проходит
проверку, а не молча заменяется default.

| Переменная | Default / смысл |
| --- | --- |
| `CORE_LOG_LEVEL` | `info`; синтаксис `slog.Level`, включая числовые смещения |
| `CORE_LOG_FORMAT` | `json`; также `text` |
| `CORE_POSTGRES_HOST` | Обязательный hostname или IP; IPv6 без квадратных скобок, порт отдельно |
| `CORE_POSTGRES_PORT` | `5432`; целое от 1 до 65535 |
| `CORE_POSTGRES_USER` | Обязательный пользователь |
| `CORE_POSTGRES_PASSWORD` | Обязательный пароль; передаётся без обрезки пробелов |
| `CORE_POSTGRES_DATABASE` | Обязательное имя БД |
| `CORE_POSTGRES_SSL_MODE` | Обязательный режим: disable, allow, prefer, require, verify-ca, verify-full |
| `CORE_POSTGRES_MAX_CONNS` | `10`, положительное int32 |
| `CORE_POSTGRES_MAX_IDLE_CONNS` | `2`, максимум idle connections, от 0 до MAX_CONNS; не минимальный размер pool; явный `0` отключает idle connections |
| `CORE_POSTGRES_CONNECT_TIMEOUT` | `5s`, положительная duration |
| `CORE_POSTGRES_QUERY_TIMEOUT` | `5s`, положительная duration |
| `CORE_SHUTDOWN_TIMEOUT` | `10s`, положительная duration |

`Postgres.DSN()` собирает URL через `url.UserPassword` и `net.JoinHostPort`,
экранирует credentials/имя БД и использует поле `SSLMode`.
Исходные настройки — единственный источник; готовая строка отдельно не хранится.
Переменная `CORE_POSTGRES_DSN` больше не читается: настройки задаются полями выше.
Значения настроек не включаются в ошибки. Не логировать Config/Postgres целиком
или результат `DSN()`: они содержат пароль. `sslmode=disable` в примере задаётся явно для локальной БД;
для удалённой БД использовать проверку сертификата и имени `sslmode=verify-full`.
Проверка настроек здесь не доказывает доступность БД; driver-specific validation
и Ping выполняются в `internal/app/postgres.Open` через sqlx/pgx driver.
Прежняя переменная `CORE_POSTGRES_MIN_CONNS` больше не читается.

## Логирование

```go
cfg, err := config.Load() // обработать err до сборки приложения
logger, err := corelog.New(os.Stdout, cfg.Log.Level, cfg.Log.Format)
// обработать err; передать logger в конструкторы через internal/app
```

`corelog` — alias пакета `pkg/log`. Он возвращает `*slog.Logger`, не меняет
глобальный logger и не владеет writer. JSON/text реализованы стандартными
handlers. Референс: `references/maxhack/services/gateway/pkg/log/logger.go`,
MIT, Copyright 2026 BenzoCloud; применим корневой LICENSE этого репозитория.
Текущая сборка не зависит от наличия `references/`.

## Правила Go и SQL

`.golangci.yml` адаптирован из `references/cosmohack/backend/.golangci.yml`:
корректность/ошибки, контексты и ресурсы, вложенность, стиль, тесты и logging.
Дополнительно goimports разделяет stdlib, внешние и локальные imports.
`task check` проверяет весь модуль (`./...`), включая тесты.
Объявления embed живут в package-local `internal/repository/postgres/*/queries.go`,
SQL — в соседнем `sql/`; pool не создаётся внутри repository.
`New` возвращает приватную реализацию сервиса; узкие интерфейсы определяются
на стороне потребителей. Эти правила закреплены в `.agent/instructions.md`.

### ML client (Core → ML)

`internal/client/ml` borrows a reusable gRPC connection owned by app. App must
supply mTLS credentials and close the connection; the client does not dial per
request. `Predict` applies a maximum 30-second deadline (a shorter parent deadline
wins), 2 MiB message limits, validates snapshot/model echoes and projection
arithmetic, and returns safe classified errors with inspectable causes.

The canonical Predict originated in ML PR #27,
`994ed6e0c33969020e4fa911b1125a0fdfe7f6ea`, and now also defines normalized
financial inputs/results for the financial mean_90_components_v1 model. There is one RPC;
unsupported financial context fails explicitly, without baseline fallback. Current model: `mean_90` /
`mean_90_v1` / `daily_snapshot_v1`. It requires 90 verified consecutive daily
observations/confirmed no-operation days. It rejects a known-payment calendar;
it supplies no calibrated probability. Missing history is never filled with
zeros. FinancialSnapshot alone does not yet supply this verified daily coverage.
This client is not yet wired into forecast persistence or a running Core server.

Generate the service-local Core/Auth/ML bindings from the repository root:

```sh
buf generate --template buf.core.gen.yaml
```

Use Buf 1.75.0; the template pins local Go plugins to v1.36.12 / v1.6.2.
The initial API → Core contract and remaining runtime gaps are documented in
[Core contract](../../docs/core/contract.md). Canonical proto lives at repository
root; consuming modules generate their own packages, never import another
service's internal bindings.

Cross-language regression, with a separately checked-out ML source and isolated
Python environment containing its pinned `requirements-rpc-dev.txt` dependencies
and generated Python bindings:

```sh
CORE_ML_TEST_PYTHON=/absolute/path/to/venv/bin/python \
CORE_ML_TEST_SOURCE=/absolute/path/to/ml-checkout/services/ml \
  go test -tags=mlintegration -race -count=1 -run TestPythonPredictMTLS ./internal/client/ml
```

The test creates temporary certificates, starts the real Python server on an
OS-selected loopback port with mTLS and a `saldo-core` SAN, checks the prediction,
and terminates its own process. The ordinary CI unit job does not run Python;
missing integration environment variables cause an explicit skip.
