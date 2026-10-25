# D2.0: решения для финансового demo

24.10.2026. Разделы 1–6 **утверждены владельцем** ответом «да» после предъявления
пакета; зафиксированы ADR-030. Раздел 7 с численными gates остаётся предложением.
Контрактная реализация D2.0 выполняется отдельно от будущего runtime.
Начало исполнения: прямое поручение владельца по
[financial-demo-next-steps](../../.agent/plans/financial-demo-next-steps.md).
База: `fdee0edf6f8083acd0e9e127026149ef7c1aeb24` после fetch; PR #39
меняет только выбор компании на Dashboard. Открытых PR при проверке нет.
Onboarding D2.1 передан владельцем другому разработчику; здесь Auth и создание
компании не меняются. Документ фиксирует контракт до изменения proto/runtime.

## 1. Coverage предоставленного dataset

Принимаемый вариант: серверный allowlist manifest для изолированного demo.
Каждая запись содержит adapter=`dataset_v1`, SHA256 исходного файла, внешний
company ID, разрешённый company/account scope, inclusive `from_date`/
`through_date`, cutoff EOD и его row evidence. Profile reference:
`dataset_fixture_assumption:<sha256>`. Manifest хранится в закрытой серверной
конфигурации; браузер не передаёт `certified`, coverage или разрешённый scope.
Тот же профиль не применяется к другому hash, tenant или периоду автоматически.
Aggregate и per-company файлы могут иметь отдельные allowlist entries;
дедупликация фактов остаётся по namespace/external ID, а не по hash файла.

Commit проверяет digest, выбранную компанию, границы, все обязательные строки,
EOD conflicts и reconciliation. Только атомарно с успешным commit сохраняется
proof. Пропущенные дни внутри разрешённого окна считаются полными **по явно
принятому synthetic предположению**, не по min/max дат. Вне manifest окно
не расширяется. Неполная/конфликтная история не становится ready.

Readiness содержит limitation `dataset_coverage_assumed_not_bank_certified`;
она сохраняется в snapshot, ML request/response и публичном forecast.
Статус такого прогноза максимум `limited`. Банк такую полноту не удостоверял.
Старые proof без типизированного provenance не получают новый профиль по
префиксу reference: они доступны старому read path, но не дают scenario capability.

## 2. Proof и revision

Остаётся существующий ключ `(company_id, account_id, data_version)` в
`core_history_coverage`. Не вводим вторую factual revision.
Category/payment команды в той же Tx переносят proof только с непосредственно
предыдущей revision того же account, если изменены только metadata/expectations.
Переносятся исходные bounds/reference/provenance, без расширения окна.
Match/unmatch связывают уже существующие факты; факты не создаются и не меняются.
Настоящий no-op/replay не повышает revision и не создаёт новый proof.

Import с изменением фактов заново строит proof из manifest и полного результата
commit. Иные factual mutations старый proof не наследуют. Отдельно проверяются
EOD anchor и отсутствие новых фактов после cutoff для CURRENT режима.
Наличие coverage не отменяет эти проверки. Account lock сериализует изменение
revision/proof и capture snapshot; old snapshot/result immutable.

## 3. Загрузка: ресурсы, durable storage, retention

Сохраняем 100 MiB файла, 300000 CSV data rows, chunks до 1 MiB.
Лимит HTTP multipart envelope — 101 MiB, строго один file part; parser проверяет
размер файла отдельно. Избыточные поля/parts отклоняются, не пишутся без границ.
Core stream frame limit — 2 MiB с отдельным контролем payload 1 MiB, metadata
не более 64 KiB; stream peer authorization/deadline обязательны.

Core хранит bytes chunks в PostgreSQL: upload identity, ordered chunk rows,
size/hash и job. Не читает весь bytea/файл в память. Приём chunks — короткие
Tx в состоянии receiving с lease; финальная Tx проверяет EOF/size/hash и
атомарно переводит bytes+job в accepted/queued, сохраняет replay receipt.
Receiving не виден в ListImports и не запускает parser. Обрыв не даёт 202.
Сеть и CSV parsing не удерживают SQL Tx. Parser читает chunks постранично;
нормализованные preview rows/issues тоже durable, не один объект на 300000 строк.

