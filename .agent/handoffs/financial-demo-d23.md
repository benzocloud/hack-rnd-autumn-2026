# D2.3 — локальное продолжение, этап не завершён

25.10.2026. D2.2 принят; владелец разрешил продолжать D2.3. Commit/push/merge/
deploy D2.3 не поручены. Текущая ветка `globalarray/feat/financial_demo`; PR #44
по-прежнему содержит D2.0. Сохранить все локальные и untracked изменения.
Посторонний `.agent/prompts/ml-model-training.md` не трогать.

## Выполнено и проверено

- Atomic Core import Commit: command receipt → account lock → import CAS,
  warning acknowledgement, повторная проверка source/conflicts, scoped dedupe,
  факты/EOD, одна revision и applied state в одной app-owned Tx. Service владеет
  receipt; repository не координирует use case. Handler повторно проверяет Auth
  write scope. Idempotency fingerprint не зависит от серверной даты.
- Regression на marked PostgreSQL: replay применяет один раз при параллельных
  вызовах; чужой company scope не меняет данные; equivalent source rows между
  файлами дедупятся, изменённое содержимое под тем же ID блокируется; stale
  preview/manual source conflict отклоняются; SQL failure при EOD полностью
  откатывает факты, source binding, coverage, receipt, revision и import state.
- Ledger: Core ListTransactions/ListCategories/UpdateTransactionCategory,
  filters/paging, category CAS/no-op/clear и canonical category seed реализованы.
  PostgreSQL тест подтверждает перенос proof через category/payment metadata и
  отсутствие переноса через новую фактическую запись.
- Manifest: parser требует явного demo enablement, строгий bounded JSON и точные
  scope/digest/company/window/cutoff evidence. Commit проверяет manifest proof
  по импортированным строкам и EOD evidence. Проверен путь manifest → preview →
  commit → Core ledger HTTP reads/category edit → forecast, включая kind,
  limitations, hash reference и data version в persisted/model/API input.
- App constructor теперь eagerly валидирует преобразованный manifest; ошибочные
  IDs/ranges/digests и дубли отклоняются при wiring, а не только на Commit.
  Точечные Go tests Core imports/app прошли после изменения.
- HTTP import integration проходит через API, mTLS Core, PostgreSQL и forecast
  test doubles. Category assign/no-op/clear сохраняют корректную revision и
  coverage. HTTP Auth — test double; реальный login/browser E2E не проверен.
- Full Core integration suite с `-tags=integration -race -count=1 ./...` прошёл
  25.10.2026 на одноразовой marked PostgreSQL БД; migrations 00001–00008,
  00010–00011 применились, текущая schema version 11. API suite (241 tests),
  frontend tests (25), frontend typecheck/lint были пройдены ранее и после тех
  запусков backend contract изменений не менялись. Эти прогоны старше текущего
  diff и не заменяют итоговый gate.
- Потолок импорта измерен 25.10.2026 на одноразовой marked PostgreSQL 18.6
  (tmpfs, `compose.integration.yml`, schema 11). Хост: go1.27.3, darwin/arm64,
  14 CPU. Query timeout 5s, commit budget 90s не повышались. Fixture:
  300000 строк, 27600119 байт, уникальные purpose, одна дата, один EOD.
  Upload 392ms, processing 1m19.4s, commit 10.3s, warnings 0, facts 300000.
  Обычный suite этот тест не запускает: `SALDO_LARGE_IMPORT=1 go test
  -tags=integration -count=1 -timeout=20m -run TestImportScaleCeiling ./internal/app`.
- Persisted import receipt зафиксирован golden JSON
  `services/core/internal/service/imports/testdata/commit_receipt_v1.json`.
  NULL result не применяется. Лишнее поле в старом receipt не ломает replay и
  не пишет факты. Отдельный receipt-тип не добавлялся: текущий codec — JSON
  `CommitResult`, replay проверяет id/scope/status/version.
- Старые строки читаются. Транзакция с `created_at` и `import_id` NULL
  возвращается из ListTransactions с пустыми provenance. Категория до 00011
  (NULL direction/catalogue_code) читается с пустым direction и semantic
  `other`. Coverage до 00008 с NULL kind не становится proof и не даёт ошибку
  чтения: `ReadCoverage` возвращает not found.

## Осталось до завершения D2.3

1. Реальный browser путь не выполнен. Публичный `https://saldo.brokebyte.me/login`
   открывается и показывает настоящую форму входа, но README стека прямо говорит,
   что imports на опубликованном сервере нет, а D2.2/D2.3 не выкатаны. Локальный
   `deploy/stack` не содержит env/PKI, Compose требует готовые образы
   `AUTH_IMAGE`/`CORE_IMAGE`/`API_IMAGE`/`ML_IMAGE`. Commit/push/deploy не
   разрешены, поэтому этот checkout нельзя выложить для браузерной приёмки.
   Учётных данных для входа в репозитории нет; server-only smoke account не
   использовался. Auth test double по-прежнему не доказательство.
2. Core gate 25.10.2026: `task check` прошёл build, fmt, `go test ./...`,
   `go test -race ./...` и vet. Lint сначала не стартовал: в PATH был
   golangci-lint 2.13.2, нужен 2.15.0. После установки 2.15.0 полный
   `golangci-lint run ./...` нашёл 5 старых замечаний (errcheck close,
   nestif, QF1003, лишняя пустая строка). Они исправлены без смены
   поведения; повторный lint: 0 issues. Race по затронутым пакетам:
   92 теста, 6 пакетов. Одноразовый PostgreSQL контейнер удалён.
   API/frontend suite заново не гонялись: этот проход их не менял.
   GBrain MCP в этой сессии нет, recall не выполнен.
3. Локальное ревью diff после подготовки нашло шесть замечаний, и они
   исправлены в этом checkout. Неизвестный coverage kind больше не кодируется
   как certified. Preview не предлагает commit, если конец периода позже
   сегодняшнего дня в Europe/Moscow. Cleanup удаляет `core_import_rows` в той
   же транзакции, где удаляет raw chunks и ставит `raw_deleted`. Успешный
   прогноз без certified или dataset-fixture coverage API больше не отдаёт.
   `ObservedDays` считается по разным датам фактов внутри proof и 90-дневного
   окна. Слот import spool освобождается, даже если unlink файла не удался.
   Проверено `go test -count=1` для core handler imports, forecasting и
   repository imports (9 тестов) и `services/api/internal/handler/http`
   (147 тестов). После commit `1b4c702` полный Core `task check`
   (build, fmt, `go test`, `go test -race`, vet, golangci-lint 2.15.0)
   завершился с кодом 0; lint сообщил `0 issues`. Предупреждения кеша
   golangci-lint не меняют результат. Браузерный путь по-прежнему
   заблокирован: публичный README говорит, что imports на сервере нет,
   локальный стек без env/PKI и образов, deploy не разрешён.
   Владелец принял D2.3 25.10.2026 словом «апрув». Браузерный предел
   остаётся незакрытым и не считается пройденным. Merge/deploy не делались.
   Следующий этап по плану — D2.4. Он ещё не начат.

Миграция 00009 намеренно не создана: category schema получила номер 00011, без
вставки миграции задним числом. Gaps не закрывать снятием проверок или ручным
SQL proof. Продолжение D2.3 разрешено владельцем; новый апрув на работу не нужен.
