# Payments / frozen calendar — итоговый handoff

Статус: **На ревью**. PR остаётся draft: https://github.com/benzocloud/hack-rnd-autumn-2026/pull/48.
Branch: `globalarray/feat/payments_frozen_calendar`.
Original base: `0abae4f`; проверенный integration base: `6c9edd969b3a694bccabaf6980b86531e2722490`.
Current implementation SHA: `ac90fa3bc7118403275c7a2e9d24c5d0eba75e0b`.
Remote SHA на последнюю проверку: `ac90fa3bc7118403275c7a2e9d24c5d0eba75e0b` (совпадает).
Последующий docs-only commit не меняет implementation SHA; окончательный head проверяется `git ls-remote` и указан в PR.

## Готовность

Работает полный payment lifecycle через API → Core → PostgreSQL, включая CAS,
CSRF/Auth scope, immutable create/match replay, точные RUB-копейки и историю
reversal. Cancel сохраняет оставшуюся сумму и allocations; unmatch после cancel
явно возвращает PAYMENT_STATE_CONFLICT, factual transaction не удаляется.

В covered CURRENT профиле `mean_90_components_v1/calendar_replacement_v1`
новый прогноз учитывает remaining ровно один раз. Одна согласованная Tx фиксирует
calendar amount/remaining/status/version/source/group, независимые reconciliation
groups, mode/as-of/provenance, factual revision и model context. Worker/read не
перечитывают live calendar. Go независимо проверяет residual после replacement,
суммы компонентов, source IDs, даты, непересечение групп и overflow. Public
components/effective occurrences детерминированно проектируются из сохранённых
components и frozen snapshot; residual components/history evidence также хранятся
в immutable result. Ни одна projection не использует live payment.

Additional требует явного подтверждения. Complete replacement сохраняется при
cancel/full match/переносе за горизонт; residual возвращается только после revoke.
CURRENT допустим только cutoff сегодня/вчера и без post-cutoff фактов; overdue
отклоняется, дата не переносится. Available empty и unavailable различаются.

Последний законченный этап: lifecycle + snapshot + Predict + regression +
независимые reviews, **На ревью**, без owner acceptance.

## Frontend: фактически подключённые endpoints / DTO

Все пути ниже относительно `/api/v1/companies/{company_id}`. Account и actor
Core/API получают из авторизованного scope, браузер их не сертифицирует.

| Метод/путь | Ввод | Ответ |
|---|---|---|
| GET `/payments` | limit/cursor/from/to/status; default planned+partially_matched | items, next_cursor |
| POST `/payments` | direction, amount, scheduled_date, counterparty_name, description; optional category_id/reconciliation_group_id; Idempotency-Key | 201 Payment |
| GET `/payments/{id}` | resource UUID | Payment |
| PATCH `/payments/{id}` | version + изменяемые поля; omission сохраняет, category/group null очищает | Payment |
| DELETE `/payments/{id}?version=N` | decimal resource version | 204 |
| GET `/payments/{id}/matches` | limit/cursor | items, next_cursor; includes removed_at tombstones |
| POST `/payments/{id}/matches` | transaction_id, payment_version, transaction_version, amount; Idempotency-Key | 201 Match |
| DELETE `/payments/{id}/matches/{match_id}?payment_version=N` | decimal payment version | 204 |
| GET/POST `/reconciliation-groups` | list include_revoked/limit/cursor; create exact cutoff/horizon + tagged evidence + Idempotency-Key | list / 201 Group |
| GET `/reconciliation-groups/{id}` | group UUID | Group |
| POST `/reconciliation-groups/{id}/revoke` | {"version":"N"}; Idempotency-Key | Group |

Payment DTO: id/account_id/direction, amount/remaining_amount, scheduled_date,
category_id, counterparty_name, description, status, source, version, created_at,
reconciliation_group_id. Money.amount_minor и версии — decimal strings, currency RUB.
Match: id/payment_id/transaction_id/amount/created_at/nullable removed_at.
Group kind=additional с additional.confirmed=true либо kind=replacement с
replacement.complete_for_horizon=true и historical_operations[{transaction_id,version}].
Write requests сохраняют Origin/Referer, cookie session и X-CSRF-Token.

