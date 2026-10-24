# Saldo Auth — этап 3.2

Каркас Go-сервиса с собственной PostgreSQL: конфигурация, pool, явные SQL-миграции,
health/readiness и ограниченное завершение. Auth RPC и браузерные auth endpoints
ещё не реализованы. Контракт — [Auth](../../docs/auth/contract.md).

Go 1.27.3, PostgreSQL 18; sqlx 1.4.0, pgx/v5 5.11.0 через `pgx/stdlib`,
goose/v3 3.29.0. Зависимости закреплены
в go.mod/go.sum. PostgreSQL 18 нужен в том числе для uuid_extract_version.
Модуль независим от Core; Auth следует общим правилам sqlx/layout из
[инструкций](../../.agent/instructions.md). Native pgxpool не используется.

## Запуск

Команды выполнять в services/auth. Создать **отдельную Auth БД и роли**, без
доступа к Core. Migration role имеет DDL, runtime role — CONNECT/USAGE,
SELECT/INSERT/UPDATE/DELETE таблиц и доступ на чтение goose_db_version.
Права и DDL не выдаются автоматически сервером. Provisioning — ответственность
окружения, пример ниже использует собственную локальную scratch БД.

Настройки через окружение/secret manager; значения паролей и ключей не хранить
в shell history, .env внутри Git, аргументах CLI или сообщениях. AUTH_DATABASE_URL
задать безопасным способом, затем:

```sh
go run ./cmd/migrate
go run ./cmd/auth
```

Первая команда применяет pending **up** migrations, берёт PostgreSQL advisory
session lock и завершается. Migration work ограничен минутой;
снятие lock использует отдельный deadline AUTH_QUERY_TIMEOUT даже после отмены
основного контекста.
Сервер не запускает DDL автоматически: перед bind проверяет соединение и точную
версию схемы. Старый/будущий/отсутствующий schema
version запрещает запуск; live readiness также перепроверяет его.
Для обновлений остановка/совместимость версий планируются отдельно, не выдаются
за zero-downtime. Понижение версии CLI не предоставляет; Down SQL предназначен
для отдельно рассмотренного rollback, не для обычного запуска.

| Переменная | Значение по умолчанию / ограничение |
| --- | --- |
| AUTH_DATABASE_URL | Обязательный PostgreSQL URL с host/database/user и sslmode=verify-full |
| AUTH_ENV | production; local разрешает sslmode=disable только на loopback |
| AUTH_HTTP_ADDRESS | 127.0.0.1:8082; literal IP/localhost, port 1–65535 (0 только local) |
| AUTH_DB_MAX_CONNECTIONS | 5, диапазон 1–100; idle=5m, lifetime=30m |
| AUTH_STARTUP_TIMEOUT | 5s, диапазон 50ms–5m |
| AUTH_QUERY_TIMEOUT | 1s, диапазон 50ms–5m; общий deadline для transaction callback |
| AUTH_SHUTDOWN_TIMEOUT | 5s, диапазон 50ms–5m |

TLS prefer/allow/require без проверки имени не поддерживаются. URL может содержать
sslrootcert/sslcert/sslkey, connect_timeout, search_path, application_name;
переопределения host/user/password через query запрещены. Фактические fallback
адреса также проверяются. Не подключать Auth к proxy, теряющему семантику
session advisory locks, при запуске миграций.

HTTP:

- GET /health/live — процесс обслуживает запросы, 200.
- GET /health/ready — startup завершён, БД доступна и schema version совпадает,
  200; иначе 503 с общей ошибкой. No-store, без DSN или DB diagnostics.
- Остальные маршруты отсутствуют; listener предназначен только для внутренней сети.

Ctrl+C/SIGTERM снимает readiness, дренирует HTTP, при timeout принудительно
закрывает HTTP/context, затем закрывает DB pool. Запросы к БД получают deadline.
Ошибки startup закрывают уже открытые pool/listener; source errors/DSN не логируются.

## Хранилище

`internal/app/postgres` создаёт/configure/Ping готовый `*sqlx.DB`; composing
`internal/app` владеет `Close`, в том числе при ошибке startup или миграции.
Repository не открывает и не закрывает pool. `user.NewRepository` возвращает приватную
реализацию и принимает готовый `sqlx.ExtContext` (`*sqlx.DB` или `*sqlx.Tx`).
Узкие интерфейсы принадлежат потребителям; собственного зеркального интерфейса
провайдер не публикует. Пользовательские операции, записи и SQL принадлежат
`internal/repository/user`; `internal/repository/postgres` содержит общую
инфраструктуру PostgreSQL: schema check, миграции, транзакции и общие ошибки.

