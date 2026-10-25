# D2.3a — ограниченный CSV-профиль, локальный кандидат

25.10.2026, Europe/Moscow. Владелец согласовал ADR-031 и поручил «приступай».
В async-ответе отдельно принял первый профиль: охват 90 дней, cutoff EOD,
неизвестные дни, ограниченная оценка по доступным операциям и блокировка
обнаруженных расхождений остатков. Перед независимым review нужно остановиться
для переключения модели владельцем. Commit/push/merge/deploy не поручены.

## Сделано

- Core выбирает отдельный `mean_90_csv_v1` /
  `csv_observed_operations_v1` при отсутствии подходящего coverage у импортированных
  фактов. Обычный CSV не превращается в certified/fixture proof.
- `csv_history_quality_v1` проверяет span, single imported source, <=20000
  операций окна, EOD evidence и exact reconciliation; неизвестные даты сохраняются
  без сумм/остатков. Нет zero-filled historical facts или ручного SQL proof.
- Оценка с EOD provenance сохраняется в job input schema 2. Она проверяется
  повторно по frozen snapshot/evidence при обработке и чтении. Новое создание
  прогноза переоценивает старый импорт без повторного commit/factual revision.
- ML считает ограниченные средние доступных потоков на календарный день,
  выдаёт 30 точек, не утверждает полноту, не оценивает probability/bands. Новый
  профиль проходит один Predict; старые v1 profiles отклоняют CSV context.
- Core/API additive contracts: ForecastInput.history_assessment=17,
  ML PredictRequest.csv_history=7 / response field 26. Core error reasons 30–33
  разделяют короткую историю, reconciliation, source и operation limit.
- API валидирует отдельный профиль, assessment/revision, readiness limited,
  limitations и отсутствие coverage. UI показывает русские пояснения; форма
  ForecastDetail предлагает только поддерживаемый горизонт 30 дней.
- Старый schema 1 decoder и покрытый v1 путь сохранены; новых DB migrations нет.

## Проверки

- Core: targeted unit suite — 106 tests / 5 packages; PG/real Python mTLS
  focused suite — 174 / 2 packages. После HTTP расширения полный suite
  `go test -p 1 -tags=integration,mlintegration -race -count=1 ./...` —
  655 passed / 38 packages, на marked disposable PostgreSQL schema 12.
- Первый параллельный full PG прогон: 654 passed, один старый import guard test
  получил stale preview. Пакеты используют общую DB/queue, включая runtime
  background worker. Последовательный полный прогон прошёл. Не выдавать
  параллельную стабильность за доказанную; guards не ослаблялись.
- HTTP integration использует настоящий API binary, mTLS Core и PG: после
  удаления proof только в fixture новый forecast переоценивает историю;
  history_assessment сохраняется, coverage null, facts/revision не меняются,
  результат перечитывается. Auth и predictor в этом HTTP тесте — test doubles.
- Отдельный Go client → настоящий Python ML mTLS test проверил новый профиль,
  30 точек и арифметику. Python RPC/security suite — 44 tests, без skips.
- API full suite — 248 tests / 12 packages. Frontend: typecheck/lint + 30 tests.
- Core/API full module golangci-lint 2.15.0 — 0 issues; vet прошёл.
  Попутно исправлены два старых API lint замечания (type assertion и POST
  literal), формат import block ledger_fixture_test. Это не новые функции.
- Buf lint/build/breaking, contract coverage (58 operations), временная
  generation/compilation Core/API/Auth consumers прошли. API-local generation
  требует pinned protoc-gen-go 1.36.12; стандартный /Users/globalarray/go/bin
  содержит 1.36.11, поэтому использовать temporary pinned tools (см. ниже).
- OpenAPI schema tests — 9; Ruff check/format прошли для затронутого Python.

## Качество и ограничения

`services/ml/evaluate_csv_profile.py` задаёт описательный протокол без fitting:
150 компаний, номинальные cutoffs 31.07/31.08.2026, только реальные будущие EOD.
Оценены 248 окон / 128 компаний / 4514 точек. MAE модели 123347040.06 коп.,
constant-EOD 113610070.75 коп.: CSV профиль хуже на ~8.6%. Это не promotion pass.
24 окна отклонены по reconciliation, 12 — короткая история, 2 — нет прошлого,
14 — нет будущего EOD для оценки. Подробности и честные пределы:
`docs/core/csv-forecast-quality.md`. `not_calibrated` не закрывает ТЗ по вероятности.

Настоящий browser/Auth сценарий ещё не выполнен; новый кандидат не выложен.
Нельзя заявлять исправленный публичный сайт. Перед rollout нужны ML+API/UI,
затем остановка старого Core и запуск нового: старый worker/reader не понимает
job schema 2. Rollback должен сохранять новый decoder. См. отчёт качества.