Предлагаемые исходные квоты: 2 незавершённых job и 200 MiB raw на account;
1 GiB raw глобально. Reservation по объявленному размеру и реальный размер
контролируются атомарно до/во время записи. Quota exhausted → 429 RATE_LIMITED,
file/row limit → 413 PAYLOAD_TOO_LARGE. Квоты не гарантируют ёмкость normalized
rows/WAL: при отказе storage никакого 202; нагрузочная проверка — gate D2.2.

API сначала spool/hash-check, затем streaming Core. Существующий `/tmp` —
tmpfs 16 MiB, API memory 128 MiB: он непригоден для 100 MiB файла. Нужен private
disk-backed spool в test/local Compose и отдельно согласуемом deploy template,
не новый сервис. Spool root не публичный, файлы 0600/каталог 0700, случайные
имена, O_EXCL, без путей клиента. Квота API spool 200 MiB, максимум 2 uploads;
reservation до чтения; после передачи/отказа unlink, startup scavenger убирает
orphan files. После API restart клиент повторяет запрос; accepted job уже в Core.
Live volume/server configuration этим документом не меняются.

Receiving lease: 5 минут, heartbeat во время приёма; истёкшие reservations/chunks
удаляются worker. Квота Core освобождается атомарно с физическим удалением chunks,
ровно один раз по upload identity; reservation не освобождается раньше bytes.
Повтор cleanup идемпотентен. API освобождает slot/bytes после unlink; startup
scavenger сверяет reservations с файлами, не сбрасывает квоту до удаления.
Raw bytes сохраняются до 24 часов после первого terminal
состояния APPLIED/FAILED. PREVIEW_READY без commit истекает через 7 суток от
acceptance → FAILED с IMPORT_STATE_CONFLICT и безопасным сообщением о повторной
загрузке; активный parser с lease cleanup не удаляет. Просроченный parser lease
возвращает job на повторную обработку. Metadata, preview, issues и idempotency
receipt сохраняются в ближайшем срезе без автоматического удаления; cleanup raw
не меняет GET/replay принятия. Финансовые факты и их provenance не удаляются
при cleanup staging. Это техническая политика staging, не политика удаления
персональных данных/backup: её нельзя выдавать за согласованное legal retention.

## 4. Исторический synthetic calendar

В ML `CalendarKnowledgeMode` добавляется `SYNTHETIC_SCENARIO = 4` (полное имя
с прежним префиксом). `FinancialCalendar` поле 8 — `SyntheticScenarioContext`:
`scenario_reference` string = 1, `recorded_at` Timestamp = 2,
`scenario_business_date` string = 3. Последнее равно cutoff; recorded_at —
реальное время ввода сценария. `as_of_business_date` пустое,
`no_post_cutoff_movements=false`. Контекст запрещён в остальных knowledge modes.

Режим доступен только при server-enabled manifest/profile из раздела 1,
подтверждённом выборе пользователем historical demo и frozen scenario identity.
Он не утверждает, что октябрьский remaining был известен в сентябре.
Synthetic mode не скрывает post-cutoff settlement: match допустим для прогноза,
только если факт уже учтён в исходном EOD; иначе нужны новый cutoff и coverage.
Обязательная limitation: `synthetic_calendar_scenario_not_historical_fact`.
Не используется для historical backtest, calibration или оценки реального риска.
CURRENT/HISTORICAL_AS_OF_CERTIFIED/UNAVAILABLE сохраняют прежние ограничения.

Внешний запрос выбирает `calendar_mode` (`available` по умолчанию либо
`synthetic_scenario`); доступ/сценарную identity/provenance определяет Core,
не браузер. Idempotency fingerprint и job reuse включают mode/context.
Core input/read DTO, immutable job input, ML response и REST read projection
сохраняют context и limitations, UI явно подписывает исторический сценарий.
Точные additive поля Core/REST проверяются вместе с generation после апрува;
старый отсутствующий context читается как прежнее поведение, не как scenario.

## 5. Reconciliation и lifetime группы

Payment получает отдельное typed reconciliation: `unresolved`,
`additional_obligation_confirmed`, `complete_replacement`. Отсутствие поля у
старого платежа означает unresolved; автоматически выбирать по категории нельзя.
Additional не содержит historical IDs. Complete replacement содержит stable
group UUIDv7, unique scoped historical operation IDs/versions и подтверждение
полноты на точный forecast horizon. Группа — отдельная persisted сущность;
payment ссылается на неё, один payment принадлежит максимум одной группе.

