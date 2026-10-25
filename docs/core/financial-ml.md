# Единый Predict: финансовая модель и baseline

24.10.2026, ADR-029. Единственный ML RPC — `ml.v1.ForecastService/Predict`,
канонический `proto/ml/v1/forecast.proto`. Сервер выбирает реализацию только по
явно указанным model_name/model_version/preprocessing_version. Не по наличию
календаря, не после ошибки, без автоматического fallback.

## Доступные версии

Контракт D2.0 дополнен ADR-030: schema/proto описывают будущий explicit v2,
synthetic scenario и 90-дневный analysis. Runtime сейчас поддерживает только
версии таблицы ниже. V1 явно отвергает новые v2-only поля; наличие generated
DTO не означает активации v2. Новый профиль/worker/schema2 реализуются D2.4–D2.5.
Точные решения: [financial-demo-decisions](financial-demo-decisions.md).

ML `operation_id` проецируется в Core/REST `transaction_id` только после проверки
scope/version. `monthly_anchor.month_end` (protobuf Empty) — REST `month_end:true`;
false, оба anchor или ни одного недопустимы.
ML uint64 version >MaxInt64 отклоняется до Core int64 conversion. Существующий
ML category_source=`user` отображается в публичный `manual`, category UUID
сохраняется; rules/unknown имеют явный mapping. У manual/unknown category_rule_id
пустой, у rules — allowlisted stable rule ID, не произвольный текст.
Direction/weekend/status имеют
явный enum mapping; unspecified/unknown отклоняются. REST int64 остаются строками.
ForecastResult.analysis отсутствует/null для старых результатов; новые derived
данные привязаны к immutable snapshot/model, не меняют факты и ручные категории.

| Назначение | model_name | model_version | preprocessing_version |
| --- | --- | --- | --- |
| Продуктовый финансовый расчёт | mean_90 | mean_90_components_v1 | calendar_replacement_v1 |
| Явный baseline для оценки/теста | mean_90 | mean_90_v1 | daily_snapshot_v1 |

Финансовая реализация пришла из ML PR #27 head `49a7862`. Два исходных RPC
сведены к одному Predict; численная логика replacement/компонентов/empirical
bands сохранена. Ответ плоский: `days`, `components`, `uncertainty`, calendar IDs,
classified_operations, history_evidence находятся в одном PredictResponse.
Отдельного финансового service/endpoint/request/response больше нет.

Core выбирает финансовую версию для пользовательского прогноза даже с пустым
календарём и фиксирует полный model context в job. API не выбирает ML RPC/модель.
Baseline rejects operations/calendar/availability_limitations/неподдерживаемые
configuration/seed; неизвестная версия модели — FailedPrecondition. Никакого
retry baseline при отказе финансовой версии. Общая цепочка Core worker → SQL →
Go API пока не реализована; успешный ML RPC сам по себе не закрывает P0.

## Вход

`PredictRequest`: snapshot, horizon_days, model, operations, calendar,
availability_limitations. Snapshot identity/data_version/RUB совпадают с Core
immutable snapshot. Суммы int64 копейки, UUID ненулевые canonical lowercase.
Core ограничивает uint64 version значением MaxInt64 для совместимости с БД.
`model.configuration_id` у финансовой версии пустой либо mean_90_components_v1;
если seed/trajectory_count присутствуют, они точно равны 20261024/512. Это текущие
закреплённые настройки, не автоматически применяемая произвольная конфигурация.

Финансовая версия сейчас требует **ровно 30 дней** горизонта и 90 полных дней
истории. REST допускает 30–90; для неподдерживаемого горизонта явный отказ, нельзя
усекать 90 до 30. История заканчивается cutoff, initial_balance_minor явно
присутствует и соответствует принятому observed EOD именно cutoff.

DaySource OBSERVED — принятый EOD; VERIFIED_NO_OPERATIONS — подтверждённое
отсутствие операций; RECONSTRUCTED_COVERED — вычисленный остаток при подтверждённом
покрытии фактов, **не банковское наблюдение**. Baseline не принимает reconstructed.
PARTIAL/UNAVAILABLE не заполняются нулями и текущими моделями не поддерживаются.
Reference mapper `services/ml/cashflow_rpc/core_input.py` строит историю назад
от принятого EOD, требует Coverage.start/end/reference. Это shape/reference check,
а не доказательство полноты: Core обязан установить и сохранить его основание.
Неизвестные промежутки, будущие факты и нерешённые дубли не маскируются.