Canonical wire diff: `PaymentMatch.optional Timestamp removed_at=6`;
`ForecastInput.calendar_knowledge_mode=18` typed enum (mirror ML values0–4),
`calendar_as_of_date=19`, `no_post_cutoff_movements=20`. Поля1–17 не менялись.
REST input projection использует current/unavailable/nullable legacy mode.
calendar_mode request available/synthetic — выбор режима, не доказательство знания.
Synthetic selection сейчас явно unsupported. Contracts: proto/core/v1/payments.proto,
proto/core/v1/forecasting.proto, docs/api/openapi.yaml, docs/core/rest-rpc-map.json.

## Проверки

- Core после merge: full serial `-tags=integration,mlintegration -race -count=1 ./...`
  PASS, 755 tests/subtests,30 test packages,9 packages без tests; один skip —
  opt-in large-import measurement. Full tagged module lint0, vet и fmt PASS.
- Real HTTP `TestPaymentForecastHTTPWithPostgresPythonAndRestart` и
  `TestPaymentHTTPAPILifecyclePersistsAudit`: семь forecasts после create →
  amount edit → date edit → partial match → unmatch → rematch → cancel;
  все30точек/remaining/components, stale старого прогноза и byte-identical
  snapshot/input/result, история и API restart. Auth double явно отмечен;
  финансовые Core/PG/Python/mTLS вызовы настоящие.
- `TestPaymentForecastFrozenCalendarPostgresPythonRestart`: live edit до старта
  worker не меняет старый forecast; replacement остаётся после cancel/full match/
  даты за горизонтом; explicit revoke восстанавливает residual; reopened pool
  читает старые bytes. Covered fixture не доказательство ordinary CSV.
- Negative PG tests: duplicate key/body conflict, stale payment/fact/group,
  foreign scope/reference/link, overlap, revoked group, overmatch, cancelled
  unmatch; current/mode/overdue/ML unknown/unavailable regressions.
- API штатный check: generation/build/unit/race/vet/fmt/full lint/Ruff/schema,
  own coverage **88.42% statements /88.77% lines**, HTTPS/mTLS process smoke,
  OpenAPI conformance41 responses — PASS. `/tmp` noexec потребовал TMPDIR в
  workspace для smoke; первоначальный env failure не считался pass.
- Buf lint/build/breaking/drift + consumer compilation +58 REST maps PASS;
  API schema13 PASS. API bindings --check PASS с pinned grpc_tools1.78/protoc6.33.5.
- ML после integration main:55 RPC/mTLS tests без skips PASS; pinned Ruff0.17.1
  check/format17files PASS. Обучение не запускалось, default не менялся нами.

Ранний полный Core прогон имел2 roundtrip failures; microsecond/UTC/nil-slice
normalization и проверки исправлены, последний полный merged прогон зелёный.
Первый опубликованный f344200 пропустил API file и не собирался; исправлен
f4627f1. Не считать эти ранние результаты зелёными.

## Независимое review

caveman-review + ponytail-review выполнены read-only отдельным агентом:
- f344200:4 findings (missing file, peer allowlist, nil source, imported linkage replay).
- 2723d0a: все4 исправлены, новых нет.
- 27657de: roundtrip fixes/main shared router проверены, новых нет.
- ac90fa3: новые HTTP tests проверены, production прежний; новых нет.
Ponytail net:-0 lines possible. Review не заменяет owner acceptance.
GBrain unavailable (MCP не подключён); RTK отсутствует. Original reference
checkouts недоступны; использованы tracked go-services guidance и ADR.

## Known limitations / зависимости

- `mean_90_csv_v1` не поддерживает calendar: при любом persisted payment/group
  CreateForecast явно UNSUPPORTED_MODEL_CONTEXT, без молчаливого игнорирования
  или fallback. Для совместимости нужен отдельно принятый versioned CSV/calendar
  profile + acceptance; менять модель/default этим PR не разрешалось.
- Historical nonempty/synthetic calendar не реализован; отказ явный. Не выдавать
  CURRENT covered сценарий за historical knowledge или ordinary CSV поддержку.
