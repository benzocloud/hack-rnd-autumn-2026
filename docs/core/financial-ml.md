# Единый Predict: финансовая модель и baseline

24.10.2026, ADR-029. Единственный ML RPC — `ml.v1.ForecastService/Predict`,
канонический `proto/ml/v1/forecast.proto`. Сервер выбирает реализацию только по
явно указанным model_name/model_version/preprocessing_version. Не по наличию
календаря, не после ошибки, без автоматического fallback.

## Доступные версии

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
