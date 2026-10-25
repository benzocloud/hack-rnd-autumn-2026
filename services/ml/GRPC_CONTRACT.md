# Core → ML: единый ForecastService/Predict

Актуальный контракт общей поставки PR #29, 24.10.2026, ADR-029.
Canonical: [forecast.proto](../../proto/ml/v1/forecast.proto).
Финансовая реализация ML PR #27 `49a7862` объединена с Go-клиентом и полным
Core/API контрактом. Два исходных RPC сведены к одному, без baseline fallback.

| Профиль | model_name | model_version | preprocessing_version |
| --- | --- | --- | --- |
| Продуктовый финансовый | mean_90 | mean_90_components_v1 | calendar_replacement_v1 |
| Явный history-only baseline | mean_90 | mean_90_v1 | daily_snapshot_v1 |
| Экспериментальный обучаемый, Core пока блокирует | linear_pool_90 | linear_pool_components_v1 | calendar_replacement_v1 |

Эксперимент требует configuration_id с SHA закреплённого артефакта и явные
seed=20261024/trajectory_count=512; старые профили сохраняют прежнюю семантику.
Обученные веса совпали с mean_90, улучшения нет; default не переключён.
[Артефакт, оценка, ограничения подключения](TRAINED_MODEL_RESULTS.md).

Оба вызываются через PredictRequest/PredictResponse. Финансовая версия получает
operations, calendar и availability_limitations. Ответ плоский: days/components/
uncertainty/calendar identity/provenance. Отдельного endpoint или wrapper нет.
Неподдерживаемая версия/конфигурация → FailedPrecondition, без повторного вызова
baseline. Пустой календарь не переключает модель. Версию фиксирует Core, не API.

Полная нормативная семантика, coverage, group replacement, ограничения модели,
ошибки и маппинг в REST: [финансовый контракт](../../docs/core/financial-ml.md).
Reference mapping Core facts → запрос и независимая consumer validation:
[CORE_CONNECTION.md](CORE_CONNECTION.md), `cashflow_rpc/core_input.py`.
Старые dataset-проверки: [DATASET_RPC_RESULTS.md](DATASET_RPC_RESULTS.md),
исторический план: [GRPC_PLAN.md](GRPC_PLAN.md).

## Что реализовано

Python gRPC server, mTLS/allowlist Core, deadlines и лимиты, 90 дней exact money,
30-дневный финансовый mean_90 с явным календарём/компонентами/происхождением,
эмпирические bands. Неоткалиброванная вероятность недоступна с причиной.
Клиент/сервер не читают dataset/CSV/SQL при inference; immutable input даёт Core.

Go-клиент проверяет пока только ограниченный baseline-профиль, включая настоящий
mTLS-вызов Python. Финансовый Go adapter, jobs/SQL persistence, HTTP handlers и
полный browser E2E ещё предстоят. Транспортный smoke не доказывает завершение P0.

## Проверки и запуск

```sh
python -m pip install -r services/ml/requirements-rpc-dev.txt
python services/ml/generate_rpc.py --buf /path/to/buf-1.75.0
python -B services/ml/check_rpc_tests.py
python -B services/ml/run_server.py --insecure-local
# Во втором терминале:
python -B services/ml/smoke_rpc.py
python -B services/ml/smoke_financial_rpc.py
```

Insecure-local только для synthetic smoke на 127.0.0.1. Рабочий режим требует
certificate/private-key/client-ca/allowed-peer, без смешивания с insecure-local.
2 workers, 4 concurrent RPC, 2 MiB, caller deadline <=30 s. SAN default saldo-core;
production provisioning/rotation/deployment не выполнялись. Неверный SAN при
доверенном CA отклоняется; CN-only/чужой CA/нет client cert не дают доступ.