operations содержит unique operation_id/date/version, одно положительное
inflow/outflow, category_code/category_source (явный unknown допустим), regularity,
category UUID/company и factual provenance. Суммы operations строго совпадают
с дневными inflow/outflow. Лимит 20000 операций, 1000 calendar groups/payments,
2 MiB transport: oversize явно отвергается, данные не усекаются.

## Календарь без двойного учёта

FinancialCalendar хранит snapshot_id/data_version, knowledge_mode, дату знания,
groups и payments. Версия одна с фактами. Known payment содержит positive version,
original_due_date/effective_date и remaining, не полную исходную сумму после
частичного match. Просроченное обязательство не переносится молча: effective_date
должна быть явно определена Core и лежать в горизонте. Matched/cancelled исключаются.

- CURRENT допустим на cutoff или cutoff+1. Для следующего дня обязательно
  no_post_cutoff_movements=true: Core подтверждает, что remaining не успел
  измениться из-за факта после принятого EOD. Иначе нужен новый cutoff/bridge,
  которого текущая модель не выдумывает.
- HISTORICAL_AS_OF_CERTIFIED требует дату cutoff и подтверждённую историческую
  доступность; сегодняшний календарь в backtest не передаётся.
- UNAVAILABLE явно хранит identity фактов, пустые groups/payments/date и
  historical_calendar_unavailable в limitations. Это ограниченный финансовый
  расчёт **той же явно выбранной версии**, не переключение на baseline и не
  утверждение, что календарь подтверждённо пуст. UI обязан показать limitation.

CalendarGroup перечисляет historical_operation_ids и complete_for_horizon=true.
Исторические операции группы исключаются из среднего residual baseline; её
future remaining добавляются один раз. Полная группа без платежей означает
явно подтверждённое отсутствие будущего потока. Для нового обязательства без
исторических операций additional_obligation_confirmed=true обязателен. Пересечения,
неизвестные/повторные IDs, несовместимые суммы/даты отвергаются. Сходство суммы
или даты не выбирает replacement. Все проверки выполняет ML повторно; Core
отвечает за истинность входных утверждений и сохранённый снимок.

## Выход и Core → REST

Core проверяет весь echo snapshot/company/account/version/cutoff/model,
30 последовательных дат cutoff+1..+30, nonnegative потоки, exact signed int64
balance(t)=balance(t−1)+in−out и first_negative_balance_date. Переполнение/
невалидный ответ не сохраняется как succeeded. Reference mapper verify_response
показывает независимую проверку; не заменяет Go adapter/runtime тесты.

components содержит суммы по category/source/regularity, известные платежи и
residual mean_90_uncovered. HistoryGroupEvidence позволяет проверить source_ids.
Classified_operations сохраняет происхождение классификации и scope категорий;
это не подтверждение прав пользователя. Calendar echo и список replaced IDs
позволяют проверить отсутствие повторного учёта.

Residual mean_90_uncovered не выдаётся за обнаруженную регулярность/продажи и не
становится eligible target сценария. Core проецирует в REST только объяснимые
known_payment/ML sales/recurring components с обоснованной семантикой; остальное
остаётся в дневных суммах и assumptions. Нельзя выдумывать REST kind/ID ради
сценария. При отсутствии подходящих components capability=false. Полный wire
component имеет category_code и source IDs, а не готовый REST enum/discriminator.

uncertainty: paired moving-block bootstrap, 512 совместных 7-дневных блоков,
seed=20261024, эмпирические 0.1/0.9 bands. Они **не откалиброваны** и не являются
вероятностью разрыва. calibrated=false, unavailable reason обязателен.
ForecastDay.balance_lower/upper остаются пустыми, пока Core не принимает
интерпретацию empirical bands для публичного UI с явной маркировкой.
Probability oneof: baseline not_estimated, финансовая версия not_calibrated;
API выдаёт value:null и reason, а не 0%. Поддержка calibrated estimate в DTO —
цель, не результат текущей модели. ТЗ по вероятности остаётся открытым.

Исторический snapshot/модель/результат immutable; current_data_version только
помечает stale. Сценарий не меняет Ledger/Payments. Stress + response наследует
весь stress. Не пересчитанная вероятность сценария недоступна, не копируется
из исходной базы. Сравнения требуют одинаковый snapshot/model/seed.

## Транспорт

mTLS с allowlist Core SAN, deadline <=30s, 2 MiB, 2 workers/4 concurrent RPC.
Неверный peer — PermissionDenied; handshake — Unavailable. InvalidArgument —
неверные даты/суммы/coverage/ссылки/overflow; FailedPrecondition — неизвестная
версия/конфигурация или неподдерживаемая финансовая семантика. Отмена/timeout
не создают успешный частичный прогноз. Рабочая PKI/deployment не проверены.