## Окружение и чужие изменения

База работы HEAD `3b3cebc`, ветка `globalarray/feat/import_source_preview`.
Не трогать `.agent/prompts/ml-model-training.md` и `frontend/AGENTS.md`.
Во время работы появились чужие deploy/stack/README.md, smoke.py и test_smoke.py;
не включать их в scope этого блока. Auth runtime не менялся; случайная генерация
Auth core consumer была удалена, Auth auth bindings восстановлены к HEAD.

Временные инструменты: `/tmp/saldo-tools/buf` 1.75.0,
`/tmp/saldo-csv-venv/bin/python` с pinned RPC/API schema dependencies,
`/tmp/saldo-go-cache`, `/tmp/saldo-csv-api`. Для контрактных CLI generators
использовать `PATH=/tmp/saldo-tools:$PATH` с go plugin 1.36.12/grpc 1.6.2.
Docker integration project `saldo-core-integration`; перед завершением удалить
только его через `docker compose -f services/core/compose.integration.yml down`.

Pinned Go generators установлены в `/tmp/saldo-tools`; API-local generation
и `generate.py --check` прошли. Из API bindings меняются только errors.pb.go и
forecasting.pb.go; шум неизменённых generated headers удалён восстановлением
побайтно эквивалентных файлов. Auth bindings не меняются.

Итоговые Buf format/diff-check прошли. Одноразовые PostgreSQL container/network
удалены. Локальный блок остановлен перед независимым review по просьбе владельца;
можно переключать модель. D2.3a не объявлен полностью принятым: остаются
настоящая browser/Auth приёмка и решение по качеству профиля. Не выдавать
работающий технический путь за доказанное преимущество модели или исправленный
публичный сайт. Полный suite не повторять ради документации/комментариев.

## Дополнение: ручные операции после импорта

Владелец поручил «давай разрешим и исправим». ADR-032 заменяет запрет manual
после imported: внешний namespace сохраняется, каждая ручная запись имеет
manual provenance. Точный импортированный эквивалент отклоняется с
TRANSACTION_DUPLICATE_SUSPECTED. Следующий CSV предупреждает о совпадении с
manual и требует ack: подтверждение сохраняет обе записи, без скрытого merge.
Ручные факты/остатки не получают fixture proof; совпадающий manual EOD сохраняет
источник, конфликт суммы блокируется. Mixed history проходит CSV assessment/ML
с повторной reconciliation и новой revision; старые forecast results неизменны.

Подключён отсутствовавший HTTP POST transactions → существующий Core
CreateManualTransaction: strict body, decimal money/version, CSRF/Origin,
scope и idempotency. UI разрешает ручное создание на imported account.

Проверки блока: API 251 tests / 12 packages и build; frontend typecheck/lint
и 30 tests; ML RPC/security 45 tests без skips; API schemas 9; Core/API full
module lint — 0 issues. Core lint с integration/mlintegration tags — 0 issues.
Buf lint/build/breaking/format и consumer contracts прошли. Core full serial
race suite дал 657 passed, 1 failed, 1 skipped (opt-in large import): новый
тест повторного другого CSV использовал старый upload key. В helper добавлена
возможность отдельного ключа; исправленный mixed import regression прошёл.
Старые тесты запрета manual обновлены; обязательная Note тестового manual EOD
заполнена. Последующий app rerun не прошёл из-за отсутствующего PostgreSQL
container (postgres ping failed), а не утверждённого product regression.
Финальный прогон выполняется в отдельном project saldo-csv-manual-checkpoint;
результат фиксируется ниже после завершения. Предыдущие failed runs не объявлять
полностью зелёными.

Независимое review ещё не выполнено: владелец просил остановку для переключения
модели. Последующее поручение разрешило commit/push; публикация кандидата не
равна acceptance/merge/deploy. Для параллельной работы подготовлены три
самодостаточных поручения: ../prompts/checkpoint-team-index.md. Онбординг и
обучение модели не дублируются; PR создают получатели этих поручений.

### Checkpoint публикации 25.10.2026

Поручения и индекс распределения закоммичены в `581ca13` и успешно pushed в
`origin/globalarray/feat/import_source_preview`. Runtime-кандидат ещё не
закоммичен/не запушен, PR для него ещё не создан. На момент чтения GitHub
открыты отдельные draft #41 (onboarding), #42 (ML rehearsal), #45 (training).
Нашу работу не выдавать за их содержимое и не менять эти PR.

