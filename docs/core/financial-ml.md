# Единый ML Predict и финансовая семантика

24.10.2026, ADR-029. Канонический RPC: `ml.v1.ForecastService/Predict` в
`proto/ml/v1/forecast.proto`. Отдельного финансового endpoint нет. API вызывает
Core ForecastingService, не ML; Core выбирает модель из серверной конфигурации,
фиксирует model/preprocessing/configuration/seed и не изменяет их после enqueue.
Пустой календарь не разрешает автоматический выбор исторического baseline.

## Реализация и целевой контракт

Работает только mean_90 / mean_90_v1 / daily_snapshot_v1: 90 подтверждённых дней,
1–30 дней горизонта, history-only, без финансового календаря и вероятности.
Он не реализует продуктовый финансовый прогноз. Переданный финансовый контекст
(facts, groups, purpose/calendar, configuration, seed/trajectory settings или
coverage warnings) отклоняется с FailedPrecondition, а не игнорируется. Неизвестная
версия модели также даёт FailedPrecondition. Новые финансовые поля описывают
**целевую** реализацию той же границы Predict, не готовность модели.

Продуктовый job требует финансовую модель; если она не реализована/недоступна,
Core не вызывает mean_90 вместо неё. Ошибка задачи должна объяснять недоступность.
Baseline сохраняется внутри ML для тестов/оценки; существующий Go ML adapter
проверяет этот ограниченный профиль. Его успешный smoke не закрывает P0.
На данном этапе нет подтверждённого production-потребителя старого KnownPayment;
до общего merge его зарезервированный DTO заменён нормализованным календарём.

## Вход PredictRequest

- Snapshot/company/account — ненулевые canonical lowercase UUID, data_version
  положительный и <=MaxInt64 для совместимости Core. Все amounts signed int64
  копейки, currency=RUB. Потоки/обязательства положительные, EOD может быть <0.
- `history_start..cutoff_date`: непрерывный список `history`, по одному дню;
  модель определяет необходимую длину. DaySource OBSERVED означает принятый
  фактический EOD; VERIFIED_NO_OPERATIONS — подтверждённое отсутствие операций.
  PARTIAL/UNAVAILABLE не превращаются в нулевые потоки. Optional amounts
  отсутствуют там, где неизвестны. Модель обязана явно отказать либо выдать
  честно ограниченный результат, если поддерживает такую обработку.
- `initial_balance_minor` присутствует, `balance_as_of=cutoff_date`, последний
  EOD подтверждён. Для baseline эти новые metadata поля не передаются; его
  существующие invariants сохранены. Запрещено переносить старый остаток через
  неизвестные дни и приписывать реконструированным остаткам observed provenance.
- `facts`: уникальные Core transaction UUID/version, дата, направление, сумма,
  category UUID/semantic_kind, source. Facts ограничены историей/cutoff; даты
  после cutoff запрещены. Их агрегаты должны совпадать с известными inflow/outflow
  дневного покрытия. При полноте фактов повторяющийся EOD не суммируется.
- `known_payments`: только planned/partially_matched, оставшаяся сумма >0,
  дата в горизонте, уникальные payment UUID/version, category/source. Matched/
  cancelled и полностью распределённые суммы исключаются. Просроченные
  обязательства не переносятся на cutoff+1 молча: их дата/правило переноса
  требуют явного решения Core, иначе ошибка/limitation.
- `purpose` различает CURRENT_FORECAST и HISTORICAL_EVALUATION.
  VERIFIED_CURRENT допускается только для текущего прогноза. Исторический тест
  принимает только VERIFIED_AS_OF_CUTOFF; нынешние обязательства нельзя подмешать
  в прошлый срез. UNAVAILABLE не означает пустой подтверждённый календарь.
- `captured_at` — реальное UTC-время фиксации согласованного снимка. Snapshot
  immutable; Core хранит его и достаточные provenance для воспроизведения.
- `horizon_days`: продуктовый REST 30–90; точное поддерживаемое значение
  проверяется выбранной моделью. Нельзя обрезать 90 до 30 или выбрать другую
  модель. `configuration_id`, decimal seed и trajectory_count фиксируются
  сервером, относятся к конкретной модели; неизвестные настройки отклоняются.

## Устранение двойного учёта

Core передаёт явные reconciliation_groups для одного экономического потока:

