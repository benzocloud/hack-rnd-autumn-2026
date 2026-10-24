# Go API: первый финансовый проход к сдаче

Дата: 24.10.2026. Дедлайн: **25.10, 12:00 Europe/Moscow**.
Статус: предложен разработчику; новые финансовые этапы не начаты.
Ревью: после каждого этапа остановка до явного апрува по ADR-020.
Публикация, merge в main и deployment — только по отдельной авторизации владельца.

## Актуальная база после синхронизации PR #30

Сверка 24.10.2026: ветка PR #30 включает main `6afa1dc` обычным merge.
Историческая таблица ниже описывает базу `31e7913`; она не является текущим
блокером контрактов. Теперь canonical `proto/core/v1` покрывает 54 REST operationId,
а `proto/ml/v1/forecast.proto` предоставляет один ForecastService/Predict по ADR-029.
Финансовый профиль: mean_90 / mean_90_components_v1 / calendar_replacement_v1;
baseline — mean_90_v1 / daily_snapshot_v1; автоматического fallback нет.
Готовые API bindings генерируются через `buf.api-core.gen.yaml`; маршруты и
presence/nulls/errors/job states/streaming описаны в `docs/core/api-adapter.md`,
полная финансовая ML-семантика — `docs/core/financial-ml.md`.
Core handlers/bootstrap/worker/persistence финансового результата ещё не готовы;
API адаптирует этот контракт, не создаёт второй proto и не заменяет Core Python CLI.
Auth business E2E также требует отдельной проверки настоящего Auth.
Разработчик API сохраняет владение своим планом F1–F3; их реализация/принятие
не объявляется завершённой этим merge.

## Проверенная база и сохранение работы

- `git status` → `git fetch origin` → именованный stash с untracked → обычный
  merge `origin/main` → восстановление stash. Main: `31e7913`, локальный merge:
  `b00a828`. Reset/force push не использовались; авторские ветки не менялись.
- Незакоммиченные десять auth endpoints, тесты/CI/документы сохранены.
  Конфликт `docs/api/README.md` разрешён сохранением обеих частей: ADR-028/P0
  из main и описания локального API. Stash оставлен; дополнительная резервная
  копия `/tmp/saldo-api-before-sync-20261024.tgz` и patch рядом.
- Прочитаны AGENTS, instructions, decisions, [общий план D1–D4](demo-integration.md),
  [integration-status](../../docs/integration-status.md), canonical Auth proto,
  Core snapshot, frontend dashboard calls и ветка ML.
- GBrain recall выполнен; источником текущих возможностей остаётся Git.
  GitHub discussions/CI новой поставки не объявляются проверенными.

## Что реально доступно

| Граница | Состояние |
| --- | --- |
| Browser → API → test Auth | Локально реализованы/проверены десять auth endpoints; это не настоящий Auth и не финансовая цепочка |
| Auth | `auth.v1.AuthService` описан в main; business RPC/runtime demo-profile пока отсутствуют |
| Core | Ledger/Payments и принятый FinancialSnapshot: `CaptureSnapshot`, `GetSnapshot`, PostgreSQL; это внутренние Go use cases, не RPC |
| API → Core | В main и проверенной Core-ветке нет `proto/core/v1` и работающего gRPC transport |
| Core → ML | В `origin/god4k1ta/feat/ml_grpc` на `994ed6e` есть `ml.v1.ForecastService/Predict`, Python server/inference; в main этого proto ещё нет |

