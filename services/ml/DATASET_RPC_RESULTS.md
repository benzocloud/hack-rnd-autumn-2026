# Проверка финансового RPC на компании из датасета

24.10.2026. Локально, без обучения, commit/push/PR и изменений исходных CSV.
Ветка god4k1ta/feat/ml_grpc; main 31e79132f860f468a867f9e0a73a567e2abfacff
включён в HEAD b71765f6d31bc29ebfa5c66d25a6dd6f4d29840a после свежего fetch.
Новых merge/conflicts не требовалось. 35 прежних локальных файлов сохранены с SHA.

## История C089

Заранее известный проблемный пример: C089, срез 26.05.2026, история 26.02–26.05,
прогноз 27.05–25.06. Это диагностика, не новый holdout. Датасет синтетический;
RUB — соглашение адаптера. Клиент отобрал 70 фактов до cutoff, использовал
существующие правила категорий и детектор регулярности только по прошлому
(4 гипотезы по 180 дням); ключи переданы как метаданные. Обучения/подбора не было.

Запрос прошёл через настоящий gRPC к отдельному ML-процессу по loopback. Сервер
не открывал CSV. Нормализатор из фактов восстановил 90 дневных остатков, только
cutoff EOD отмечен OBSERVED. Полнота fixture — явное допущение
dataset_coverage_assumed_not_bank_certified, сохранённое в response.

| Проверка | Результат |
|---|---|
| Все 30 date/inflow/outflow/balance против frozen mean_90 | До копейки совпали |
| Начальный остаток | 227 038,48 денежных единиц |
| Прогнозный остаток на 25.06 | −292 410,02 |
| Первая прогнозная дата минуса | 09.06 |
| Первая фактическая дата в frozen истории | 28.05: опоздание 12 дней сохранилось |
| Повтор RPC | Идентичный response |
| Календарь | UNAVAILABLE, без выдуманной даты знания |
| Вероятность | null; пункт ТЗ открыт |

Это совместимость транспорта/нормализации с baseline и сохранение его ошибок,
не рост качества. MAE/F1/покрытие интервалов календарной сборки здесь не оцениваются.

## Отдельный искусственный календарный сценарий

Гипотетический будущий остаток зарплаты 45 000 ₽ на пятый день после среза —
**не исторически известный платёж**. Источник manual_synthetic_scenario,
limitation synthetic_calendar_scenario_not_historical_fact. Часы replay = 26.05;
сегодняшние обязательства в прошлое не перенесены.

Полная группа явно покрывает 3 прошлые salary_final операции одного контрагента.
Их суммы исключаются из среднего; все остальные факты сохраняются. Один remaining
добавляется один раз. Сценарий: новый snapshot/data_version=2.

| Проверка | Результат |
|---|---|
| Все 30 дней против независимого расчёта residual mean + known | Точно |
| Итог при remaining 45 000 ₽ | 8 953,78 ₽ |
| Remaining 40 000 ₽, новый snapshot/data_version=3, payment.version=2 | 13 953,78 ₽ |
| Дельта с даты платежа | Ровно +5 000 ₽ на каждый следующий дневной остаток |
| Дубликат payment_id | INVALID_ARGUMENT |
| Компоненты/версии/категории/regularity/ссылки | Проверены consumer validator |

Положительный остаток искусственного сценария не доказывает улучшение модели
или настоящую платёжеспособность C089.

## Воспроизведение и передача

Скрипт: [verify_dataset_rpc.py](verify_dataset_rpc.py).
Чистый mapping и consumer checks: [core_input.py](cashflow_rpc/core_input.py).
Подключение worker Core по gRPC: [CORE_CONNECTION.md](CORE_CONNECTION.md).
REST/BFF в этом стыке не участвует; Go worker принадлежит Core.

Проверенный capture — artifacts/core_rpc_C089_20260526_verified/: по request/response
JSON для history-only, calendar-scenario, remaining-updated; summary.json с SHA CSV,
frozen predictions и каждого fixture. Int64 — JSON-строки; UUIDv5/версии тестовые,
production ID/версии задаёт Core. Черновая capture без suffix verified сохранена;
для handoff используются только verified файлы.

Source SHA256 all_operations.csv:
28d7a2ebc6d4d65f31f93e59e24659a04c76990d20833337b9351316fd445472.
Логи — artifacts/core_readiness_20261024/.

В общей исследовательской копии прошли 127 tests, включая 33 RPC/mTLS. Срез
публикации проверен отдельно без неопубликованных исследований: 102 tests,
включая 34 RPC/mTLS без skips после защиты от пропуска движений после cutoff EOD.
Ruff 0.17.1 и Buf 1.75.0 проверяются для этого дерева. Auth/исходные CSV
не меняются, generated/artifacts вне Git.
Go generation/build и работающий worker Core этим прогоном не проверялись.
Подключение worker требует действия владельца Core; transport готов на стороне ML.

## Go Core client → Python ML по mTLS

После нового fetch появилась ветка globalarray/feat/core_ml_client. Её точный SHA
8cc068eb4f3a21c6e7977715012521bd2de9645d извлечён в отдельную игнорируемую копию;
исходники Core не изменялись. Go 1.27.3 проверен по официальному SHA архива.
Запущен существующий пакет Core:

```text
go test -count=1 -tags=mlintegration -v ./internal/client/ml
CORE_ML_TEST_PYTHON=<изолированное Python окружение с pinned RPC dependencies>
CORE_ML_TEST_SOURCE=<services/ml публикуемого среза>
```

Все 10 верхнеуровневых tests прошли, включая настоящий TestPythonPredictMTLS
без skip: Go client → отдельный Python server → 30 точек, closing −2700 копеек,
first-negative 27.10.2026 на синтетическом inputFixture Core. TLS проверяет CA,
SAN saldo-core/saldo-ml; также проверен ответ с допустимым расхождением 1 копейки
в наблюдённой истории. Client tests дополнительно проверили плохие inputs/outputs,
версии, overflow, deadline/cancellation и mapping статусов.

Это старый ml.v1.ForecastService/Predict, его совместимость сохранена.
Для PredictFinancial потребителю ещё нужны новые local bindings и mapping.
Полный task check Core, PostgreSQL, race detector, worker queue/storage и REST
этим прогоном не проверялись. Лог в локальном integration_chain_20261024/core-client-tests.log.
