# ML на демо 25.10.2026: запуск, проверка, восстановление

Повторно проверено 24.10.2026 на main `ea433ddd222aa7e279245b6bcd164c1687da7db3`.
Ветка `god4k1ta/docs/ml_demo_runbook` синхронизирована через `git merge origin/main`;
конфликтов нет. Код ML-сервера и канонические контракты не менялись.

Статус локального этапа: **на ревью владельца**, без commit/push/PR/deployment.

> **Граница проверки.** Проверен только путь **клиент в форме Core → ML**
> по mTLS. Настоящие Core, API, frontend и развёрнутый стенд в этой проверке
> не участвовали. Полный путь «экран → API → Core → ML» должны подтвердить
> владельцы Core/API на стенде. Их собственный отчёт —
> `.agent/plans/financial-demo-integration-verification.md`; я его не повторял.

## Что важно знать до демо

- **Будущая выплата через Core сейчас до ML не доходит.** В main
  `services/core/internal/client/ml/financial.go` всегда шлёт календарь
  `UNAVAILABLE` и отклоняет запрос с `KnownPayments`. Проверка ответа в Core
  тоже ждёт `HISTORY_ONLY_NO_CALENDAR` и пустой список заменённых операций.
  Пересчёт с выплатой работает в ML (проверено ниже), но на экране его не будет,
  пока Core/API не передают календарь. Решают владельцы Core/API.
- ML не хранит состояния: перезапуск безопасен, сохранённые в Core прогнозы
  не меняются.
- Вероятности разрыва нет: `not_calibrated`, причина
  `REQUIREMENT_OPEN_NO_CALIBRATED_GAP_PROBABILITY`. На экране — «недоступно»,
  а не 0%.
- Подпись диапазонов: **«чувствительность сценария, не вероятность»**.
  Это эмпирические сценарии, не гарантия покрытия всего 30-дневного пути.
- Передача владельцам Core/API: [календарь и приёмочный сценарий](DEMO_CALENDAR_HANDOFF.md).
  Текст подготовлен локально, никому автоматически не отправлялся.

## Ожидаемый результат на демо-компании стенда

Компания из `deploy/stack/seed-demo.py`: 90 дней с приходом 1 000 ₽ и расходом
4 000 ₽ ежедневно, остаток на cutoff 50 000 ₽. Данные синтетические.

| Сценарий | Первый отрицательный день | Остаток на 30-й день |
|---|---|---|
| Без выплаты (как шлёт Core) | cutoff + 17 | −40 000 ₽ |
| + ручная выплата 10 000 ₽ на cutoff + 5 (только ML) | cutoff + 14 | −50 000 ₽ |
| + из неё 3 000 ₽ уже оплачено на cutoff; remaining 7 000 ₽ (только ML) | cutoff + 14 | −50 000 ₽ |

Числа не зависят от даты cutoff: проверены 30.09 и 23.10.

Частичная оплата показана отдельным синтетическим снимком с accepted EOD
47 000 ₽ вместо 50 000 ₽. До даты выплаты его путь ниже на 3 000 ₽, а начиная
с выплаты совпадает с вариантом «10 000 ₽ ещё не оплачено». Обязательство
сохраняет общую сумму: **3 000 оплачено + 7 000 remaining = 10 000 ₽**.
Ранее скрипт ошибочно сохранял remaining 10 000 ₽ и ожидал −53 000 ₽;
это исправлено и покрыто независимым RPC-регрессионным тестом.
Оплата после cutoff требует нового accepted EOD/cutoff, а не ложного
`no_post_cutoff_movements=true` для старого снимка.

## Где работает ML на стенде

Описание стенда — `deploy/stack/README.md`, им управляет его владелец.
- Сервис `ml` в `/opt/saldo-stack/compose.yaml`, образ `ML_IMAGE`.
- Сертификаты лежат в `./pki/ml`. Пускается только `spiffe://saldo.local/core`.
- Core подключается к `CORE_ML_ADDRESS=ml:50051`, имя TLS — `CORE_ML_SERVER_NAME=ml`.
- Порты gRPC наружу не публикуются.

## Проверка перед показом (оператор стенда)

```sh
cd /opt/saldo-stack
docker compose ps
python3 smoke.py
docker compose logs --tail=50 ml
```

`smoke.py` логинится, создаёт прогноз и ждёт 30 точек: это проверка API → Core → ML
и сохранения результата для прогноза **без календаря**. Скрипт не управляет
браузером и не добавляет/сопоставляет платежи; полный пользовательский сценарий
нужно проверить отдельно. В этом локальном прогоне стенд не вызывался.

## Восстановление при сбое

| Симптом | Вероятная причина | Действие |
|---|---|---|
| Прогноз в статусе failed, в логах ML `INVALID_ARGUMENT` | Вход от Core: не 90 дней, нет покрытия, нет EOD на cutoff | Данные Core, ML не трогать; к владельцу Core |
| `PERMISSION_DENIED` | Сертификат клиента не `spiffe://saldo.local/core` | PKI стенда, к оператору |
| `UNAVAILABLE`, таймауты, контейнер `ml` не `running` | Процесс упал или превышен лимит памяти 192m | `docker compose restart ml`, затем `python3 smoke.py` |
| После релиза ошибка не уходит | Новый образ | Откат по `releases/<revision>-<time>/` из `deploy/stack/README.md` (оператор) |
| Live-демо недоступно | Сеть или стенд | Показать заранее сохранённый скриншот успешного прогноза, подписав его |