ML-снимок ветки: [proto](https://github.com/benzocloud/hack-rnd-autumn-2026/blob/994ed6e/proto/ml/v1/forecast.proto),
[контракт](https://github.com/benzocloud/hack-rnd-autumn-2026/blob/994ed6e/services/ml/GRPC_CONTRACT.md).
Проверены исходники, новый ML runtime здесь не запускался; опубликованный код
не объявляется включённым в main, развёрнутым или подключённым к Core.

`Predict` сейчас: `mean_90/mean_90_v1/daily_snapshot_v1`, ровно 90 последовательных
подтверждённых дней, наблюдаемый EOD на cutoff, RUB/int64, горизонт 1..30.
Непустой `known_payments` → FAILED_PRECONDITION. Ответ history-only, включает
регулярные потоки из истории, не содержит категорий/выделенных регулярных событий
или калиброванной вероятности. Нельзя прибавить весь календарь поверх mean_90.

Core snapshot сейчас явно исключает исторический календарь
(`historical_calendar_unavailable`) и не доказывает point-in-time knowledge.
Для рабочего прогноза с известными будущими платежами нужен согласованный
операционный снимок календаря; нельзя объявлять исключённые платежи учтёнными
или нулевыми. Агрегирование 90 дневных точек/покрытие/EOD — ответственность Core,
не frontend/API и не заполнение неизвестных дней нулями.

## Потребности к контрактам: запрос ML/Core/Auth, не новая версия proto

Ниже точные необходимые операции. **Core-имена — рабочий запрос на согласование**,
а не утверждение о наличии RPC/утверждённых wire fields. Их protobuf под
`proto/core/v1` и расширения `proto/ml/v1` готовит ML-разработчик совместно с Core.
API не создаёт параллельный proto. Потребности подготовлены для владельцев;
публикация плана отдельно разрешена разработчиком. Согласование методов/семантики
внешними участниками пока не подтверждено; публикация не означает их принятия.

| Приоритет | Владелец | Нужные методы/возможности |
| --- | --- | --- |
| D1 | Auth | Реализовать существующие `GetPersonalDataConsent`, `GetSecurityContext`, `Register`, `VerifyEmail`, `Login`, `GetCurrentAccount`, `Logout`; `ListMemberships`, `AuthorizeCompany` |
| D1 | Auth + Core | Существующие Auth `BeginCompanyCreation`, `AuthorizeCompanyBootstrap`, `CompleteCompanyCreation`; необходимые Core `CreateCompanyBootstrap`, `GetCompanyCreationResult`, `ReadCompany` уже описаны как требования в Auth contract, но Core RPC нет |
| D1 | Core / ML proto-owner | Согласовать `GetAccount`, `CreateForecast`, `ListForecasts`, `GetForecast`; защищённые RPC, durable job/input до HTTP 202, сохранённый результат и повторное чтение |
| D1 | Core | ML client → существующий `ForecastService/Predict`, mapping внутреннего snapshot в 90 дневных точек, проверка ответа/пересчёт остатков/рисков, model/data versions и хранение |
| D2 | Core / ML proto-owner | `UploadImport` (или иной согласованный bounded file transport), `GetImport`, `ListImports`, `GetImportIssues`, `CommitImport`; preview без записи фактов, атомарное применение, dedup/version/source binding |
| D2 | Core / ML proto-owner | `ListTransactions`, `ListCategories`, `SetTransactionCategory`, `CreatePayment`, `ListPayments`, `GetPayment` |
| D2 | ML + Core | Результат категоризации/регулярности из реальных операций с происхождением и недопущением двойного учёта; нужный RPC либо расширение Predict определяет владелец proto |
| D2 | ML + Core | Согласовать текущий календарь и замену модельных компонентов при известных платежах; поддержка непустого known_payments или другого совместимого согласованного входа |
| D2/D3 | ML + Core | Обоснованная вероятность, её событие/калибровка и качество; rule confidence не заменяет вероятность, null оставляет пункт ТЗ открытым |

API передаёт исходные cookie/WriteContext, CSRF, Idempotency-Key, company scope
как параметр запроса. Auth/Core повторно проверяют полномочия; company/role из
браузера не являются доказательством доступа. JSON money/version strings ↔
точные protobuf values без float; IDs/даты, error mapping, pagination, deadlines
и mTLS peer identity согласуются в одном контракте. БД Auth/Core API не читает.

ML default peer `saldo-core` и Auth SPIFFE URI profile требуют согласованной PKI/
allowlist для межсервисного запуска; TLS нельзя отключать ради демонстрации.

## Минимальные HTTP-маршруты из существующего OpenAPI

Префикс `/api/v1`, `C = /companies/{company_id}`. Новые REST paths не вводятся.

| Этап | Маршруты |
| --- | --- |
| D1 workspace/access | `GET/POST /companies`, `GET C`, `GET C/account` плюс уже готовый auth subset входа/регистрации/подтверждения/выхода |
| D1 forecast | `GET/POST C/forecasts`, `GET C/forecasts/{forecast_id}` |
| D2 statement | `GET/POST C/imports`, `GET C/imports/{import_id}`, `GET C/imports/{import_id}/issues`, `POST C/imports/{import_id}/commit` |
| D2 categories | `GET C/transactions`, `GET C/categories`, `PATCH C/transactions/{transaction_id}/category` |
| D2 calendar | `GET/POST C/payments`, `GET C/payments/{payment_id}` |

`GET C/forecasts` нужен уже для первого dashboard: frontend ищет сохранённый
результат перед POST. Отдельный `/dashboard` не нужен: существующий frontend
собирает companies → account → list/create forecast → get/poll → daily_points.
Основной режим первой поставки — 30 дней; UI 60/90 не обеспечивается текущим
ML RPC и требует явного согласования, без скрытой подмены горизонта.

## Этапы API и критерии ревью

### F1 — контрактный стык и первый реальный forecast

Статус: Не начат. Цель общего D1 — 24.10, 18:00 МСК; зависит от upstream,
это контрольная точка, не обещание завершить отсутствующие сервисы за API-команду.
Пути: `services/api/internal/client/core`, `internal/gen/core`, company/account/
forecast handlers, API integration tests; опубликованные proto потребляются,
а не разрабатываются параллельно.

- [ ] Получить подтверждённый минимальный Core proto от ML-разработчика и RPC
  readiness Core/Auth; согласовать DTO, ошибки, snapshot/model context и PKI.
- [ ] Переиспользовать API startup/security; подключить Core client и D1 routes.
- [ ] С Auth подключить отдельный server-controlled demo-profile ADR-028:
  любой шестизначный код только при действующем challenge/expiry/user binding;
  пароль, серверная сессия, CSRF и компания остаются обязательными. Default strict.
- [ ] Включить frontend real API mode; показать маркировку demo email отдельно,
  не включать глобальный developerMode=mock и не использовать test Auth как
  замену runtime company authorization.
- [ ] Из браузера запустить POST forecast с Idempotency-Key/cutoff/horizon=30;
  Core сохраняет input/job, вызывает настоящий ML и сохраняет проверенный результат.
  GET polling не пересчитывает прогноз; повтор POST не дублирует job.

Проверки: реальные frontend HTTP → API → Core → Python ML; лог/артефакт request/
forecast/snapshot IDs без секретов; 30 точек на графике, повторное чтение после
перезагрузки; отказ ML не даёт успешный график; чужой tenant не читается.
До готовности календаря результат помечен history-only и не принимается за D2.
После результата — ревью и апрув владельца.

### F2 — выписка, категории/регулярность, будущий платёж

Статус: Не начат. Зависимости: принят F1, реальные import/category/payment RPC,
согласованное включение календаря без двойного учёта. Цель D2 — 24.10, 23:00 МСК.
Пути: API import/ledger/payment handlers и соответствующие upstream adapters/tests.

- [ ] Подключить реальный CSV upload/preview/commit по контракту без Python CLI
  вместо Core. Показать issues, версии, источник; повтор не удваивает операции.
- [ ] Отобразить настоящие категории и обнаруженную регулярность с provenance:
  использовать существующие `ForecastComponent.kind=recurring_expense`,
  `recurrence_group_id`, `category_id` там, где они подтверждены upstream.
  Текущий history-only Predict их не отдаёт; согласовать заполнение с ML/Core
  и отображение с frontend, без самостоятельного нового REST/RPC.
- [ ] Создать будущий платёж, запустить новый forecast и показать его влияние
  ровно один раз; показать сумму/дату разрыва, причины и warnings.

Проверки: настоящая выписка допустимого качества, повтор импорта, невалидный CSV,
конфликт версии, перезагрузка, недоступность зависимости, чужой scope; изменение
платежа меняет новый результат и не переписывает сохранённую базу.
После результата — ревью и апрув владельца.

### F3 — сквозная проверка и стабилизация

Статус: Не начат. Зависимость: принят F2. Общий D3 до 25.10, 09:00;
D4 09:00–12:00 — только блокирующие исправления и репетиция, без новых функций.
Пути: API E2E/smoke/README/verification, согласованная общая конфигурация.

- [ ] Зафиксировать воспроизводимые команды и артефакты реального прохода,
  metric/baseline/uncertainty ограничения ML и состояния loading/error/stale UI.
- [ ] Проверить demo vs strict code, запрет browser-controlled demo, CSRF,
  отзыв/чужую компанию, повтор команд, сетевой сбой, рестарт/повторное чтение.
- [ ] Повторить относящиеся к изменённым модулям checks, race и contract tests.

Критерий: основной путь воспроизводим до дедлайна; финансовых mock responses нет.
Если вероятность не обоснована, назвать незакрытым требованием, не выдумывать %.
Итоговое ревью; публикация/PR/push/deployment по отдельной авторизации.

## Следующее действие

Согласовать с владельцем этот P0-объём и передать таблицу потребностей владельцам
ML/Core/Auth. До получения canonical Core proto можно готовить только общие
адаптерные проверки/конфигурацию и документированную карту DTO; финансовый RPC
не имитировать. Прежний [Auth-план](go-api-implementation.md) сохраняет результаты,
но завершение recovery/invitations не является условием первого financial pass.