1. Historical transaction IDs уникальны, принадлежат снимку, встречаются не более
   чем в одной группе. Помеченные recurrence_group_id соответствуют этой группе.
2. Все факты группы исключаются из обучаемого unknown baseline. Будущая часть
   группы представлена полным списком future_occurrences на весь горизонт.
   `full_horizon_verified=true` обязателен. Пустой список допустим только с
   проверенным no_future_occurrences_reason, не как отсутствие данных.
3. Каждое occurrence добавляется один раз. Если указан payment_id, он должен
   принадлежать known_payments, иметь ту же дату/направление/remaining_minor и
   ту же группу. В текущем контракте один payment не дробится между occurrences.
4. Негрупповой known payment требует `additional=true`, добавляется один раз
   сверх unknown baseline и означает явно новое обязательство. У grouped
   `additional=false`. Эти варианты взаимоисключающие; сходство даты/суммы не
   выбирает вариант автоматически.
5. Известная часть группы плюс её исторический средний расход не складываются.
   Unknown baseline вычисляется только по оставшимся историческим движениям;
   его суммы и все явные occurrences вместе формируют итоговые дневные потоки.

Не найденная/неполная группа, неизвестный payment/fact, дубликаты или несовпадение
экономики — InvalidArgument либо FailedPrecondition при отсутствии поддержки,
без частичного результата. Этот механизм пока контракт, не реализованный inference.

## Выход и обязанности Core

Echo snapshot/company/account/data_version/cutoff/model совпадает **полностью**,
включая configuration/seed/trajectory_count. `days` строго cutoff+1..+horizon,
без пропусков/дубликатов, потоки неотрицательны, reference_balance вычисляется от
initial_balance по целочисленной формуле без переполнений. Core перепроверяет
метаданные, денежную арифметику, count/dates и first_negative_balance_date.
Неправильный ответ не сохраняется как succeeded.

`components` объясняют сумму дневных потоков: уникальные component/occurrence IDs,
source facts и known payment references из снимка. Нельзя выдавать непрозрачный
unknown baseline за обнаруженные продажи/регулярность. UNCLASSIFIED_BASELINE
не становится scenario_eligible. При проекции в REST он остаётся частью daily
points и явно объяснённым допущением; REST components содержит только доступные
в его enum объяснимые компоненты. Отсутствие объяснимого компонента выключает
соответствующую scenario capability, не выдумывает ID для действий пользователя.

`history_only_includes_recurring_flows=true` маркирует ограниченный baseline.
Финансовая модель должна возвращать false и подтверждённую группировку; Core не
принимает true как полноценный финансовый результат. Warnings/assumptions
сохраняются и доходят до пользователя, включая неполное покрытие/календарь.

Probability — oneof: calibrated estimate (finite [0,1] + calibration_version +
evaluation_reference) либо unavailable_reason. Допустимые причины: not_estimated,
not_calibrated, insufficient_history, incomplete_coverage. Ноль — только реальная
оценка, confidence правила не вероятность. Core переводит в REST Probability
{value, unavailable_reason} с явным null противоположной ветки. Для baseline
вероятность всегда not_estimated. Интервалы обеими границами либо отсутствуют,
finite integral lower<=upper; отсутствие интервала не подменяется balance.

Сценарии применяет Core к неизменяемому базовому результату и его occurrences.
Если вероятностная оценка после преобразования не пересчитана обоснованно,
возвращать unavailable_reason, а не копировать вероятность исходной базы.
Сравнение использует один snapshot/model/seed и те же условия. Stress + response
наследует весь stress; суммы/occurrence IDs повторно не учитываются.

## Транспорт и ошибки

В реализации baseline: mTLS с разрешённым DNS SAN Core, deadline <=30 секунд,
2 MiB, 2 потока/4 concurrent RPC; негативные mTLS-тесты проверяют отказ чужому
peer. Эти параметры не объявляют production PKI настроенной. Расширенные снимки
должны укладываться в явно настроенный бюджет сообщений; oversized даёт
ResourceExhausted, без усечения facts/history. API не имеет прямого ML-доступа.

InvalidArgument — неверные суммы/даты/coverage/дубликаты/ссылки/overflow;
FailedPrecondition — модель/параметры/финансовая семантика не поддержаны;
Unavailable — зависимость недоступна; ResourceExhausted — лимит; Canceled /
DeadlineExceeded — прерывание. Автоматический retry другой моделью запрещён.