```text
internal/repository/
  user/
    repository.go           # приватная реализация и конструктор
    insert_user.go
    user_by_email.go
    user.go                 # storage projection без db-тегов
    errors.go               # ошибки пользовательских операций
    queries.go              # embed только пользовательских SQL
    record/user.go          # запись БД с db-тегами
    sql/insert_user.sql
    sql/user_by_email.sql
  postgres/
    schema.go               # приватный schemaRepository и NewSchemaRepository
    record/schema_version.go
    queries.go              # embed schema_version.sql
    sql/schema_version.sql
    transaction.go
    migrate.go
    errors.go
```

Преобразование `record.User` в `user.User` выполняет родительский пакет `user`;
пакет записей не импортирует родителя. Каждый repository хранит собственные SQL
в своём `sql/` и embed в `queries.go`. Отдельная `app.Migrate` владеет соединением
миграционной команды, инфраструктурный пакет получает готовый DB.
Проверка схемы создаётся через `postgres.NewSchemaRepository`; её запись БД
принадлежит `postgres/record`. Ошибки поиска, email conflict и invalid user record
принадлежат `user`; общая недоступность БД — `postgres`.

Миграция 1: users, credentials, security_contexts, sessions,
verification_challenges, reset_tokens и goose history. Credential version
принадлежит users; sessions сохраняют snapshot, без FK на изменяемую version.
Security context хранит hash cookie, nonce/key_id CSRF, срок и отзыв; session
не может ссылаться на pre-auth context. Challenge хранит HMAC/key_id и 0–5 ошибок,
TTL ≤10m; reset — hash и TTL ≤15m. Код/ссылка/cookie в открытом виде не хранятся.
Ключи HMAC/AES/CSRF, методы verify/login/reset и очистка ещё относятся к 3.3/3.4.

`user.User` — storage projection, а операции repository — низкоуровневое
хранилище, не регистрация. InsertUser сохраняет
уже нормализованный email и UUIDv4; БД проверяет их и уникальность email.
WithTx передаёт bounded context и `*sqlx.Tx`; callback создаёт repository над этим
Tx. Ошибка callback/cancellation делает rollback; успешный callback — commit.
До добавления consent/evidence/outbox в 3.3/3.4 пользовательская регистрация
через это хранилище не предоставляется. Memberships/invitations/Company остаются
за последующими этапами и Core. Готовность health не означает готовность Auth flow.

## Проверки

```sh
golangci-lint fmt --diff $(git ls-files -- '*.go')
golangci-lint run ./...
go test ./...
go vet -tags=integration ./...
go build ./cmd/auth ./cmd/migrate
go test -tags=integration -count=1 -timeout=90s ./...
```

`.golangci.yml` адаптирует согласованный набор Core/cosmohack с Auth module
prefix. Форматирование, импорты и lint проверяются по всему модулю, включая
тесты и integration build tag (`run.build-tags` в конфигурации). `go vet`
не заменяет полный lint. [Auth CI](../../.github/workflows/auth-ci.yml) задаёт
lint, unit/integration tests с race detector и build; добавление workflow
не подтверждает его успешное выполнение на GitHub.
Фактические результаты и ограничения — в [отчёте 3.2](../../docs/auth/stage-3.2.md);
список команд сам по себе не означает успешный запуск CI.

Integration требует AUTH_TEST_DATABASE_URL на PostgreSQL 18, имя БД заканчивается
на _test. Тест создаёт собственную случайную schema auth_test_<uuid> и удаляет
только её; исходную БД не удаляет. Это должна быть отдельная локальная scratch БД,
не production/Core. Без переменной integration tests явно skipped. Для local
loopback допустим URL с sslmode=disable; настоящие секреты не печатать.

Проверяются миграции пустой схемы и concurrent up, storage constraints, duplicate
email race, rollback, persistence после переоткрытия pool, readiness с настоящей
БД, несовместимая схема и два запуска/остановки сервиса. Unit tests проверяют
TLS/config/redaction, ошибки startup и lifecycle. При доступном C toolchain:

```sh
go test -race -tags=integration -count=1 ./...
```

Pattern provenance: lifecycle/config/readiness и embedded parameterized SQL из
описания references/auth-service в [.agent/references](../../.agent/references/go-services.md),
HEAD e800594; отдельный PostgreSQL app-пакет/sqlx и Go-lint — из согласованного
cosmohack layout, зафиксированного в Core-снимке `20d2adc0a44be6839ee5fef3351c04ccd746e20f`.
Исходники/лицензионно невыясненный код не копируются. Наша реализация
исправляет cleanup, timeout, SQL и TLS проблемы референса.

## D1/Auth и изолированный demo-profile

Минимальные RPC, server-only параметры, поведение demo/strict, UI/API поле и
оставшиеся Core onboarding зависимости описаны в [интеграционном срезе](../../docs/auth/demo-integration.md).
Профиль по умолчанию выключен; mock касается только email-кода. Проверка пароля,
серверная сессия, CSRF и права компании остаются настоящими.
Миграция 2 обязательна для RPC-среза; исторический отчёт 3.2 описывает прежнюю схему.