Не делать: `compose down -v`, prune, ручную правку БД, смену модели или флагов
ради «красивого» результата.

## Локальная репетиция (ML отдельно)

Команды для PowerShell из корня рабочей копии main. Привязки генерируются
так же, как в `deploy/stack/Dockerfile.ml`:

```powershell
$mlPython = 'D:\hack-rnd-2026\hack-rnd-autumn-2026\.venv\Scripts\python.exe'
$mlMainSha = git rev-parse origin/main
& $mlPython -m grpc_tools.protoc -Iproto --python_out=services/ml --grpc_python_out=services/ml ml/v1/forecast.proto
& $mlPython -B services/ml/check_rpc_tests.py
```

Сервер с mTLS. Сертификаты выпускаются по схеме `deploy/stack/provision.py`:
DNS-имя + URI `spiffe://saldo.local/<service>`, назначение serverAuth+clientAuth.
Для локальной репетиции используются отдельные одноразовые сертификаты,
не приватные ключи стенда. Пути ниже нужно заменить на их фактические пути;
сертификат ML должен содержать DNS SAN `ml`. Не отключать TLS ради обхода ошибки.

```powershell
$mlPki = 'D:\path\to\one-time-pki' # заменить на каталог одноразовой PKI
& $mlPython -B services/ml/run_server.py --port 50061 --certificate "$mlPki/ml.pem" --private-key "$mlPki/ml.key" --client-ca "$mlPki/ca.pem" --allowed-peer spiffe://saldo.local/core
```

Во втором терминале:

```powershell
$mlPython = 'D:\hack-rnd-2026\hack-rnd-autumn-2026\.venv\Scripts\python.exe'
$mlMainSha = git rev-parse origin/main
$mlPki = 'D:\path\to\one-time-pki' # тот же каталог PKI
& $mlPython -B services/ml/verify_demo_company_rpc.py --target 127.0.0.1:50061 --server-name ml --ca "$mlPki/ca.pem" --certificate "$mlPki/core.pem" --private-key "$mlPki/core.key" --cutoff 2026-09-30 --output-dir services/ml/artifacts/demo_rehearsal_new --main-sha $mlMainSha
```

Каталог `--output-dir` должен быть новым; старые результаты не перезаписываются.
В обоих терминалах рабочий каталог — корень этой рабочей копии. В обычной
копии можно использовать её `.venv`; в worktree виртуальное окружение не копируется.

Только для изолированной проверки арифметики, не для репетиции демо mTLS:

```powershell
# Первый терминал
& $mlPython -B services/ml/run_server.py --insecure-local
# Второй терминал: определить $mlPython и $mlMainSha, как выше
& $mlPython -B services/ml/verify_demo_company_rpc.py --insecure-local --target 127.0.0.1:50051 --output-dir services/ml/artifacts/demo_loopback_new --main-sha $mlMainSha
```

После репетиции остановить только свой сервер, проверить закрытие его порта
и удалить только выпущенные для неё одноразовые ключи. Не публиковать ключи,
bindings, participant dataset или JSON-артефакты.

## Что проверено 24.10

Использованы одноразовый CA (срок 1 день) и идентичности `ml`, `core`, `api`;
ключи после проверки удалены.

- RPC/mTLS-тесты: 39 из 39 без пропусков; все ML-тесты: 107 из 107.
  Добавлен `tests/test_rpc_demo_company.py`: до исправления он падал на
  −53 000 ₽, после исправления проверяет сохранение суммы обязательства и все дни.
  Второй тест вызывает тот же клиент по mTLS с URI `spiffe://saldo.local/core`;
  одноразовые сертификаты находятся только в его временном каталоге.
- `verify_demo_company_rpc.py` на компании стенда, cutoff 30.09 и 23.10:
  - 30 последовательных дат;
  - остаток совпадает до копейки с `cutoff EOD + Σ(приход − расход)`;
  - первый отрицательный день совпадает с расчётом;
  - ответ без выплаты проходит все правила проверки Core из `financial.go`;
  - повтор того же запроса даёт идентичный ответ.
- Без двойного учёта:
  - выплата — ровно один компонент на 10 000 ₽;
  - расход за горизонт = 30 × 4 000 + 10 000 ₽;
  - после частичной оплаты расход за горизонт = 30 × 4 000 + 7 000 ₽,
    accepted EOD = 47 000 ₽, оплаченная часть исключена из среднего через группу замены;
  - конечный остаток до и после partial одинаков: −50 000 ₽.

  Без этой связи среднее завышается на 33,33 ₽ в день.
- Вероятность: `gap_probability` и каждый день — `not_calibrated`,
  `calibrated=false`.
- mTLS:
  - идентичность `api` — `PERMISSION_DENIED`;
  - клиент без сертификата не подключается;
  - `core` с пустым запросом — `INVALID_ARGUMENT`.
- `verify_manual_payment_rpc.py` (компания датасета C002) по mTLS: добавить,
  изменить, перенести, частично оплатить, отменить — точная арифметика.
  Дубль ID и отсутствие подтверждения после cutoff отклоняются.

Артефакты повторной mTLS-проверки: `artifacts/demo_mtls_20261024T205040Z/`;
`manifest.json` содержит SHA main/кода/proto/источников, версии библиотек,
результаты вызовов и проверки завершения сервера/удаления ключей.
Полный набор: `artifacts/demo-final-unit.log`, `artifacts/demo-final-rpc.log`.
Ruff 0.17.1 check/format — PASS. Это локальные результаты, не новый CI GitHub.
