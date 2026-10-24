# Core → ML: первый gRPC endpoint

Статус: реализация этапа 1, на ревью разработчика; подготовлена для публикации
по прямому поручению владельца. Деплой и подключение Core не выполнены.
Canonical contract: [forecast.proto](../../proto/ml/v1/forecast.proto).
План и границы следующего этапа: [GRPC_PLAN.md](GRPC_PLAN.md).

Этот документ описывает прежний history-only Predict. Для worker Core с явным
календарём доступен отдельный PredictFinancial:
[контракт и подготовка worker](CORE_CONNECTION.md). Его добавление не меняет
численный результат или ограничения старого Predict.

## Что работает

`ml.v1.ForecastService/Predict`: нормализованный immutable snapshot Core →
фиксированный mean_90 → дневные inflow/outflow/reference balances.
Сетевой обработчик использует только inference.py и исходный baseline predict();
CLI evaluation, dataset, будущие факты, CatBoost/ETS и обучение не вызываются.

Core ещё не подключён. ML не формирует снимок из БД и не сохраняет прогноз.
Наличие этого endpoint не завершает цепочку Frontend → API → Core → ML.

## Семантика запроса

| Поле | Правило |
|---|---|
| snapshot_id, company_id, account_id | Ненулевые canonical lowercase UUID; не локальные C001 |
| data_version | Положительная версия согласованного снимка Core |
| currency | Только RUB; все суммы int64 в копейках |
| cutoff_date | ISO YYYY-MM-DD, локальная бизнес-дата компании |
| initial_balance_minor | Обязательно присутствует, включая допустимый ноль; принятый наблюдённый EOD именно cutoff |
| history | Ровно 90 последовательных дней cutoff−89 … cutoff, без сортировки/дозаполнения сервером |
| history amounts | inflow/outflow/closing_balance явно присутствуют; потоки неотрицательны |
| history source | OBSERVED либо VERIFIED_NO_OPERATIONS; unspecified/unknown отклоняется |
| known_payments | Только пустой список на этом этапе |
| horizon_days | 1..30; начало всегда cutoff+1 |
| model_name | mean_90 |
| model_version | mean_90_v1 |
| preprocessing_version | daily_snapshot_v1 |

Версии — явный запрос, а не пожелание «использовать latest». Изменение численного
поведения baseline или подготовки требует новой версии; незаметная подмена запрещена.
Идентификаторы/версии возвращаются неизменными, но ML не может проверить их
истинность в БД: за snapshot consistency и доступ отвечает Core.

OBSERVED — день с принятым фактическим EOD (в том числе одобренное Core ручное
наблюдение с сохранённым происхождением). VERIFIED_NO_OPERATIONS означает
подтверждённое отсутствие операций, потоки строго нулевые, остаток не меняется.
Core предварительно исключает неизвестное покрытие, нерешённые дубли и gaps.
По агрегатам ML не способен повторно найти дубли отдельных операций.
Последний день обязательно OBSERVED, его остаток равен initial_balance_minor.

Со второго дня независимо проверяется balance(t)=balance(t−1)+in−out.
Только для OBSERVED допускается исходное расхождение не более одной копейки
с предупреждением OBSERVED_RECONCILIATION_WITHIN_ONE_MINOR_UNIT. Неизвестный
первый opening balance не выдумывается; первый день служит начальной точкой
проверки внутренних наблюдений. Прогнозная арифметика всегда точная без допуска.
Если результат не помещается в int64, запрос отклоняется целиком.

## Что означает ответ

Средние inflow/outflow считаются по 90 дням отдельно, округляются в целых
копейках исходной rounded_mean(); остаток накапливается от initial_balance_minor.
Даты и потоки имеют одинаковую длину horizon_days. Это одна детерминированная
траектория; одинаковые запросы дают одинаковые сериализованные значения.

reference_balance_minor и first_negative_balance_date — математическая
диагностика этой линии. Core проверяет даты, версии, полноту, потоки и заново
считает остаток по формуле, затем владеет финансовым результатом и его хранением.
Отрицательный EOD не доказывает невозможность платежа; лимиты овердрафта неизвестны.
Поле вероятности отсутствует, warning UNCALIBRATED_NO_PROBABILITY обязателен.

Ответ явно помечен history_only_includes_recurring_flows=true и предупреждением
HISTORY_ONLY_NO_CALENDAR. В средних уже есть прошлые регулярные платежи.
Нельзя добавлять к этому прогнозу будущую зарплату/налоги без согласованной
замены соответствующего модельного компонента. Непустой known_payments поэтому
возвращает FAILED_PRECONDITION; пока это не прогноз с полноценным календарём.

## Транспорт и ошибки

- В рабочем режиме mTLS: сервер проверяет client CA и разрешённый SAN Core
  (`saldo-core` по умолчанию, `--allowed-peer` для явной настройки PKI).
  Само поле company_id не авторизует доступ; вызов API/браузера к ML не предусмотрен.
- Core проверяет server certificate/SAN стандартным TLS-клиентом. Provisioning,
  ротация сертификатов и деплой не выполнялись. Локальные ephemeral certs — тестовые.
