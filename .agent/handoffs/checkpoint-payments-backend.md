# Payments / CSV + frozen CURRENT calendar — handoff A

Статус: **На ревью**, не Принят. Один draft PR:
https://github.com/benzocloud/hack-rnd-autumn-2026/pull/48.
Branch: `globalarray/feat/payments_frozen_calendar`.
Initial base:0abae4f; проверенный integration base:
`71f2a86ffbcf218d7fff517830b7aa41f0a97c95` (fresh main, влит обычным merge).
Current implementation/local SHA: `0826fa6118f29bf3609577c668af5e0cd291bf1d`.
Remote SHA на проверку: `0826fa6118f29bf3609577c668af5e0cd291bf1d`, совпадает.
Последующий docs-only checkpoint сохраняет этот implementation SHA; итоговый
head публикуется в PR и проверяется git ls-remote. Рабочее дерево до docs clean.

## Основной путь закрыт

Обычный CSV без operator manifest → preview/commit → CSV-only forecast →
создание additional/replacement payment → новый CSV+CURRENT forecast →
amount/date edit → partial match → unmatch → rematch → cancel → новый forecast
→ API/Core reader reload/restart. Все financial API/Core/PG/Python/mTLS вызовы
настоящие. Auth double обозначен; настоящий Auth/browser сценарий остаётся B/C.
SQL facts/proof/fixture allowlist для приёмки этого пути не вставлялись.

Предыдущий CSV/calendar UNSUPPORTED_MODEL_CONTEXT для валидного CURRENT пути
устранён. Owner continuation прямо разрешил минимальный совместимый profile;
новый промежуточный апрув не требовался. Все стадии остаются На ревью.

## Точный profile context / compatibility

- model_name: `mean_90`
- model_version: `mean_90_csv_calendar_v1`
- preprocessing_version: `csv_observed_operations_calendar_v1`
- configuration_id: `mean_90_csv_calendar_v1`

Новый context выбирается Core только для eligible CSV с persisted calendar и
допустимым CURRENT. До календаря старый `mean_90_csv_v1` сохраняется. Covered
`mean_90_components_v1` и trained context из #45 не менялись; training не запускался,
глобальный default не менялся. RPC по-прежнему один Predict.

CSV eligibility прежняя: 90 календарных дней, cutoff EOD, exact reconciliation
исходных полных supplied facts до replacement, unknown days, manual provenance,
scope/revision и existing limits. CSV полнота не становится certified от группы.
Новый assessment/model/calendar/groups/remaining заморожены в schema2 input +
snapshot. Старые schema1/2 CSV-only/covered jobs/results читаются без изменения
bytes; worker/read не используют live calendar. Новая revision/context создаёт
новый result (кроме нормального replay того же idempotency key).

Limitations нового input/assessment/result в точном порядке:
`csv_history_completeness_unverified`, `unobserved_days_unknown`,
`observed_operations_mean_per_calendar_day`, `historical_availability_unverified`,
`not_calibrated`, `current_calendar_only`. Probability unavailable/null, bands
отсутствуют; unknown не становится наблюдённым нулём.

Additional требует explicit confirmation. Replacement содержит exact scoped
operation IDs/versions, completeness evidence и непересекающиеся группы; direction
и RUB amounts сохраняются. Категория не доказательство. Residual доступных facts
после replacement /90 с прежним half-up rounding плюс known remaining ровно один
раз по исходным датам. Go независимо проверяет arithmetic/components/evidence/
source IDs/overflow. Cancel/full match/за горизонтом не отзывают mapping; только
explicit revoke возвращает историю в residual.

CURRENT поддержан на cutoff сегодня/вчера без post-cutoff movements. Historical/
synthetic не расширялись; overdue явно отклоняется, не сдвигается.
Available empty отличается от unavailable. Если ввод/контекст невалиден — явный
отказ, no silent fallback. Unmatch после cancel — PAYMENT_STATE_CONFLICT, audit
история и factual transaction сохраняются.

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


Продолжение не меняет wire/REST DTO/path: используются существующие CSV assessment,
CURRENT provenance и model_context strings. OpenAPI/schema descriptions уточнены;
новых proto fields/генерации или миграций для этого продолжения не требуется.
B может использовать прежние endpoints/DTO; модель/ограничения видны в input/result.

## Проверки продолжения

- Реальный `TestOrdinaryCSVCalendarHTTPPostgresPythonLifecycle` с race PASS:
  динамический Moscow yesterday cutoff, 90-day CSV с2 operation days и88 unknown
  days; настоящий multipart upload/preview/commit без manifest/SQL proof;
  старый CSV-only model → новый calendar model; весь lifecycle; все30точек,
  remaining/revisions/staleness/immutable snapshot/input/result; replacement
  fullmatch/outside-horizon/cancel/revoke; API restart и новый Core reader pool.
  Узкий прогон2.11s, package3.166s; включён также в full suite.
- Core full serial `go test -json -p 1 -tags=integration,mlintegration -race -count=1 ./...`
  PASS: **765 tests/subtests,30 test packages**,9 packages без tests. Один skip —
  opt-in large-import measurement. Full tagged module lint **0 issues**, vet PASS.