Group — tagged union с kind=`additional` или `replacement`. Additional group
содержит явное actor/time подтверждение, пустые historical refs и не исключает
историю. Replacement требует непустые historical refs и complete_for_horizon.
Unresolved payment не имеет active compatible group. Каждый включённый scheduled
payment ссылается на группу, включая additional: это соответствует ML contract.
Group хранит account scope, version, active/revoked, cutoff/window, confirmations,
historical refs и actor/time. IDs не пересекаются между active replacement
groups одного snapshot. Нужны чтение и explicit revoke с CAS/CSRF; никакого
автоматического revoke при cancel, полном match или выходе даты за горизонт.
Ноль future occurrences допустим: historical IDs всё равно вычитаются из
residual. Повторное использование группы для иного cutoff/horizon требует
нового явного подтверждения; сервер не расширяет completeness автоматически.

Unresolved показывается в UI как не включённый; forecast readiness `limited`
с `unreconciled_calendar_payments`, а не утверждение полноты пустого календаря.
Matched/cancelled не создают remaining, но сохраняют group mapping.
Ошибочные scope/IDs/overlap → VALIDATION_ERROR (или NOT_FOUND без раскрытия
чужого ресурса), CAS → VERSION_CONFLICT; запрещённая state transition →
PAYMENT_STATE_CONFLICT. Не добавляем общий workflow engine.

## 6. Анализ категорий и регулярности

Один Predict, только его ровно 90 полных календарных дней/до 20000 операций;
переполнение → явный отказ, не усечение. Derived analysis хранится отдельно
от Ledger: snapshot/model/analysis version + operation ID/version.
Фактические дата/суммы/direction/provenance и ручная категория не изменяются.
Core владеет code → tenant category UUID mapping. Неанализированные строки
отличаются от `unknown`. Из анализа не создаются Payment/complete groups.

Предлагаемый профиль: model_version=`mean_90_components_v2`, preprocessing=
`calendar_analysis_v2`, configuration=`mean_90_components_analysis90_v2`,
analysis_version=`text_categories_monthly_v1`; seed=20261024, trajectories=512.
На одинаковом unavailable-calendar input все 30 денежных точек совпадают с v1.
Hypotheses ограничены 1000, evidence refs — 20000; transport 2 MiB сохраняется,
превышение даёт ошибку без усечения. Monthly anchor — day_of_month либо month_end;
weekend shift — none/previous/next. Typed expected amount >0; pattern ID строится
из snapshot/rule/version и sorted source UUIDs, без PII/сумм в идентификаторе.

Новый model profile явный, прежние v1 payload/decoder остаются доступны для
старых jobs. Новая версия не выбирается по наличию поля и не служит fallback.
В ответ добавляются typed recurring hypotheses: ID, rule/version, direction,
source operation IDs/versions, window start/end, confirmations,
period и статус hypothesis. Ни confidence правил, ни bootstrap share не
сериализуются как probability риска. Поля classification имеют только
разрешённые semantic codes и provenance; evidence не включает сырой purpose
или ИНН в логи/ошибки. UI читает финансовые данные только через company scope.

## 7. Качество и поставка

Вероятность события: хотя бы один отрицательный EOD в следующих 30 днях;
per-day probability — отдельная оценка. Synthetic calendar исключён из её
подтверждения. До обучения нужен versioned protocol со split, достаточностью
independent company/time windows, численными Brier/reliability/FP/FN gates
и baseline. Уже исследованные окна не объявляются новым blind holdout.
Пороги утверждает владелец до fitting; до этого probability остаётся
`not_calibrated`, задача вероятности открыта. Человеческая разметка категорий
также не считается сделанной наличием sample CSV.

Численные предложения для отдельного утверждения **до fitting**:

