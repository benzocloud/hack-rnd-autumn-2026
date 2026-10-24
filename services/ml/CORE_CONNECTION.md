# Worker Go Core → Python ML

Рабочий стык: worker внутри Go Core вызывает ML по gRPC и получает результат.
REST/BFF не участвует в этом стыке. ML не создаёт API handlers, Core worker,
очередь, авторизацию пользователя или SQL storage. Их реализацией владеет Core.
Auth schema не меняется; проект proto/core/v1 теперь входит в общую поставку PR #29. Согласование worker проверяется отдельно от готовности всего продукта.

## Минимальный вызов

- RPC: ml.v1.ForecastService/Predict.
- ModelContext: mean_90 / mean_90_components_v1 / calendar_replacement_v1.
- Вход: immutable snapshot и 90 дней покрытых фактов от worker, не путь к CSV.
- Выход: ровно 30 дневных точек, компоненты и ссылки, эмпирические диапазоны;
  вероятность разрыва отсутствует с явной причиной.
- Deadline: рекомендуемый первый бюджет 3s, сервер требует <=30s.
- Рабочий transport: mTLS, client SAN saldo-core, проверка server CA/SAN у Core.
  Один переиспользуемый connection; insecure только 127.0.0.1 для local checks.
- Лимиты: 2 MiB в каждую сторону, 4 concurrent RPC / 2 workers; до 20 000 фактов,
  1 000 групп/платежей. Нельзя усекать полный снимок ради прохождения лимита.

Worker генерирует только ml/v1/forecast.proto в свой
services/core/internal/gen. Для обоих go и go-grpc plugins применить mappings:

| Proto | Import / package |
|---|---|
| ml/v1/forecast.proto | github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/ml/v1;mlv1 |

Вызов generated NewForecastServiceClient(conn).Predict(ctx, request).
Плагины/версии закрепляет потребитель. Generated файлы вне Git. Canonical JSON
fixtures можно читать через protojson.Unmarshal; суммы int64 — строки, не float64.
Generated Go bindings входят в общий PR #29; финансовый Go adapter ещё не реализован. Существующий
Core client прежнего Predict проверен отдельным реальным mTLS integration test
на ветке Core 8cc068e; это не проверка очереди/сохранения результата worker.

## Маппинг снимка

| Core FinancialSnapshot.Data() | ML |
|---|---|
| ID, Scope, DataVersion | snapshot/company/account UUID, положительная data_version |
| Cutoff / Opening.Observation.Amount | бизнес-дата и принятый EOD именно cutoff, int64 копейки RUB |
| Facts | только cutoff−89 … cutoff, исходные UUID/entity versions, одно положительное направление |
| Category UUID/company | category_id/category_company_id, сохранить scope |
| Каталог / classifier до вызова | category_code/category_source; допустим явный unknown |
| Результат детектора регулярности | regularity_key как метаданные с сохранённым происхождением |
| ИНН, имя, назначение, Source | строки counterparty_*, payment_purpose, factual_source_kind, source_namespace, external_operation_id |
| Limitations() | availability_limitations без удаления исторических ограничений |

Reference без CSV/БД: [core_input.py](cashflow_rpc/core_input.py),
prepare_financial_request. Входные UUID/версии настоящего worker принадлежат Core;
UUIDv5/начальные версии dataset fixtures — только тестовые.

Нужны доказательство покрытия всех 90 дней и сохраняемая ссылка на него.
Отсутствие строк его не доказывает. Helper требует Coverage(start,end,reference),
проверяет форму, но правдивость подтверждает импорт/worker. При неизвестном
покрытии нельзя строить фиктивные нулевые дни. В dataset-проверке полнота — явное
допущение dataset_coverage_assumed_not_bank_certified, а не сертификат импортера.

Суммировать факты по дням целыми копейками. От cutoff EOD восстановить назад:
closing(t−1)=closing(t)−inflow(t)+outflow(t). Только cutoff маркировать OBSERVED;
остальные восстановленные дни — RECONSTRUCTED_COVERED. Самостоятельные наблюдения,
если они есть, дополнительно сверить; дубли/gaps не удалять молча. Переполнение — ошибка.

## Календарь и один учёт

Если Calendar Core сообщает историческую недоступность, использовать UNAVAILABLE:
snapshot_id=ID фактов, data_version та же, groups/payments/as_of_business_date пустые,
availability_limitations содержит historical_calendar_unavailable. Это fallback
mean_90, а не заявление об отсутствии обязательств. Дата знания не выдумывается.

CURRENT требует реальный снимок календаря той же финансовой data_version,
даты знания cutoff либо cutoff+1, актуальные remaining/status/version. Worker
отсекает отменённые/оплаченные и платежи вне 30 дней, явно задаёт effective_date,
сохраняет original_due_date. ML не переносит просроченный платёж автоматически.

Для даты знания cutoff+1 дополнительно обязателен no_post_cutoff_movements=true:
Core подтверждает отсутствие движений после cutoff EOD в том же покрытом view.
Иначе уже оплаченный сегодня счёт исчезнет из remaining, а его списание ещё не
войдёт во вчерашний EOD — остаток будет завышен. Такой вход отклоняется; нужен
новый принятый cutoff или отдельно согласованный учёт внутридневных движений.

Полная CalendarGroup содержит точные historical_operation_ids. Эти суммы
исключаются из среднего, будущие remaining добавляются один раз. Новый платёж
без аналогов требует additional_obligation_confirmed. Пустой полный график группы
допустим для отмены/оплаты. Истинность связей и один счёт под одним ID обеспечивает
Core. Нельзя скрывать известный несогласованный платёж через UNAVAILABLE.
Изменение платежа требует нового immutable snapshot/data_version и calendar
snapshot; payment_id сохраняется, payment.version повышается.

HISTORICAL_AS_OF_CERTIFIED допустим только с настоящим журналом доступности;
сегодняшние remaining нельзя перенести в прошлое, изменив дату. Пример зарплаты
в fixture — отдельный искусственный replay, не исторический календарь.

## Проверка и сохранение ответа

core_input.verify_response — reference для worker: scope/snapshot/data/model/calendar
versions, 30 дат/start_date, точная формула баланса, компоненты, каждый remaining
один раз, ссылки истории и первая дата минуса. Classified facts/limitations сохраняются.
Worker дополнительно считает глубину дефицита и сохраняет ответ вместе с immutable
input/model context. Сетевой вызов вне SQL-транзакции; проверка stale принадлежит Core.

INVALID_ARGUMENT — плохой снимок/группы; FAILED_PRECONDITION — несовместимый context;
PERMISSION_DENIED/TLS failure — service identity. UNAVAILABLE/DEADLINE_EXCEEDED/
RESOURCE_EXHAUSTED — ограниченный retry того же immutable input в бюджете job.
Ошибки/неверный response не дают успешный прогноз с нулевыми точками.
calibrated=false, probability=null с причиной; диапазон не превращать в процент риска.

Проверка отдельного процесса и fixtures: [DATASET_RPC_RESULTS.md](DATASET_RPC_RESULTS.md).
Наличие Python client или reference mapping не означает, что worker Core уже подключён.

В общей поставке PR #29 ответ финансовой версии развёрнут в один PredictResponse,
без response.forecast wrapper. financial_pb2 и второй gRPC service удалены;
все bindings генерируются из forecast.proto. Правила: docs/core/financial-ml.md.