- API полный `scripts/check.sh` PASS: generation/build/unit/race/vet/fmt/full lint0,
  pinned Ruff/data-contracts, HTTPS/mTLS smoke10endpoints и OpenAPI conformance41
  real responses. Coverage **88.70% statements /89.04% lines** (gate85 не ослаблен).
  Добавлены positive+18 negative new-profile validation cases.
- ML full RPC/security/financial/trained suite **59 tests,0 skips PASS**. Pinned
  Ruff0.17.1 check/format **19 files clean**. Shared CSV history validator и calendar
  contributions переиспользуются без изменения прежних CSV/covered/trained правил.
- Buf lint/build/breaking/generated drift/consumer compilation и58 REST mappings
  PASS; API schema13 PASS. Protocol shape не менялся.
- Negative/backward проверки: missing EOD, short history, reconciliation gap,
  unknown mode/model/preprocessing/config, no-post-cutoff guard, unavailable ML,
  overlap/foreign scope/stale/CAS/replay/overmatch, forged source/proof/reconstructed
  history, duplicate occurrence/evidence, large money/overflow, fabricated bands/
  probability. Старые CSV-only/covered regression tests сохранены.

Предыдущие gates (755 Core /55 ML и покрытый CURRENT lifecycle) не потеряны:
пройден новый расширенный suite. Ранние4 findings missing API file/peer allowlist/
nil source/import linkage replay и roundtrip microsecond/UTC/nil-slice исправления
остались; не откатывались ради нового пути.

## Независимое review

На `a97f9c694ae41360867bb66877452c863c3326e0` относительно прежнего1f10b6e
независимый read-only агент применил caveman-review и ponytail-review:
correctness findings нет; ponytail нашёл только dead csvProfile helper(-3стр).
Удалён в `0826fa6118f29bf3609577c668af5e0cd291bf1d`; independent followup этого SHA
подтвердил закрытие, новых findings нет, net:-0. Review не является acceptance.

RTK отсутствует. GBrain unavailable: в доступном MCP tool catalog нет GBrain;
запросы recall/remember не изображались. Original reference checkouts отсутствуют,
использованы tracked go-services guidance, ADR и фактические исходники.

## Actual CI — внешний blocker

GitHub Actions на `0826fa6` возвращает FAILURE до запуска job: **steps=[]**,
logs отсутствуют. Check annotation дословно:
“The job was not started because recent account payments have failed or your
spending limit needs to be increased. Please check the 'Billing & plans' section
in your settings”. Это billing/spending-limit аккаунта, не тестовый failure.
CI не называется passing. Billing/settings не изменялись, guards не отключались.

Repro: `gh pr checks 48`; `gh api repos/benzocloud/hack-rnd-autumn-2026/check-runs/121117431070/annotations`.
[Core run](https://github.com/benzocloud/hack-rnd-autumn-2026/actions/runs/40176243852),
[API run](https://github.com/benzocloud/hack-rnd-autumn-2026/actions/runs/40176243843).
После восстановления Actions владельцем — повторить failed runs актуального head.
Все доступные локальные gates завершены до этого отчёта.

## Scope / schema / rollback

#41/#45/#47 merged и включены в проверенный main71f2a86. B:#49, C:#50 — отдельные
активные draft PR; их файлы/планы/handoff/PR не менялись. Полные доступные discussions
48/49/50 прочитаны (comments/reviews/inline пусты). Issues нет. Собственный diff к
main не меняет frontend, Auth/onboarding, scripts/financial-e2e, docs/verification,
deploy или training/assets. Дополнительные изменения frontend пришли только из
обычного upstream merge main.

Продолжение использует existing snapshot/job schema1/2; новых миграций нет.
Ранее в #48 добавлены00013–00015 для audit/groups/snapshot2, их rollback guards
сохраняются. Новый profile требует поддерживающего ML и Core decoder: при будущей
разрешённой поставке сначала ML, остановить старый Core до нового producer/worker;
rollback сохраняет profile/schema2 support. Неподдержанный старый бинарник нельзя
смешивать с новыми jobs. Старые immutable payloads не переписываются.

Own disposable PG18: .agent/local/pg18,127.0.0.1:55488,DB saldo_core_test,
marker saldo-core-integration, штатные migrations1–15. Никакого общего запущенного
стека, чужой DB, Docker socket или публичной среды не использовалось. Финальный
teardown собственного процесса фиксируется docs checkpoint. Tools/logs только в
ignored .agent/local, сессионные логи/credentials не добавлены в Git.

## Точный подхват

Следующая команда: `gh pr checks 48` (billing blocker, не pass).
Owner: восстановить Actions quota/billing и повторить failed checks, review draft48;
B подключает тот же REST contract, C прогоняет настоящий Auth/browser сценарий
ordinary CSV→calendar на изолированном кандидате. Historical/synthetic и качество/
calibration остаются отдельными scope. Merge/ready/deploy не выполнялись.

Финальный local teardown 25.10.2026 08:26 UTC: собственный PostgreSQL18 остановлен
успешно. Directory/state сохранены в ignored .agent/local; общий стек не трогался.
Source0826fa6 и remote совпадали до docs checkpoint; следующий docs commit не
меняет проверенный source. Repro gates/версии выше сохранены, старые fixes не потеряны.