| Проверка | Предложенный gate |
| --- | --- |
| Категории | ≥200 слепо размеченных операций, ≥20 компаний; macro precision ≥0.85, classified coverage ≥0.60; ≥10 примеров на заявленный класс |
| Recurrence hypotheses | ≥30 размеченных гипотез, ≥10 компаний; precision ≥0.90 |
| Calibration sample | ≥200 неперекрывающихся окон, ≥50 positives/50 negatives, ≥20 компаний |
| Untouched evaluation | ≥500 окон, ≥100 positives/100 negatives, ≥30 компаний; group/time separation |
| Brier | Skill относительно train-only prevalence baseline ≥0.05; нижняя company-cluster bootstrap 95% граница >0 |
| Reliability | Error ≤0.05 по 5 bins, зафиксированным на calibration; ≥20 evaluation observations/bin |
| Решение по threshold | Threshold фиксируется на validation; evaluation recall ≥0.80 и FPR ≤0.10 |

Это предложения критериев, не измеренные результаты. Недостаточный sample
означает `insufficient`, не pass. Per-day оценка требует собственного протокола.
Даже calibrated history-only profile не разрешает probability для synthetic
calendar без отдельного conditional validation: `context_not_validated`.

## Проверка и следующий шаг

Предлагаемая additive карта (номера свободны на указанной базе):

| Контракт | Новое поле |
| --- | --- |
| ImportPreview | HistoryCoverage history_coverage = 23 (предполагаемый proof) |
| Import | HistoryCoverage history_coverage = 15; optional Timestamp expires_at = 16 |
| ForecastInput | HistoryCoverage history_coverage = 14 |
| ExpectedPayment | optional string reconciliation_group_id = 14 |
| CreateManualPaymentRequest | optional string reconciliation_group_id = 10 |
| UpdatePaymentRequest | oneof group change: string group_id = 12 / Empty clear_group = 13 |
| ML FinancialCalendar | SyntheticScenarioContext scenario_context = 8 |
| ML FinancialOperation | string category_rule_id = 17 |
| ML PredictResponse | AnalysisContext analysis_context = 23; repeated RecurringHypothesis recurring_hypotheses = 24; SyntheticScenarioContext scenario_context = 25 |

HistoryCoverage: kind (unspecified/certified/dataset_fixture_assumption), bounds,
reference, data_version, limitations. Legacy absence не означает fixture.
REST read-only nullable projections сохраняют int64 версии строками.
Group RPC: CreateReconciliationGroup (idempotency key), Get/ListReconciliationGroups,
RevokeReconciliationGroup (CAS). Перенос payment между группами — CAS payment;
clear снимает связь, но не отзывает группу. Создание/отзыв группы повышает account
revision, переносит допустимый proof и делает старый forecast stale в одной Tx.
Повторное подтверждение другого окна создаёт новую группу с явным отзывом старой,
не меняет frozen snapshots. Unknown enum/presence или смешанные поля отклоняются.

Новые jobs/input/result schema2 сохраняют полный ModelContext и context/evidence.
Schema1 читается прежним decoder; старый worker не должен захватывать schema2
jobs: claim filter/deployment order — обязательный gate. Простое добавление
JSON-полей не доказывает совместимость. Raw retention не удаляет snapshot input.

Документ сверён с Imports/Payments/Forecasting/ML proto, migration 00006,
coverage query и compose limits. После утверждения разделов 1–6 обновлены
canonical proto/OpenAPI/map, schema fixtures и service-local bindings.
Buf lint/format/breaking/drift, consumer compilation, 58 REST→RPC mappings,
Core/API race/vet/full lint, 41 ML RPC-тест без skips, 11 schema-тестов,
Ruff и diff-check прошли. V1 явно отвергает новые неподдерживаемые ML поля.
Runtime v2, import storage, SQL migrations и UI этих функций не реализованы.
Независимые caveman-review/ponytail-review выполнены. Исправлены освобождение
storage quota при cleanup и tagged union additional/replacement; повторная
проверка закрыла эти замечания. Убран лишний party key из derived evidence.
Относительные ссылки и whitespace проверены; runtime этим не подтверждается.
Контрактная реализация D2.0 ожидает ревью владельца перед D2.2.
D2.1 идёт у другого разработчика; состояние его реализации здесь не подтверждено.

Разделы 1–6 уже утверждены. Численные предложения раздела 7 можно
утвердить отдельно; до утверждения обучение/продвижение вероятности не начинается.
Commit/push/merge/deploy
не выполнялись и не следуют из готовности этого документа.