- Probability not_calibrated; настоящий browser/Auth acceptance у других агентов.
- #47 @7e5c0d4 интегрирован обычным merge; теперь включён в main6c9edd9.
  Onboarding/training/frontend/deploy изменения из merge принадлежат main,
  собственный diff к main эти компоненты не меняет.

## Schema / rollback / окружение

00013: audit tombstones; down отказывается при reversed allocations.
00014: независимые scoped groups и nullable payment FK.
00015: snapshot payload_version1/2; down откажется при payload2 records.
Новые current jobs используют schema2, старые codecs/results читаются.
Перед возможной поставкой остановить старый Core, затем запускать новый
producer/worker. Rollback сохраняет schema2 decoder; старый бинарник нельзя
смешивать с новыми jobs. Ни merge в main, ни ready-for-review, ни deployment,
ни изменение работающего сервера не выполнялись.

Own disposable PostgreSQL18: .agent/local/pg18,127.0.0.1:55488,
DB saldo_core_test, marker saldo-core-integration, штатные migrations1–15.
Локальные tools/state в ignored .agent/local. Финальный process teardown
фиксируется отдельным checkpoint; чужие DB/socket/Docker не использовались.

## Следующая команда / owner pickup

`gh pr checks 48` — проверить CI окончательного head, pending не pass.
Owner: review draft48; frontend B подключает DTO выше; C прогоняет настоящий
Auth/browser сценарий. CSV/calendar activation требует отдельного profile решения.
Окончательный CI/status и remote head публикуются последующим docs checkpoint.

## История checkpoint

34b6add — ранние plan/handoff/draft;22a5bcc — dependency merge;
f344200 — lifecycle;f4627f1 — publication/peer/projection fixes;
2723d0a — current calendar;bed1e68 — roundtrip fixes;
a4c7078 — clean main merge;27657de — integration handoff;
ac90fa3 — API coverage tests. Полная история статусов/команд сохранена в Git.

## Финальная фиксация

Own PostgreSQL18 остановлен успешно (`pg_ctl ... -m fast stop`), каталог данных
сохранён в ignored `.agent/local/pg18`. Чужие процессы/серверы не затрагивались.

Actual CI implementation ac90fa3 на 25.10.2026 07:52 UTC: ML RPC **SUCCESS**;
Core contracts/lint, API verify и Real company onboarding **IN_PROGRESS**.
Pending не pass. Core source прежний проверенный27657de; последний test-only
commit увеличивает API coverage. Проверки могут продолжаться после docs-only
финального checkpoint, точный head/status: `gh pr view 48 --json headRefOid,isDraft`
и `gh pr checks 48`.

