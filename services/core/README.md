# Go Core

Основа модульного DDD-монолита. Подэтап 2.1 содержит env-конфигурацию и
локальный `pkg/log`; подэтап 2.2 добавляет доменные модели денег, Ledger и Payments.
Подэтап 2.3 в работе: подготовлены sqlx pool в `internal/app/postgres`, SQL Ledger
и миграции; write-path и полный PostgreSQL integration suite ещё не закончены.
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
счёта разделены; прикладная транзакция будет координировать их в 2.3.

Payments ограничивает match остатком ожидания и нераспределённой суммой факта,
проверяет scope/направление и возвращает новое состояние вместе с PaymentMatch.
Просрочка — признак для заданной даты, без автоматического перехода в факт.
Отмена сохраняет историю распределений. Переданный в match общий объём уже
распределённых средств должен быть прочитан для точного scoped transaction
под блокировкой в будущей PostgreSQL-транзакции. Проверка модели не заменяет
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
и сериализация сохранённых результатов определяются на соответствующих границах.
Домен и сохранённый REST-контракт этим изменением не меняются. Реализации
транзакционных storage ports по-прежнему требуют завершения подэтапа 2.3.

## Проверки

Go **1.27.3**, golangci-lint **2.15.0**, [Task](https://taskfile.dev/) **3.48+**
(проверено на 3.48.0). `Taskfile.yml` задаёт точный `GOTOOLCHAIN` в Go-командах,
включая случай другого значения в окружении. Go при необходимости скачивает
этот toolchain. Закреплены google/uuid 1.6.0, sqlx 1.4.0 и pgx 5.11.0; `go.sum` отслеживается.
pgx/stdlib используется как database/sql driver, без native pgxpool.
Pool создаёт `internal/app/postgres`, закрывает вызывающий app; repository
получает готовый DB/Tx. Goose — отдельный migration CLI в 2.3.

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
Реализованы регрессии: сохранение отсутствующего счёта возвращает `sql.ErrNoRows`,
CHECK отвергает несовместимые authority/namespace; валидные состояния проходят
UUID roundtrip и SaveAccount. Полный набор проверок подэтапа 2.3 ещё не готов.

Использовать только одноразовую БД из `compose.integration.yml`: запустить
`docker compose -f compose.integration.yml up -d --wait postgres`, узнать
случайный порт через `docker compose -f compose.integration.yml port postgres 5432`.
Передать его и тестовые настройки из Compose через `CORE_POSTGRES_*` (host
`127.0.0.1`, database `saldo_core_test`, SSL mode `disable`); отдельно применить
миграции закреплённым goose CLI 3.29.0, затем выполнить `task integration`.
Перед записью тесты требуют server marker `saldo.test_database=saldo-core-integration`;
без него завершаются ошибкой. Данные каждого теста откатываются транзакцией.
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
Объявления embed живут в `internal/repository/postgres/ledger/queries.go`,
SQL — в соседнем `sql/`; pool не создаётся внутри repository.
`New` возвращает приватную реализацию сервиса; узкие интерфейсы определяются
на стороне потребителей. Эти правила закреплены в `.agent/instructions.md`.