Временные инструменты `/tmp/saldo-tools`, `/tmp/saldo-csv-venv` и бинарник
`/tmp/saldo-csv-api` исчезли. Новый полный прогон на свежей DB запущен до
восстановления инструментов и применения миграций: 556 passed, 89 failed,
1 skipped; причины — отсутствующая схема и Python. Этот запуск не подтверждает
готовность и не заменяет прежние результаты. Нужно восстановить окружение,
применить штатные миграции и выполнить финальный serial suite.

Создан только наш disposable Docker project `saldo-csv-manual-checkpoint`:
PostgreSQL healthy, `127.0.0.1:54065`, marker `saldo-core-integration`, стандартные
тестовые user/database из compose.integration.yml. Финальный teardown пока
не выполнен. Прежний project уже отсутствовал; его не пересоздавать.

Восстановление прервано automatic approval review: refresh token revoked,
нужно выйти из Codex и войти повторно. Проверка сообщила, что действие не
исполнено и это ошибка review, не оценка небезопасности. Не обходить sandbox/
approval другим каналом. Параллельно запрашивались migrations, поиск Python,
API build; завершение этих действий не подтверждено.

После восстановления авторизации:

1. Проверить status и текущие processes/тестовую DB; сохранить чужие изменения.
2. Применить goose 3.29.0 из `/Users/globalarray/go/bin/goose` к нашей DB:
   `GOOSE_DRIVER=postgres GOOSE_DBSTRING=postgres://core_test:core_test_only@127.0.0.1:54065/saldo_core_test?sslmode=disable goose -dir services/core/migrations up`.
   Это только локальные одноразовые credentials из compose, не production.
3. Найти установленный Python 3.12+ (`uv python list --only-installed`), создать
   отдельное временное venv, установить pinned services/ml/requirements-rpc-dev.txt
   и необходимые API schema test deps. Пересобрать API binary из services/api.
4. Запустить Core `go test -p 1 -tags=integration,mlintegration -race -count=1 ./...`
   с CORE_POSTGRES_* для port 54065, CORE_API_TEST_BINARY и проверенными
   CORE_ML_TEST_PYTHON/CORE_ML_TEST_SOURCE. Не ставить несуществующие temp paths.
5. Обновить этот handoff фактическим результатом; только затем commit/push
   кандидата и создать один draft PR. Независимое review — после переключения
   модели владельцем. Не merge/deploy.
6. Удалить только наш project:
   `docker compose -p saldo-csv-manual-checkpoint -f services/core/compose.integration.yml down`.

Список отобранных файлов перед commit сохранялся в /tmp/saldo-csv-publish-paths.txt,
но temp-файлы ненадёжны: перепроверить по status. Исключить чужие deploy/stack
README.md/smoke.py/test_smoke.py, `.agent/prompts/ml-model-training.md` и
`frontend/AGENTS.md`. Не git add -A всего дерева.

### Финальная проверка после восстановления доступа

Авторизация восстановилась. В нашем отдельном Docker project применены
штатные migrations до schema 12. Восстановлен Python 3.12.13 environment
`/tmp/saldo-checkpoint-venv` с pinned RPC dependencies, API binary пересобран
в `/tmp/saldo-checkpoint-api` из текущего кода.

Полный Core serial suite с PostgreSQL, API binary и настоящим Python ML:
`go test -p 1 -tags=integration,mlintegration -race -count=1 ./...` —
**658 passed / 38 packages**. Opt-in large-import measurement не включался.
API suite повторно — **251 passed / 12 packages**; ML RPC/security повторно —
**45 tests, OK**; Core full lint с integration/mlintegration tags — **0 issues**.
Diff-check прошёл. Этот прогон заменяет неуспешные recovery-прогоны выше.

Работа публикуется локальным кандидатом по прямому поручению commit/push.
Независимого review и browser/Auth приёмки ещё не было; stage acceptance,
merge/deploy и доказанное улучшение точности по-прежнему не заявляются.

### Публикация подтверждена

Кандидат закоммичен в `330555d3cc8891dc989237b16b5cb272cc41be4b` и pushed
в `origin/globalarray/feat/import_source_preview`; remote SHA проверен.
Создан **draft PR #47**:
https://github.com/benzocloud/hack-rnd-autumn-2026/pull/47.
Поручения агентам находятся в той же ветке (коммит `581ca13`), каждый получатель
создаёт собственный implementation draft PR. Независимое review нашего #47
ожидает переключения модели владельцем. CI после push ещё не проверен.

Наш Docker project `saldo-csv-manual-checkpoint` после успешного прогона удалён:
container и network teardown подтверждены. В checkout остались только чужие
deploy/stack README.md/smoke.py/test_smoke.py, ML training prompt и frontend/AGENTS.md;
они не включены в наш commit/PR. Следующий шаг — review #47, затем browser/Auth
приёмка согласованного кандидата. Merge/deploy не выполнялись.
