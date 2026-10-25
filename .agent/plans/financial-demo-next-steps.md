# Saldo: реализация D2–D4 во всех компонентах

Дата: 24 октября 2026, Europe/Moscow. Статус: **исполнение начато по поручению владельца**.
Первоначальный запрос разрешал подготовку плана. Последующим прямым поручением
«приступай к его исполнению» владелец разрешил реализацию нашей части.
D2.0 принят и опубликован в draft PR #44 по прямому поручению владельца;
D2.1 передан удалённому разработчику (draft PR #41). D2.2 начат по поручению
«коммит пуш в PR и следующий этап». Остальные этапы не начаты. Merge/deploy
не разрешены; публикация D2.2 потребует отдельного решения после ревью результата.
Режим реализации по умолчанию: после каждого этапа остановка на ревью и явный
апрув разработчика по ADR-020. Мультиагентность не отменяет эти точки ревью.

### Частота проверок и ревью

Уточнение владельца от 25.10.2026: тесты и независимые ревью запускаются после
содержательного завершённого блока, а не после каждой мелкой правки.

- Блок — проверяемый результат: например, durable Core upload/preview,
  готовый HTTP adapter или связанный end-to-end сценарий. Сначала закончить
  реализацию и собрать связанные исправления, затем выполнить подходящие gates.
- Изменения текста, форматирования, имён и другие локальные низкорисковые
  корректировки накапливать до конца блока. Не запускать ради них полный test/
  race/lint/build, повторную генерацию или новые агентские ревью.
- В ходе реализации допустима узкая проверка, если она нужна для диагностики
  конкретной ошибки или подтверждения существенного риска: авторизация,
  конкурентность, финансовая арифметика, SQL atomicity, transport/ресурсные limits.
  Выполнять минимальную подходящую проверку и сохранять результат.
- Один согласованный набор тестов/сборки/lint и независимых ревью — на готовый
  блок. Замечания ревью исправлять пакетом; повторять только проверки затронутых
  invariants. Полный набор повторяется, только если изменились общие контракты,
  зависимости, wiring либо осталась обоснованная неопределённость.
- Координатор ведёт общий список выполненных gates. Не дублировать тот же suite
  у нескольких агентов и не перезапускать успешную проверку без новых относящихся
  к ней изменений. Прогресс и handoff обновлять без дополнительного runtime gate.
- Перед передачей этапа на ревью владельцу обязательные gates по его итоговому
  коду должны быть закрыты либо явно указаны как невыполненные. Эта частота
  не отменяет приёмку этапов и отдельное разрешение публикации/merge/deploy.

Это уточнение ближайших работ из [D1–D4](demo-integration.md), а не сброс истории
принятых этапов Core/Auth. Дедлайн владельца — **25 октября, 12:00 МСК**.
При подготовке плана уже около 23:00 МСК 24 октября: старые контрольные точки
D1/D2 нельзя выдавать за новый реалистичный срок. Вероятность и расширенный
onboarding требуют отдельной оценки после согласования решений D2.0.

## 1. Цель, основания и текущая база

Получить воспроизводимый пользовательский путь:

**вход → доступ к компании/счёту → CSV → preview → commit → операции и категории
→ объяснимая регулярность → будущий платёж → новый 30-дневный прогноз
→ график, дата/сумма риска → сохранение и повторное чтение**.

Требование вероятности риска остаётся обязательным требованием продукта.
Работающий финансовый путь и доказанная вероятность принимаются отдельно;
`not_calibrated` — допустимое честное состояние UI, но не закрытие этого требования.

Проверены локальные исходники `86c13748bba417bcb51fb3ee18dfbeb758eed66c`.
Во время подготовки выполнен fetch: свежий `origin/main` —
`5c5797ea92636a75a59cb2a87c6462bc4a6d0f68`. Разница — deployment PR #37
и безопасные REST-сообщения PR #38; их изменения просмотрены отдельно.
Checkout не переключался и чужие изменения не переносились.
Перед реализацией нужна тематическая ветка от свежего main и повторная проверка
пересекающейся работы, а не реализация поверх старого локального main.

Issue этому поручению не назначен; scope задан прямым запросом владельца,
существующим планом D1–D4 и продуктовым описанием. В GitHub прочитаны описания,
все доступные conversation/review/inline комментарии PR #34–38; комментариев
в них нет. На момент проверки открытых PR не было. GBrain доступен, но найденные
заметки отстают от Git; узкий keyword-only recall не дал дополнительных фактов.

| Компонент | Подтверждённая база | Оставшаяся работа |
| --- | --- | --- |
| Frontend | HTTP adapters, экраны Imports/Transactions/Calendar, forecast polling/chart/stale | Реальные backend endpoints; Workspace без зависимости от приглашений; причины/регулярность; согласованный горизонт |
| API | Auth, company list/read, account read, forecast create/list/get | Upload/preview/commit, ledger reads/category update, payments и company creation |
| Core | Ledger/Payments use cases, company bootstrap/receipt, snapshots/jobs/forecast persistence | Import runtime, чтения/категории, Payments RPC, календарь в snapshot/ML и публичные components |
| Auth | Реальные пароли/сессии/CSRF/membership, server-only demo email | Durable пользовательское создание компании; финансовые permissions переиспользуются |
| ML | Один Predict, финансовый профиль, replacement/additional payment arithmetic | Runtime категоризация/регулярность, согласованная версия анализа, доказанная вероятность |
| PostgreSQL | Раздельные Auth/Core БД, sqlx/app-owned pool/Tx | Feature migrations для imports/analysis/coverage/onboarding, совместимые со старой версией приложения |
| CI/deploy | PR #37 добавил действующий стек и main-triggered deployment | Проверки новых сценариев, миграций и upload; повторное подтверждение на новом SHA |

Первая сквозная проверка была на операторской синтетической компании с заранее
подготовленной историей и membership. Она **не доказывает импорт, самостоятельное
создание компании или качество вероятности**. Публичное начальное развёртывание
и restart/persistence записаны в PR #37; заново оно в D2 не проектируется.
GitHub run [40126782155](https://github.com/benzocloud/hack-rnd-autumn-2026/actions/runs/40126782155)
успешен на `53ac14efb57543480423e18665dfb6badb936a9b`.
Run [40127358803](https://github.com/benzocloud/hack-rnd-autumn-2026/actions/runs/40127358803)
на `5c5797e` при проверке ещё выполнялся; его успех здесь не заявляется.
Новые runtime-тесты при подготовке плана не запускались.

### Исходники, определяющие границы

Ссылки фиксированы на проверенный commit; пути новых файлов ниже — предложения.

- [Текущие API clients](https://github.com/benzocloud/hack-rnd-autumn-2026/blob/86c13748bba417bcb51fb3ee18dfbeb758eed66c/services/api/internal/handler/http/router.go#L46-L58)
  и [регистрация Core handlers](https://github.com/benzocloud/hack-rnd-autumn-2026/blob/86c13748bba417bcb51fb3ee18dfbeb758eed66c/services/core/internal/app/server/server.go#L77-L85): imports/payments runtime ещё не подключён.
- [Контракт dataset_v1](https://github.com/benzocloud/hack-rnd-autumn-2026/blob/86c13748bba417bcb51fb3ee18dfbeb758eed66c/docs/api/openapi.yaml#L1038-L1043)
  и [upload RPC](https://github.com/benzocloud/hack-rnd-autumn-2026/blob/86c13748bba417bcb51fb3ee18dfbeb758eed66c/proto/core/v1/imports.proto#L110-L142): durable bytes/job, hash, preview, атомарный commit.
- [Workspace](https://github.com/benzocloud/hack-rnd-autumn-2026/blob/86c13748bba417bcb51fb3ee18dfbeb758eed66c/frontend/src/features/Workspace/Workspace.tsx#L23-L29): загрузка companies зависит от отсутствующего invitations endpoint.
- [Подготовка ML input](https://github.com/benzocloud/hack-rnd-autumn-2026/blob/86c13748bba417bcb51fb3ee18dfbeb758eed66c/services/core/internal/service/forecasting/job_input.go#L53-L104)
  и [coverage query](https://github.com/benzocloud/hack-rnd-autumn-2026/blob/86c13748bba417bcb51fb3ee18dfbeb758eed66c/services/core/internal/repository/postgres/forecastjobs/sql/read_coverage.sql#L1-L3): 90 дней, proof связан с общей data_version.
- [Go ML request/response](https://github.com/benzocloud/hack-rnd-autumn-2026/blob/86c13748bba417bcb51fb3ee18dfbeb758eed66c/services/core/internal/client/ml/financial.go#L43-L140)
  и [evidence validation](https://github.com/benzocloud/hack-rnd-autumn-2026/blob/86c13748bba417bcb51fb3ee18dfbeb758eed66c/services/core/internal/client/ml/financial_evidence.go#L15-L75): календарь только UNAVAILABLE, known components и новые категории пока не принимаются.
- [ML replacement и pass-through labels](https://github.com/benzocloud/hack-rnd-autumn-2026/blob/86c13748bba417bcb51fb3ee18dfbeb758eed66c/services/ml/cashflow_rpc/financial.py#L170-L250),
  [uncalibrated probability](https://github.com/benzocloud/hack-rnd-autumn-2026/blob/86c13748bba417bcb51fb3ee18dfbeb758eed66c/services/ml/cashflow_rpc/financial.py#L310-L335).
- [Ограничения датасета](https://github.com/benzocloud/hack-rnd-autumn-2026/blob/86c13748bba417bcb51fb3ee18dfbeb758eed66c/.agent/references/dataset-profile.md)
  и [нормализованный формат](https://github.com/benzocloud/hack-rnd-autumn-2026/blob/86c13748bba417bcb51fb3ee18dfbeb758eed66c/docs/data-contracts/README.md).
- [Deployment baseline](https://github.com/benzocloud/hack-rnd-autumn-2026/blob/5c5797ea92636a75a59cb2a87c6462bc4a6d0f68/deploy/stack/README.md)
  и [безопасные сообщения API](https://github.com/benzocloud/hack-rnd-autumn-2026/blob/5c5797ea92636a75a59cb2a87c6462bc4a6d0f68/services/api/internal/handler/http/error_messages.go).

## 2. Принятые решения и границы

Сохраняются ADR-002/003/020/022/024–029: browser → HTTP API → gRPC Core,
Auth отдельно, Core → один ML Predict; PostgreSQL, sqlx, ручная сборка,
UUIDv7 для новых Core entities, RUB в точных копейках, один счёт компании.
Core владеет моделью, версией снимка и календарём; API остаётся adapter.
ML inference выполняется вне SQL Tx. Repositories не координируют use cases;
переиспользуются feature-local Work/DTO, app-owned Tx и consumer interfaces.
Ориентир — cosmohack app/postgres из [Go references](../references/go-services.md);
оригинальные checkout референсов при этом планировании повторно не проверялись.

В скоупе: dataset_v1 CSV, факты/EOD, категории и гипотезы регулярности,
ручной календарь с match/unmatch, 30 дней прогноза, причины, probability gate,
доступ к компании, общие проверки и подготовка сдачи.
Ранее написанные UI, proto и Python rules переиспользуются.

Вне ближайшего среза: произвольные банковские CSV/XLSX/PDF, live bank/ФНС/1С,
автоматический налоговый расчёт, новые брокеры/сервисы, несколько счетов/валют,
горизонты 60/90, полноценные what-if/recommendations, invitations/reset/email delivery.
Моки 1С/ФНС остаются P1; `tools/mock_import` не является runtime импортом Core.
Регистрация показывает только checkbox и ссылки consent/privacy, сохраняя
version/digest/stale handling; полный текст документа в форму не добавляется.

## 3. D2.0 — согласовать блокирующую семантику и контрактные изменения

Статус: **Принят разработчиком**. Владелец поручил commit/push в PR и следующий
этап после предъявления результата D2.0. Это разрешает публикацию D2.0,
но не merge/deploy.
Пакет точных решений подготовлен в
[financial-demo-decisions](../../docs/core/financial-demo-decisions.md).
Разделы 1–6 утверждены владельцем ответом «да», ADR-030. Canonical proto,
OpenAPI/map, read projections анализа, schema fixtures и bindings обновлены.
Buf lint/format/breaking/drift и сборка всех потребителей прошли; 58 REST→RPC
связей проверены. Core/API race/vet/full lint проходят; ML 41 RPC-тест без skips,
11 schema-тестов, Ruff и diff-check прошли. Независимые ревью проведены,
подтверждённые замечания исправлены. Узкая runtime-правка только запрещает
новые неподдерживаемые v2-only поля в ML v1. V2/import/payment runtime не активирован.
Численные gates качества остаются предложением; D2.0 принят явным поручением владельца.
Результат: зафиксированы решения, без которых следующие реализации несовместимы.
Компоненты: `docs/core/{contract,financial-ml,api-adapter}.md`, `docs/api/openapi.yaml`,
`proto/{core,ml}/v1/`, `docs/data-contracts/`, этот план; ADR только для новых решений.

Предложения для согласования:

1. **Coverage.** CSV не доказывает нулевые движения в отсутствующие дни.
   Для предоставленного synthetic dataset предложен явный профиль
   `dataset_fixture_assumption:<hash>` с ограничением
   `dataset_coverage_assumed_not_bank_certified`, воспроизводимым окном и EOD.
   Профиль разрешается сервером для изолированного dataset demo, а не произвольным
   браузерным `certified=true`. Иначе прогноз остаётся insufficient до получения
   действительного proof. Min/max даты и reconciliation сами не дают сертификацию.
2. **Proof после изменений.** Минимальное предложение: trusted category/payment
   command в своей Tx переносит прежнее доказательство на новую revision только
   если набор/экономика фактов не изменились. Изменение фактов требует переоценки;
   blanket-copy при любом data_version запрещён. Match использует уже принятый
   факт, не создаёт факт исполнения. Отдельная factual revision — альтернатива,
   если перенос окажется сложнее; не реализовывать обе схемы.
3. **Календарь при историческом cutoff.** Данные заканчиваются сентябрём,
   ручной платёж вводится в октябре. Предложено явно типизированное расширение
   knowledge mode для synthetic scenario с provenance и ограничениями через
   REST/Core/ML; точное имя и поля фиксируются до генерации. Он не считается
   HISTORICAL_AS_OF_CERTIFIED, не попадает в historical backtest и не маскируется
   под CURRENT с выдуманной датой/отсутствием движений. Для реального CURRENT
   сохраняются свежий EOD и проверка no_post_cutoff_movements.
4. **Reconciliation календаря.** Existing Payment DTO не содержит основания
   `additional_obligation_confirmed` или complete replacement group. Предложено
   минимальное typed расширение write/read DTO и UI: пользователь явно отличает
   новое дополнительное обязательство от замены группы исторических операций.
   Replacement содержит проверяемые IDs и подтверждение полноты на горизонт;
   совпадение категории/описания не подтверждает полноту автоматически.
   Неоднозначный платёж показывается как не включённый до уточнения.
   Зафиксировать durable identity/lifetime группы отдельно от активного платежа:
   cancel/полное исполнение/выход даты за горизонт по умолчанию меняют occurrence,
   но не отменяют подтверждённое replacement-сопоставление. Complete group может
   иметь ноль оставшихся occurrences; отзыв mapping — явное отдельное действие.
5. **Категории/регулярность.** Предложен анализ только 90-дневного окна текущего
   Predict: существующее classified_operations плюс typed recurring hypotheses.
   Это ограничение нужно принять явно; анализ всей загруженной истории потребует
   другого согласованного объёма входа. Core хранит derived evidence по
   snapshot/model/operation version, не меняя факты в процессе forecast.
   Manual override приоритетнее; mapping semantic code ↔ tenant category UUID
   и seed справочника принадлежат Core. Старые/ещё не рассчитанные строки UI
   показывает как «не анализировались». Гипотезы не создают обязательства сами.
6. **Доступ.** Предложен самостоятельный company onboarding в D2.1. Операторская
   пустая demo-company допустима для параллельных интеграционных проверок, но
   не заменяет пользовательскую приёмку и не должна содержать manual facts,
   конфликтующие с первым external source binding.
7. **Качество.** До fitting зафиксировать событие вероятности, допустимую truth
   coverage, split, метрики и численные promotion gates; не выбирать пороги
   после просмотра результатов. Не гарантировать калибровку к дедлайну.

Работы и проверки:

- [ ] Записать выбранные варианты и точные новые поля/enum/errors/presence,
  analysis version, proof lifecycle, upload storage/retention и модельный контекст.
- [ ] Выбрать private durable staging bytes (ограниченное SQL-хранилище либо
  private volume с восстановлением/очисткой orphan files); до 202 bytes/job
  должны переживать restart. Storage choice и ресурсный предел зафиксировать.
- [ ] Согласовать только 30 дней в UI; остальные значения API явно отклоняет
  как unsupported для выбранного профиля, даже если общий OpenAPI шире.
- [ ] Обновить canonical proto/OpenAPI/mapping/consumer generation согласованно;
  без второй версии контракта в service-local handwritten DTO.
- [ ] Проверить Buf lint/build/breaking, generated drift, REST→RPC coverage,
  backward reads старых snapshot/result и negative presence/enum fixtures.

Завершение: решения 1–7 рассмотрены владельцем; нет невидимого допущения, от
которого зависит импорт/календарь. После результата — ревью и апрув.

## 4. D2.1 — рабочий вход в компанию без операторской membership

Статус: **Не начат**. Зависимости: D2.0 принят.
Результат: пользователь создаёт одну компанию/счёт и получает owner membership
через durable Core receipt; Workspace работает независимо от P1 invitations.

Пути: Auth `internal/{service/account,handler/grpc,app}`, feature repositories/SQL
и миграция для creation command, предлагаемый `internal/client/core/`;
Core существующие `service/company`, `handler/grpc/company` и receipt;
API `internal/{app,handler/http}`; frontend `src/features/Workspace`, company services/tests.

- [ ] Реализовать BeginCompanyCreation → авторизованный Core bootstrap →
  CompleteCompanyCreation с серверным чтением committed receipt; восстановление
  одного command после timeout/restart. Не доверять browser-утверждению об успехе.
- [ ] Подключить `POST /companies`, current account/membership refresh;
  не создавать внешний source binding до commit импорта.
- [ ] Разделить загрузку companies и invitations. Недоступный P1 panel не должен
  скрывать доступные компании или показывать фиктивный успешный ответ.
- [ ] Сохранить strict/demo email границы, проверку пароля, session/CSRF и
  повторную company authorization в Core; не добавлять новые Auth механизмы.

Проверки: реальные Auth/Core/PG, retry после Core commit, повтор команды,
concurrent completion, отсутствие receipt, чужой command, revoked session,
чужой company ID; браузер register/verify/login/create/select company/reload.
Завершение: одна company/account/membership без ручного SQL; invitations failure
не блокирует путь. После результата — ревью и апрув.

## 5. D2.2 — durable upload, валидация и preview через существующий UI

Статус: **Принят разработчиком** ответом «апрув дальше» после предъявления
реализации и ограничений кандидата. Разрешён переход к D2.3; новая публикация,
merge и deployment этим ответом отдельно не поручены.
D2.0 принят; D2.1 выполняется отдельно. Полная пользовательская приёмка D2.2
остаётся открыта до перечисленных ниже browser/Auth/resource проверок.
Код и основные автоматические проверки D2.2 готовы; фактический результат,
команды продолжения и непроверенные сценарии:
[handoff D2.2](../handoffs/financial-demo-d22.md).
До полной приёмки остаётся browser/real Auth rehearsal и проверка максимального
файла под deployment resource limits. Защитные пределы кандидата 64 KiB/logical
CSV record и 1000 EOD dates в preview описаны явно; при превышении preview
блокируется, факты не применяются. Этап не помечается принятым автоматически.
Владелец поручил начать нашу часть параллельно onboarding. До его готовности
изолированные проверки используют пустую test company; это не приёмка D2.1.
Результат: CSV доходит до Core, после restart доступен preview; Ledger не меняется.

Новые Core feature paths: `internal/{domain,service}/imports/`,
`internal/repository/postgres/imports/{record,sql,queries.go}`, `handler/grpc/imports/`,
versioned migration. Существующие: app/config/grpcserver; API client/core,
предлагаемые `handler/http/imports*.go`, app/router; frontend Imports/imports_service/tests.
Deploy stack/nginx config — только если нужны согласованные body/storage limits.

- [ ] Использовать ImportsService lifecycle, bounded worker/lease и durable bytes;
  не вводить брокер. API принимает bounded multipart, private spool, считает
  size/hash, передаёт metadata первым frame; Core пересчитывает bytes/hash.
- [ ] Выполнить ≤100 MiB/300000 rows, chunks ≤1 MiB с учётом protobuf overhead
  (нынешний Core MaxRecvMsgSize ровно 1 MiB недостаточен для такого payload).
  Настроить отдельный stream peer interceptor/deadline/cancellation; unary
  interceptor не защищает UploadImport. Согласовать nginx/API body/read timeout.
- [ ] Парсить только dataset_v1: BOM/`;`, обязательные колонки, точные суммы,
  ISO dates, направление, строковый ИНН. Выбор одной внешней компании; чужие
  строки aggregate исключаются с видимым count, не создают tenants.
- [ ] Preview хранит base_data_version/preview_version, issue paging и can_commit.
  Обрыв, digest mismatch, repeated metadata и oversize не создают accepted job.
- [ ] Подключить уже существующие UI polling/preview/issues; временные private
  файлы очищаются при отказе, accepted uploads — по согласованной retention.

Проверки: настоящий HTTP multipart → mTLS streaming Core → PG/storage, restart
после 202, replay key/body conflict, bad peer/CSRF, отмена, полный paging issues,
limits на каждом hop. В preview нет ни facts, ни source binding.
Завершение: браузер получает воспроизводимый preview/error без применения данных.
После результата — ревью и апрув.

## 6. D2.3 — атомарное применение истории, coverage и чтение операций

Статус: **Принят владельцем 25.10.2026**. Core `task check` на `1b4c702` прошёл.
Браузерный путь file → preview → commit → list → forecast/reload на настоящем
Auth не выполнен: публичный сервер без imports, локальный стек без env/PKI и
образов, deploy не разрешался. Этот предел не считается закрытым приёмкой.
Результат: commit сохраняет историю/EOD и источник; операции доступны в UI,
из принятой истории можно получить новый forecast без ручной вставки proof SQL.

Пути: imports; `service/ledger`, `repository/postgres/{ledger,forecastjobs}`,
`handler/grpc/ledger`, migrations/app; API новые `handler/http/ledger*.go`,
imports commit mappings; frontend Imports/Transactions/account context и services.

- [ ] Одна app-owned Tx: reauthorize, CAS, warnings ack, scoped source binding,
  batch фактов/EOD, revision, proof по D2.0, receipt и job state. Не вызывать
  по строке публичный CreateTransaction с отдельной Tx. Resource locking:
  idempotency key → account → import/resources; rollback не оставляет часть истории.
- [ ] Ключ дедупликации: scope + dataset namespace + source company + external ID.
  Повтор эквивалентен; изменившееся содержание под тем же ID — blocking conflict.
  Aggregate/per-company упаковки дедупятся независимо от filename/hash;
  похожие строки с разными ID сохраняются с предупреждением.
- [ ] Свернуть одинаковый EOD за день в одно наблюдение с row evidence;
  конфликт EOD блокирует commit. Reconciliation residual сохраняется как issue,
  не исправляется выдуманной транзакцией. Нет implicit zero/forward-fill unknown days.
- [ ] Реализовать ListTransactions/ListCategories/UpdateTransactionCategory,
  seed semantic mapping, paging/filter/cursor scope и ручной override/clear.
  Для точного no-op replay не повышать revision повторно.
- [ ] Реализовать выбранный proof lifecycle: payment/category mutation не ломает
  валидный forecast; factual mutation не наследует старое доказательство молча.
  После commit обновить account context/readiness и stale-индикацию в UI.

Проверки: реальная PG Tx, concurrent commit/replay, crash/rollback, stale preview,
отсутствующий ack, source/manual history conflict, другая company/account,
exact large money/INN, category clear/CAS; <90 дней, missing cutoff EOD,
unknown coverage и reconciliation не превращаются в ready/no risk.
Завершение: browser file → preview → commit → list → forecast/reload; source
и proof получены штатным use case, старый forecast помечен stale и не переписан.
После результата — ревью и апрув.

## 7. D2.4 — runtime категории и объяснимая регулярность

Статус: **Не начат**. Зависимости: D2.3 принят и scope анализа из D2.0.
Результат: у рассчитанного 90-дневного окна есть объяснимые automatic labels
и recurrence hypotheses с источником; финансовые факты неизменны.

Пути: `services/ml/cashflow_baseline/{categories,recurring}.py`,
`cashflow_rpc/financial.py`, tests; `proto/ml/v1/forecast.proto` и нужные Core/REST
read DTO; Core `client/ml/{financial,financial_evidence}.go`, service/forecasting,
analysis repository/SQL и schema version; API mappings; frontend Transactions/
ForecastDetail/interfaces/services/tests.

- [ ] Переиспользовать pure categorization/monthly fit helpers, а не весь offline
  loader/daily_as_of, который содержит dataset assumptions. Сохранить пороги
  подтверждений: не снижать их ради красивого примера; 90 дней могут не дать паттерн.
- [ ] Один Predict возвращает derived category/source/regularity и typed pattern
  evidence: operation IDs/versions, окно/cutoff, правило/версия, direction/party,
  confirmations, статус hypothesis. Закрепить новую analysis/preprocessing version;
  прежний финансовый профиль не менять молча и не продвигать новый default без gate.
- [ ] Core проверяет неизменность factual полей вместо proto.Equal всего output;
  валидирует allowed derived fields, manual precedence, IDs/evidence и mapping
  code→tenant UUID. Derived analysis хранится отдельно и читается лишь для
  совместимой версии факта/снимка; forecast не создаёт facts и не повышает их revision.
- [ ] UI различает ручную категорию, результат анализа, неанализированные строки
  и неопределённую метку. Показывает гипотезу регулярности/источники; она не
  становится подтверждённым платежом или complete replacement group автоматически.

Проверки: детерминизм, manual override, факт изменился, чужой ID/evidence,
операции после cutoff, окно до/после 90 дней, no pattern, unknown analysis version,
reload без мутации Ledger/Payments. Категории не удваивают residual forecast.
Завершение: реальные импортированные данные дают прослеживаемый runtime результат;
scope анализа и отсутствие измеренной accuracy указаны честно. Ревью и апрув.

## 8. D2.5 — платежи, frozen календарь и причины нового прогноза

Статус: **Не начат**. Зависимости: D2.3 и D2.4 приняты; решения календаря D2.0.
Результат: create/edit/match/cancel платежа меняет новый forecast ровно один раз.

Пути: Core `service/payments`, `repository/postgres/payments`, новый
`handler/grpc/payments/`, grpcserver/app; domain/forecasting snapshot + codec,
`repository/postgres/forecasting/read_inputs.go`, `service/forecasting/job_{input,worker}.go`,
`client/ml/financial*.go`, handler/forecasting encode; ML financial.py и tests;
API client/core, новые payments handlers/mappings; frontend Calendar/ForecastDetail/
Dashboard, payments_service/forecast DTO/tests; миграции/versioned snapshots.

- [ ] Подключить list/get/create/update/cancel и match/unmatch с остатками,
  resource versions, replay и existing domain invariants. Match связывает факт,
  не исполняет платёж и не создаёт вторую транзакцию. Сохранить source provenance.
  Согласовать UI actions со state machine: отмена remaining доступна также для
  partially_matched, существующая история match доступна после cancel. Unmatch
  после cancel разрешать только при явно определённом контрактом переходе;
  если переход запрещён, проверить явный отказ, а успешный unmatch — до cancel.
- [ ] Сохранять reconciliation mapping/knowledge mode, выбранные в D2.0.
  В Tx snapshot замораживает facts, EOD, remaining/status/version, календарные
  группы, availability/evidence. Старые v1 snapshots читаются; new version
  не переинтерпретирует сохранённые прогнозы и не читает изменённый live calendar.
- [ ] Передавать реальный FinancialCalendar в единый Predict; unavailable и
  подтверждённый пустой календарь различаются. Не утверждать полноту группы
  по одному найденному платежу; не сдвигать overdue в первый день молча.
- [ ] Go validator принимает known_remaining_payment и residual components,
  проверяет replacement IDs, непересечение групп, принадлежность source IDs,
  category mapping и точную сумму всех components по каждому дню. Убрать
  требования HISTORY_ONLY_NO_CALENDAR только там, где календарь действительно доступен.
  Независимо проверить residual mean по истории после replacement плюс known
  remaining по датам: прежняя проверка равенства каждого дня среднему всей истории
  несовместима с календарём и заменяется только для согласованного нового профиля.
  Сохранять complete replacement groups даже без активных occurrences: cancel
  или полный match не должны внезапно возвращать вычтенную историю в residual.
- [ ] Core отдаёт сохранённые components/evidence/effective occurrences вместо
  пустых массивов. UI показывает даты/остатки/причины, 30 дней, loading/error/stale;
  действующий механизм forecast reuse не возвращает старую revision как новую.

Проверки: реальный Go→Python mTLS и browser lifecycle create→forecast→amount
edit→date edit→partial match→unmatch→partial match→cancel, reload/restart;
отдельно доступ к истории и разрешённое действие/явный отказ unmatch после cancel;
новая версия и stale
старого результата. Отмена remaining не удаляет исполненный факт из истории.
Отдельно: additional obligation, complete replacement, overlap, overmatch,
недоказанная availability, post-cutoff движение, unknown mode, overflow, чужой tenant,
конкурентный edit/CAS, ML unavailable. Ни fallback на baseline, ни двойного учёта.
Для complete replacement отдельно проверить cancel/полный match/перенос за
горизонт: без явного отзыва mapping исторический residual не возникает снова.
Завершение: известный платёж и изменённый остаток объяснимо отражаются в новых
30 дневных точках; старый snapshot/result побайтно не меняется. Ревью и апрув.

## 9. D3.1 — качество и обоснованная вероятность

Статус: **Не начат**. Протокол зависит от принятого D2.0; интеграция кандидата —
от D2.5 и отдельного решения по его качеству. Диагностика возможна параллельно
остальным работам только в пределах согласованного этапа, без смены default.
Результат: воспроизводимый отчёт и решение promote/reject по заранее заданным воротам.

Пути: `services/ml/cashflow_baseline/{evaluation,*experiment}.py`, research/tests,
`.agent/research/` report/protocol; при успешном gate — versioned ML context,
Go evidence/probability decoder, Core result codecs, API/UI mapping/tests.

- [ ] До обучения определить horizon event «хотя бы один отрицательный EOD за
  30 дней», отдельно per-day probability, пригодные truth windows/coverage,
  minimum positives/sample size, company/time split, Brier/reliability и baseline
  comparison, FP/FN/date/depth и численные границы принятия с владельцем.
- [ ] Reuse rolling-origin; никаких future labels/recurrence/calendar leakage.
  Учитывать зависимость перекрывающихся окон/компаний; уже исследованный поздний
  набор не называть новым независимым holdout. Synthetic calendar metrics отдельно.
- [ ] Выполнить/получить человеческую слепую разметку категорий и регулярности,
  confusion/precision/recall по согласованным срезам. Подготовленные 260 примеров
  сами по себе не измеряют accuracy; это внешняя зависимость на участие человека.
- [ ] Если gates пройдены, выбрать явную новую model/config/preprocessing version
  в Core, затем согласованно поддержать её validator/probability/evidence в каждом
  слое. Текущий validator жёстко принимает только uncalibrated и нуждается в изменении.
- [ ] Если независимых данных/качества недостаточно, оставить default и `null`
  с причиной. Отчёт может быть готов, но подзадача probability и полный P0
  остаются незакрытыми; не считать долю bootstrap траекторий готовой вероятностью.

Проверки: воспроизводимость на закреплённых inputs/hash/seed, held-out report,
no calibration leakage, negative zero-vs-missing/NaN/range, unknown version
без fallback, validator принимает только согласованный профиль; график не
обозначает empirical bands как проверенный confidence interval.
Завершение probability: заранее согласованные gates действительно пройдены
на допустимом evaluation, результат проходит настоящий Core/API/UI. Иначе
stage остаётся незавершённым в этой части. После отчёта — ревью и решение владельца.

## 10. D3.2 — общая приёмка и репетиция

Статус: **Не начат**. Зависимости: D2.1–D2.5 приняты; D3.1 имеет рассмотренный
результат. Незакрытая probability явно отмечается и не исчезает из общей приёмки.
Результат: один воспроизводимый integrated test/demo с настоящими сервисами.
Пути: feature/integration tests, предлагаемый bounded E2E script под `scripts/`,
`frontend/tests/`, CI workflows, `docs/integration-status.md`, verification report,
при необходимости расширение `deploy/stack/smoke.py` после согласования миграций.

- [ ] Из пустой изолированной test DB/новой компании: register/login → company →
  file upload → preview/issues/ack → commit → ledger/category → forecast →
  hypothesis/evidence → payment lifecycle → новый прогноз → reload/restart.
  Без вставки финансовых данных/coverage через SQL, финансовых mocks и reuse
  существующего operator fixture вместо проверки импорта.
- [ ] Проверить duplicate file/key, invalid file, partial upload, stale preview,
  короткую/неполную историю, EOD conflict, worker crash/recovery, ML failure,
  session expiry/revocation, CSRF и чужие scope/resource/cursor.
- [ ] Проверить demo любой шестизначный ASCII-код только в server-enabled
  allowlist профиле; strict отвергает неверный код, VerifyEmail без автологина.
  Consent links/version/digest/stale-document handling сохраняются.
- [ ] Запустить точные gates ниже; отчёт привязать к source SHA и artifact hashes.
  Независимые caveman-review и ponytail-review после diff; исправления повторно
  проверять по затронутым invariants. Для Go дополнительно соответствующие skills.
- [ ] Обновить сводку фактической готовности и связанные старые статусы только
  на основании результатов/апрува. Репетиция: проблема, файл, календарь, риск,
  действие пользователя, источники и честные ограничения; резервная запись.

Завершение: основной сценарий воспроизводим; ошибки не выглядят нулевым риском;
качество/готовность/публикация/deployment раздельно подтверждены. После результата —
итог этапа, ревью и апрув, с отдельным перечнем незакрытых требований.

## 11. D4 — стабилизация и подготовка поставки

Статус: **Не начат**. Зависимости: рассмотрен D3.2; окно владельца 25.10 09:00–12:00 МСК.
Результат: проверенный кандидат, инструкции/запись/ограничения для сдачи.
Пути: только обнаруженные блокеры и release/verification docs, существующий CI/deploy.

- [ ] Не добавлять функции в окно стабилизации; устранить согласованные блокеры.
- [ ] Зафиксировать release SHA, schema compatibility, migration/rollback plan,
  рабочие fixture/сценарий/метрики, ограничения и резервную демонстрацию.
- [ ] Перед разрешённой публикацией fetch свежего main, integration checks и CI
  на итоговом SHA. Main stack workflow автоматически развёртывает релевантные
  изменения: согласование merge должно учитывать этот эффект.
- [ ] Только при отдельном поручении публикации/поставки: immutable images,
  backup, explicit migrations, existing smoke и новый dataset/payment smoke,
  public browser/restart/persistence; записать фактический revision/digest.
  Старый frontend-mock workflow поверх real API приложения не запускать.

Завершение: к 12:00 готов заявленный комплект; оставшиеся требования отражены
честно. Текущий план не разрешает commit/push/merge/deploy, изменение серверных
scripts/PKI, Fadwix/GBrain/Grafana/чужих контейнеров. Откат images/config не
означает downgrade БД; новые миграции должны сохранять совместимость.

## 12. Мультиагентная организация реализации и общие проверки

Три исследовательских агента уже сверили Core/import, API/frontend/Auth и ML.
Их выводы проверены координатором и отражены выше; агенты не меняли runtime.
Для длительной работы удалённого разработчика подготовлено самодостаточное
[поручение по company onboarding](../prompts/company-onboarding-autonomous.md):
один draft PR с ранней публикацией, push после каждого законченного slice,
совместно версионируемый handoff и итоговое ревью владельца. Поручение
передано владельцем удалённому разработчику; состояние его запуска/PR пока
не подтверждено здесь. Оно изолирует D2.1 от решений о финансовом
coverage/календаре/ML; пересечение с нашей работой — только согласуемый wiring.
При последующей реализации состав внутри принятого этапа:

| Исполнитель | Владеет | Передаёт соседям |
| --- | --- | --- |
| Координатор/contracts + QA | stage state, proto/OpenAPI/map/generation, общий diff/E2E | Зафиксированные DTO/errors/versions/fixtures; контрактные файлы меняет один владелец |
| Core/data | imports/ledger/payments/snapshots/SQL, при D2.1 Auth onboarding по согласованному назначению | Реальные RPC, migrations, replay/coverage/calendar evidence |
| API/frontend | HTTP mappings/upload, готовые экраны и browser tests | Запросы/ответы/conformance, loading/error/stale и пользовательский сценарий |
| ML/quality | Predict analysis/calendar/probability и evaluation | Pinned profiles/evidence/limits и real RPC fixtures |

Максимум три исполнителя плюс координатор. D2.1 Auth назначается явно одному
исполнителю, а не одновременно двум. Общие proto, app wiring и миграционные
номера резервирует координатор; shared checkout не допускает конкурирующих
правок одного файла. Отдельные worktrees — при пересечении изменений, без
переноса незавершённой работы участников и без самостоятельного push.

Параллелизм внутри этапа: сначала фиксированный DTO/fixture, затем provider и
consumer задачи, затем реальный integration. Mock unit-test допустим для mapper,
но не доказательство интеграции. Следующий этап по умолчанию начинается после
апрува текущего; параллельные ветви/отложенное ревью требуют отдельного явного
согласования владельцем и не выводятся из просьбы «мультиагентный режим».

Обязательные gates по затронутым компонентам:

- Core: `task check`, `task integration` в `services/core/`, реальные marked test
  PostgreSQL, race/concurrency/replay; integration-tag vet/lint по существующему CI.
- API: `bash scripts/check.sh` из `services/api/` (generation/build/unit/race/
  vet/lint/conformance/process smoke). Auth из `services/auth/`: `go test -race ./...`,
  `go test -race -tags=integration -count=1 ./...`, `go vet -tags=integration ./...`,
  `golangci-lint fmt --diff`, `golangci-lint run ./...`, `go build ./...`;
  реальные PostgreSQL/mTLS проверки с pinned toolchain и marked test DB.
- Контракты: `scripts/check-core-contracts.sh`, Buf lint/build/breaking,
  generated drift, consuming modules, OpenAPI conformance и schema examples.
- ML: `python -B services/ml/check_rpc_tests.py` без skips; financial/manual-payment
  и новые analysis tests; Ruff check/format с pinned инструментами, data-free CI
  плюс отдельные воспроизводимые dataset evaluation artifacts.
- Frontend из `frontend/`: `npm test`, `npm run typecheck`, `npm run lint`,
  `npm run build`; настоящий браузер с real API, financial mocks выключены.
- При затронутом deploy: Compose/scripts/actionlint, migrations с совместимостью,
  failure/rollback regression, upload limits nginx/API/Core; реальное публичное
  изменение только по отдельному разрешению. Успех старого CI не заменяет новый SHA.

## 13. Общая приёмка, риски и вопросы владельцу

Функциональный D2 принят, когда пользователь через UI применяет CSV к своему
счёту, видит provenance/категории/регулярность в согласованном окне, добавляет
известный платёж и получает новый сохранённый 30-дневный прогноз с объяснимой
датой/суммой риска; повтор/изменение/отмена не удваивают данные и суммы.
Чужие данные недоступны, старые результаты неизменны, недостаточность данных явна.
Полный P0 дополнительно требует принятого качества и обоснованной вероятности.

Основные риски: неполная CSV история/нет свежего EOD; proof revision; историческая
доступность ручного календаря; невыраженная reconciliation семантика Payment;
90-дневное окно не даёт достаточно повторений; нет независимой risk-разметки;
ограниченные deadline/host resources и durable upload объём; миграции под
автоматическим main deployment. Все имеют конкретный gate выше.

Для завершения D2.0 нужны решения по подготовленному пакету, прежде всего:

1. Принимаем ли явно ограниченный dataset coverage profile и отдельный
   synthetic scenario mode для исторической демонстрации?
2. Принимаем ли 90-дневный runtime анализ категорий/регулярности через один
   Predict, с маркировкой остальных операций как неанализированных?
3. Оставляем ли onboarding обязательным этапом этой поставки, либо владелец
   явно сокращает ближайший demo scope до подготовленной пустой компании?

Предложения не становятся Accepted ADR, статусом «Принят» или разрешением
реализации только из-за сохранения этого файла. После согласования D2.0
оцениваем выполнимый объём до дедлайна, не выдавая открытые требования за готовые.