- Клиент обязан установить deadline <=30s; будущий Core client учитывает общий
  бюджет операции. Сервис проверяет deadline/cancellation до и после вычисления.
- Два worker threads, максимум четыре одновременных RPC, max receive/send 2 MiB.
- Логи не выводят запрос, операции, ключи или сертификаты.
- Отдельный --insecure-local разрешён только на 127.0.0.1 для smoke;
  внешний bind и смешивание этого режима с TLS material отклоняются.

| Статус | Причина |
|---|---|
| INVALID_ARGUMENT | Неоднозначные/неполные данные, даты/покрытие/валюта/UUID, неверный deadline, int64 overflow |
| FAILED_PRECONDITION | Недоступная версия модели/подготовки или непустой будущий календарь |
| PERMISSION_DENIED | Сертификат доверен CA, но SAN не принадлежит разрешённому Core |
| UNAVAILABLE | TLS handshake не прошёл, сервер недоступен; не превращать в успешный forecast |
| RESOURCE_EXHAUSTED | Превышен лимит сообщения или конкурентных RPC |
| DEADLINE_EXCEEDED / CANCELLED | Deadline или отмена; успешный частичный прогноз не выдаётся |

Первая дата минуса присутствует только при отрицательном reference balance
хотя бы в одном прогнозном дне. Начально отрицательный остаток допустим;
это поле само по себе не различает новый и продолжающийся дефицит — Core
учитывает initial_balance и собственные правила риска.

## Воспроизводимая генерация и запуск

Python 3.12, grpcio/grpcio-tools 1.78.0, protobuf 6.33.5, Buf 1.75.0.
RPC-зависимости отделены от baseline/экспериментов. Buf binary скачать из
официального release с проверкой sha256.txt; не добавлять бинарник в Git.

```powershell
.venv/Scripts/python.exe -m pip install -r services/ml/requirements-rpc-dev.txt
.venv/Scripts/python.exe services/ml/generate_rpc.py --buf path/to/buf.exe
```

Генератор проверяет Buf version, строит descriptor set только canonical ML
контракта через Buf build и передаёт его закреплённому grpc_tools.protoc.
Python bindings оказываются в services/ml/ml/v1/, игнорируются Git,
не редактируются вручную. Существующая Auth generation не изменена; при её
вызове ограничивать Auth path, не генерировать ML во все Auth consumers.
Go bindings должны генерироваться в services/core/internal/gen с явным
Mml/v1/forecast.proto mapping на этапе подключения Core; здесь их нет.

Терминал 1 (без dataset):

```powershell
.venv/Scripts/python.exe -B services/ml/run_server.py --insecure-local
```

Терминал 2 — synthetic client с нулевым стартовым остатком:

```powershell
.venv/Scripts/python.exe -B services/ml/smoke_rpc.py
```

Ожидается 30 дней, итог −60 копеек, first_negative_balance_date=2026-10-24.
При занятости порта указать одинаковый --port в обеих командах.

Рабочий mTLS запуск с выданными владельцем PKI файлами:

```powershell
.venv/Scripts/python.exe -B services/ml/run_server.py --host 0.0.0.0 --certificate server.pem --private-key server-key.pem --client-ca core-ca.pem --allowed-peer saldo-core
```

Этот пример не выдаёт сертификаты и не разворачивает сервис.

## Проверки этапа 1

- 107 ML-тестов прошли без skips с установленными RPC/experiment dependencies.
  Из них 13 новых RPC-тестов с отрицательными subcases.
- Реальный отдельный процесс run_server + smoke_rpc дал ожидаемые 30 точек.
- Проверено совпадение с исходным predict() на горизонтах 1/7/30 и неодинаковых
  потоках; точное округление, нулевой EOD, int64 overflow, разрыв покрытия,
  даты, источник нулевого дня, календарь и версии.
- Реальные mTLS-тесты: Core успешен; чужой SAN и CN без SAN получают отказ;
  недоверенный CA и отсутствие client cert не проходят handshake.
- В полной локальной копии со старыми неопубликованными исследованиями:
  107 обнаруженных тестов, в stdlib 86 выполнены и 21 skipped (13 RPC + 8 experiment).
  В публикуемый RPC-скоуп не входят прежние experiment-тесты/код.
- Отдельный workflow ML RPC checks устанавливает закреплённые зависимости,
  проверяет protobuf/генерацию и запускает check_rpc_tests.py: все 13 RPC-тестов
  обязаны выполниться без skips. Обычный baseline CI этого не подтверждает.
- Новые библиотеки устанавливаются из requirements-rpc-dev; pip check прошёл.
  Ruff 0.17.1 check и format --check прошли. Buf lint/build/breaking against
  origin/main прошли; existing Auth contract не менялся.

Генерацию и финальный состав diff фиксируют локальные артефакты проверки.
GitHub CI, реальный Core client, snapshot consistency в БД, PKI deployment,
доступ из других машин и production E2E ещё не проверены.

Использованные первичные руководства: [gRPC Python](https://grpc.io/docs/languages/python/quickstart/),
[protobuf field presence](https://protobuf.dev/programming-guides/field_presence/),
[Buf](https://buf.build/docs/).