CI links: [Core](https://github.com/benzocloud/hack-rnd-autumn-2026/actions/runs/40174232071),
[API/onboarding](https://github.com/benzocloud/hack-rnd-autumn-2026/actions/runs/40174232163),
[ML RPC](https://github.com/benzocloud/hack-rnd-autumn-2026/actions/runs/40174232041).
Локальные обязательные gates завершены, upstream CI ещё ожидается. Последний
законченный этап5 — **На ревью**, без acceptance/merge/ready/deploy.

Owner pickup: проверить окончательный CI, review draft48, передать frontend B
подключённые DTO и C настоящий Auth/browser сценарий. CSV/calendar и historical
synthetic требуют принятого совместимого профиля; no silent fallback сохранён.

## Возобновление A — CSV+CURRENT (25.10.2026 11:12 МСК)

Owner прямо разрешил минимальную versioned CSV/calendar совместимость. Старый
UNSUPPORTED_MODEL_CONTEXT для валидного CSV+CURRENT теперь функциональный пробел,
который реализуется в том же draft48, без нового промежуточного апрува.
Previous head/local/remote:1f10b6e615296a01d64957730ac2ce38a2e2b11c.
Fresh main71f2a86 (verification frontend change) merged clean в текущую ветку;
собственный source frontend/Auth/deploy не редактируется. Активны PR48/49/50;
прочитаны body и полные comments/reviews/inline, все обсуждения пусты. Issues нет.
B=#49 dfa9f505, C=#50 e95e98ee; их файлы/PR не меняются.

Context: mean_90 / mean_90_csv_calendar_v1 /
csv_observed_operations_calendar_v1 / config mean_90_csv_calendar_v1.
Exact wire/DTO diff: отсутствует; existing CSV assessment + CURRENT calendar
передаются в том же Predict. Новые profile/preprocessing/config значения будут
видны в существующем ForecastInput.model_context. Limitations: старый CSV list
без historical_calendar_unavailable, с current_calendar_only в конце; остальные
CSV ограничения и not_calibrated сохраняются. Schema1/2 backwards read сохраняется.
Никакого certified proof, reconstructed observed zeros, operator manifest или
SQL facts. Residual только supplied operations после proven replacement /90
плюс known remaining ровно один раз; независимая Go проверка обязательна.

Последний законченный этап прежнего результата На ревью; продолжение6 в работе.
Tools/state .agent/local сохранены; RTK/GBrain MCP ранее отсутствовали, новые calls
не заявляются. OwnPG18 остановлен после прошлого блока, поднимается снова.
Следующая команда: `pg_ctl -D .agent/local/pg18 -l .agent/local/pg18.log -o '-h 127.0.0.1 -p 55488 -k /tmp' start`; затем Core selector/ML shared validation.

## CSV+CURRENT happy-path checkpoint — 25.10 11:16 МСК, На ревью

Plan continuation pushed a7acc469ae1a99a63e565a13fe4d172346bccec6 (remote совпал).
Новый profile mean_90_csv_calendar_v1/csv_observed_operations_calendar_v1, config
mean_90_csv_calendar_v1 реализован без wire/REST DTO изменений и без миграций.
Старые CSV-only/covered/schema1/2 сохранены. Frozen assessment для нового profile
содержит CSV limitations + current_calendar_only, не completeness proof.

`TestOrdinaryCSVCalendarHTTPPostgresPythonLifecycle` — race PASS3.166s:
настоящий multipart ordinary CSV import/commit без manifest/SQLfacts/proof,
90-day window/2operation days/88unknown days; CSV-only oldprofile→additional
newprofile→amount/date/partial/unmatch/rematch/cancel, все30points; replacement
fullmatch/outsidehorizon/cancel не возвращают residual без revoke; API/Core reader
restart, old immutable snapshot/input/result и staleness. Auth double явно указан.
ML59 RPC/security/financial/trained tests безskips PASS; pinned Ruff PASS.
API штатный fullcheck PASS, coverage88.70%statements/89.04%lines; source/DTO/path
unchanged. Go service/client units PASS, independent response corruption cases
добавлены; full Core taggedrace/vet/lint нового diff и независимые reviews далее.

Неоднозначности не скрыты: newprofile только CURRENT; historical/synthetic не
расширены. CSV assessment проверяется по исходным полным facts/EOD до исключения
replacement. Вычитается только подтверждённая group history, known remaining
прибавляется один раз; group lifetime прежний. Точное соответствие direction/RUB/
scope/version сохраняется в frozen facts и calendar, категории не доказательство.
Следующая команда: full serial Core taggedrace/vet/lint и independent review
публикуемого SHA относительно1f10b6e; исправления пакетно, затем push.

## Review continuation checkpoint — 11:21 МСК

Published/local/remote a97f9c694ae41360867bb66877452c863c3326e0 совпадают.
Независимые caveman-review/ponytail-review нового diff на a97f9c6 относительно
1f10b6e завершены: correctness findings нет; ponytail нашёл только unused
csvProfile helper (3 строки), удалён. Core full tagged lint0/vet PASS, Buf/drift/
consumer contracts и schema13 PASS. Full serial taggedrace идёт безfailures,
финальный статус будет записан после завершения. API полного check coverage
88.70/89.04, ML59безskips и realCSV lifecycle3.166s уже PASS.
Точная следующая команда: проверить `.agent/local/core-csv-calendar-full.json`,
затем finalSHA review fix и CI. No protocol/schema changes beyond prior PR.
