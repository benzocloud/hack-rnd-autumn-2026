# Auth 3.2 — каркас и собственное хранилище

Дата: 24 октября 2026 года. Статус: **На ревью** в PR #17.
Основание перехода: владелец прямо поручил выполнить 3.2 после исправлений и
ревью 3.1 (`29477c3`). Это принятие основания 3.1/ADR-026 владельцем, не апрув
от агентов и не утверждение о завершённой интеграции API/Core.

## Результат

- Самостоятельный Go-модуль [services/auth](../../services/auth/README.md).
  Тонкие cmd/auth и cmd/migrate, ручная сборка, pgx pool, HTTP health/readiness.
- Defaults → environment → validation. TLS verify-full; plaintext только при
  explicit local + loopback. Ограничены pool, query/startup/shutdown и HTTP.
  Config formatting/slog/errors не выводят DSN/пароль или PostgreSQL Detail.
- Startup проверяет pool и schema version до bind, очищает ресурсы при ошибке.
  Readiness перепроверяет БД/схему; shutdown сначала снимает готовность/дренирует
  HTTP, затем закрывает pool, с forced cancel/close на deadline.
- [Миграция 1](../../services/auth/migrations/00001_auth_foundation.sql): шесть
  Auth таблиц и goose history. Users UUIDv4/normalized unique email,
  credential_version, credentials, security contexts, sessions с snapshot version,
  verification challenge HMAC/key_id/attempts/expiry и отдельные reset token hashes.
  SQLSTATE/constraint mapping не возвращает значения полей.
- Схема применяется отдельным cmd/migrate с advisory session lock; сервер не
  запускает DDL. WithTx обеспечивает rollback и bounded context. Базовые
  InsertUser/UserByEmail — storage primitives, публичного Register нет.

Численные auth TTL и API/protobuf 3.1 не менялись. Domain use cases, Auth RPC,
consent/evidence/outbox, password hashing, verify/reset, memberships/invitations,
Core orchestration и Resend sends остаются в 3.3–3.6. Каркас их не имитирует.

## Фактически выполнено

1. gofmt, `go test ./...`, `go test -tags=integration -count=1 -timeout=90s ./...`,
   `go vet ./...`, build обоих cmd. Go 1.27.3, pgx 5.11.0, goose 3.29.0.
2. Config: неверный URL/режим TLS/remote plaintext/host override, pool/timeout caps,
   redaction через fmt/JSON/slog. Ошибки не содержат тестовый пароль.
3. Lifecycle: schema/listen startup failures закрывают DB, готовность fail-closed,
   отмена закрывает listener/pool; forced shutdown отменяет in-flight probe.
   Отдельный regression test проверяет deadline снятия migration lock после
   upstream context.WithoutCancel: stalled unlock получает ограниченный context.
4. Реальная PostgreSQL 18.6: fresh/repeated/concurrent up migrations; duplicate
   normalized email race с одним winner; not-found; callback rollback и cancellation;
   persistence после close/open pool. Проверены неверные UUID/email/version,
   plaintext password, TTL/attempt/hash bounds, запрет session на pre-auth context.
5. Реальный app.Run дважды запущен/остановлен с БД: ready 200; future schema →503,
   live 200, auth route 404; listener освобождается. Изоляция каждой проверки —
   случайная schema в отдельной scratch БД; чужие БД/таблицы не удаляются.
6. Smoke actual binaries: migrate дважды, health/no auth routes; остановка
   PostgreSQL → ready 503/live 200, затем restart → ready 200. UUID пользователя
   сохранился после restart PostgreSQL и отдельного Auth процесса. Невалидный TLS
   останавливает binary startup без секрета в stdout/stderr.

PostgreSQL запущена временно на loopback без системной установки/Docker/deployment.
Официальный EDB artifact:
[postgresql-18.6-5-windows-x64-binaries.zip](https://get.enterprisedb.com/postgresql/postgresql-18.6-5-windows-x64-binaries.zip),
SHA-256 `e2246ba91d22345bc3d017586c09ede52d9df180b1eeb480f050445f1cad84e2`,
сверен с опубликованным рядом .sha256. Дистрибутив, scratch данные/пароль,
бинарники и логи находятся вне Git. Контейнерный image digest не заявляется.

## Ревью и ограничения

Выполнены два независимых read-only ревью: caveman-review и ponytail-review.
Оба нашли потенциальное зависание Goose SessionUnlock без deadline. Добавлены
bounded wrapper, общий timeout миграции и regression test; после этого root
повторил unit/integration/vet/build. На повторном проходе снимка `87f082f1…14f1496`
оба ревьювера не нашли новых доказанных дефектов; ponytail независимо повторил
unit tests и go vet. Их вердикты не являются человеческим принятием 3.2.
Pattern scan перед repository выполнен отдельным read-only агентом по skill
golang-database: подтвердил no existing service convention и проблемы референса.
Это не заменяет два финальных ревью.

Race detector не выполнен: команда `go test -race` остановилась с сообщением
«-race requires cgo»; C toolchain отсутствует. Обычные конкурентные integration
tests прошли, но это не race-check. TLS configuration проверена unit/negative
smoke; end-to-end trusted certificate provisioning не выполнялся. Тестовая
PostgreSQL работает с explicit local plaintext на loopback.

Нагрузочные замеры, PostgreSQL backup/deploy и least-privilege provisioning,
Auth mTLS/CSRF runtime, Resend и Core/API E2E не заявляются проверенными.
Переход к 3.3 — только после явного принятия 3.2 владельцем.