## CSV с CURRENT calendar

По прямому поручению владельца поддержан отдельный профиль
`mean_90_csv_calendar_v1`, preprocessing `csv_observed_operations_calendar_v1`,
configuration `mean_90_csv_calendar_v1` (model_name `mean_90`). Это не covered или
trained context. Прежние CSV-only/covered результаты и schema1/2 читаются прежним
decoder; новый выбор фиксируется с job и snapshot, без повторного использования
старого результата при изменённой revision.

Исходные 90 дней supplied operations и cutoff EOD проходят прежний exact CSV
assessment до любого replacement. Unknown дни остаются unknown, proof/fixture
не создаётся. После допустимого replacement residual доступных операций делится
на90 (прежнее half-up округление); известные remaining добавляются однократно на
исходные даты. Groups независимы от cancel/full match/выхода платежа за горизонт.
Go независимо проверяет arithmetic/components/evidence/overflow.

Context поддерживает только CURRENT: cutoff сегодня/вчера и отсутствие post-cutoff
movements. Historical/synthetic и overdue не активируются/не сдвигаются. Limitations:
`csv_history_completeness_unverified`, `unobserved_days_unknown`,
`observed_operations_mean_per_calendar_day`, `historical_availability_unverified`,
`not_calibrated`, `current_calendar_only`. Probability unavailable, bands отсутствуют.
Обычный CSV до календаря сохраняет mean_90_csv_v1; при persisted payments/groups
и допустимом CURRENT используется явный новый profile. REST endpoints/DTO и один
Predict неизменны. Обучение/trained default не менялись.

### Runtime CSV analysis v2 (D2.4)

New ordinary CSV jobs explicitly select model_name=`mean_90`,
model_version/configuration_id=`mean_90_csv_analysis_v2`,
preprocessing_version=`csv_observed_operations_analysis_v2`. With frozen CURRENT
calendar the tuple is `mean_90_csv_calendar_analysis_v2` /
`csv_observed_operations_calendar_analysis_v2` /
`mean_90_csv_calendar_analysis_v2`. Neither profile uses random seed or trajectories.
The pinned analysis version is `text_categories_monthly_v1`.

One Predict computes the existing v1 monetary projection from the same original
CSV facts and calendar, then attaches derived labels and monthly evidence. Labels
never participate in residual allocation or establish replacement completeness.
CSV eligibility, unknown days, exact EOD reconciliation, manual provenance,
CURRENT availability and all six limitations remain unchanged. Probability stays
not_calibrated/null; there are no invented bands or measured category accuracy.
Covered/trained jobs retain their previous explicit context and are not analyzed.
Old v1/schema1/schema2 jobs/results retain their original bytes and decoders.

The analysis covers exactly cutoff-89..cutoff, at most20000 input operations.
Pure categorize/fit_monthly helpers keep the existing minimum3 confirmations for
regular outflows and4 otherwise,75% observed-period threshold,4-day date tolerance
and category exclusions. Unknown labels and an empty hypothesis list are explicit;
legacy absence/null means not analyzed. A hypothesis has status=hypothesis and
never creates a payment, reconciliation group, known occurrence or risk probability.
Party identity resolves from scoped input refs (INN, otherwise normalized name),
not from a raw identifier embedded in a pattern ID.

Core validates caller-owned facts exactly, permits only allowlisted derived
category/source/rule fields, preserves manual UUID/source, and validates all scoped
IDs/versions, nonoverlap, monthly anchors/shifts/confirmations/expected amounts and
overflow. The same derived validator runs when reading saved data. ML cannot
assign tenant UUIDs: Core maps only explicit supported semantic codes to the
frozen company catalogue, respecting direction; unmatched semantic codes have no
UUID. Catalogue entries are frozen under the job's account transaction.

Derived data lives in immutable core_forecast_jobs.result.Analysis, separately
from Ledger/Payments; optional Categories is frozen in existing schema2 job input.
No SQL migration or new protobuf field is needed. GET forecast returns the existing
optional result.analysis contract, with category_source manual/rules/unknown,
exact decimal-string versions/amounts and typed hypotheses. PATCH
transactions/{id}/category with category_id UUID or null remains the manual
command. A forecast's is_stale/current_data_version/input.data_version applies to
its entire frozen analysis. Consumers must never attach stale labels to a newer
transaction version or infer labels for operations outside the analysis window.
Reads/restarts do not call ML or modify facts/revision. A rollback must retain
support for newly persisted v2 contexts; stop new producers before rolling back
workers/readers. Old immutable payloads are never rewritten.
