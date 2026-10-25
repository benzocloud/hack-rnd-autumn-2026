# D2.2: durable upload и preview

25 октября 2026, Europe/Moscow. Ветка `globalarray/feat/financial_demo`.
D2.0 опубликован в draft PR #44, remote head `7d21ad39fb6c9492f530403c284d404e8b56ff13`.
Этот D2.2 diff пока локальный, без commit/push/merge/deploy. Onboarding — PR #41.
Посторонний `.agent/prompts/ml-model-training.md` не входит в эту работу.

## Реализовано

- HTTP upload/list/get/issues, Auth FINANCIAL_WRITE/CSRF перед spool и повторная
  авторизация в Core до приёма и перед durable acceptance. Commit пока 503;
  `can_commit=false`, UI честно показывает preview без применения.
- Private disk spool 0700/0600, два slots/200 MiB, bounded multipart 101 MiB,
  файл 100 MiB, hash/size и Core streaming frames до 1 MiB. Приватный volume/env
  и nginx limits описаны в checked-in шаблонах; live конфигурация не менялась.
- Core migration 00007: receiving/chunks/jobs/normalized preview rows/issues,
  reservations, cleanup; до 202 bytes/job сохранены. Короткие SQL Tx без сети
  и parsing, lease/fencing worker, восстановление после пересоздания service.
- dataset_v1 BOM/semicolon/exact kopecks/INN как текст, выбор external company,
  diagnostics по scoped IDs/EOD/source/reconciliation; issue paging, versions.
  Preview не пишет Ledger/EOD/source bindings и не повышает account revision.
- Import worker работает без ML. Stream mTLS allowlist, deadline/cancellation,
  frame limit с protobuf overhead; закрытие worker перед app-owned pool.
- ListImports ограничивает protobuf page до 1.5 MiB, уменьшая limit с исходным
  cursor; записи не пропускаются. Единичный слишком большой ресурс отклоняется.
- Expiry seven days, receiving lease five minutes, raw cleanup через 24 часа
  после terminal; удаление bytes и освобождение reservation атомарны.

## Проверено

Go 1.27.3, golangci-lint 2.15.0, Buf 1.75.0, Ruff 0.17.1.

- Core/API полный `go test -race ./...`, vet и полный module lint: PASS/0 issues.
  Дополнительно Core integration-tag vet/lint и fmt: PASS.
- Marked PostgreSQL 18.6, loopback port 63961, `saldo_core_test`, marker
  `saldo-core-integration`; migration 00007 применена только к этой test DB.
  Core app/repository integration/race suites: PASS. Новые тесты проверяют
  receiving/hash/short stream, orphan cleanup/quota, concurrent key/replay,
  tenant/cursor, worker recovery, отсутствие финансовых записей.
- Actual API binary → mTLS actual Core → PostgreSQL: PASS с race. Upload202,
  queued GET, новый worker/process service, preview, полный issues paging,
  replay/changed-bytes conflict, spool cleanup, expiry→failed/null preview.
  **Только Auth здесь test double**; это не проверка настоящего login/onboarding.
- Отдельный HTTPS API → mTLS Core transport fixture, strict multipart,
  >100 MiB file rejection, write authorization before spool: PASS.
- Реальные mTLS peer/frame/deadline/cancellation tests: PASS; 1 MiB chunk
  проходит с protobuf overhead, чужие service identities отклоняются.
- Handler regressions: repeated metadata и отзыв membership перед EOF не дают
  acceptance; write context/scope проверены. Race и targeted lint PASS.
- Финальная целевая регрессия ListImports: 100 крупных ресурсов прочитаны без
  пропусков/повторов, размер страницы ограничен, максимум семь запросов при
  уменьшении limit. Полные suites после этой локальной правки не повторялись
  по новой частоте проверок владельца; применён один относящийся к ней test.
- Parser: exact 300000/300001 row boundary, bounded 250-record batches,
  int64 money/UTF-8/BOM/multiline/invalid CSV: PASS с race.
- 150 duplicate diagnostics проходят paging, каждый с одним external ID;
  expiry/raw cleanup идемпотентны, original receipt сохраняется: real PG PASS.
- Frontend: 24 tests, typecheck, lint, production build PASS.
- 11 schema tests, Buf lint, Compose spool/ingress assertions, deployment
  regression test, Ruff/shell syntax и diff-check PASS.
- Независимые caveman-review/ponytail-review: исправлены несовместимые issue
  codes, failed preview, manual namespace, unbounded nested evidence и ошибочный
  `can_commit`. Новая проекция проверена реальным HTTP тестом.

## Ограничения и продолжение

Не подтверждены настоящий browser/login/Auth→API→Core полный сценарий и restart
всего Compose. PostgreSQL test проверяет durable storage после пересоздания
service, не crash хоста. Не запускалась максимальная 100 MiB загрузка под
production cgroup/WAL/disk нагрузкой; parser limit и API oversize проверены
отдельно. Live provisioning/deployment не выполнялись.

Parser ограничивает logical CSV record 64 KiB; preview более 1000 distinct EOD
дат показывает blocker и ограниченный список balances. Это защитные пределы
текущего кандидата, их нужно явно рассмотреть перед широкой поддержкой истории.
Synthetic fixture coverage создаётся только будущим D2.3 commit; upload/preview
не удостоверяют полноту. D2.3 атомарный commit, ledger reads/categories ещё не начаты.

Следующие действия:

1. Рассмотреть D2.2 diff/ограничения и выполнить real browser/Auth rehearsal
   вместе с готовым onboarding; не считать fixture Auth продуктовой приёмкой.
2. Перед отдельным разрешённым push fetch/merge свежего main и согласовать
   пересечения app wiring с onboarding. Следующие commit/push ещё не разрешены.
3. После принятия этапа реализовать D2.3: Tx commit/CAS/ack/source binding,
   дедупликация, coverage по ADR-030 и чтение ledger/category.
