# Handoff финансового frontend

Дата: 25.10.2026, старт 10:36 МСК; checkpoint 11:15/11:45 МСК.
Ветка: globalarray/feat/financial_user_journey. Base: 6c9edd9 (origin/main). Local/remote предыдущего push: 27226dd7d6818eb2e1d0fdfee11cbf44625ffd58; текущий исходный срез — следующий commit, SHA публикации проверяется git ls-remote. PR: https://github.com/benzocloud/hack-rnd-autumn-2026/pull/49.
Этап 1: На ревью. Прочитаны инструкции/ADR/workflows, README/OpenAPI, исходники Imports/Calendar/Dashboard; обсуждения #47/#48 пусты, #47 merged.

| API | Готовность/следующий шаг |
| --- | --- |
| imports upload/get/issues/commit | клиент существует; проверить stale UX и ack |
| transactions POST/list, account balance | #47 merged; не дублировать coordinator changes |
| forecasts POST/get/result | клиент существует; проверить polling/reload/30 days |
| payments list/detail/create/update/cancel/matches | клиент существует, main backend ещё не подключён; dependency #48 |
| reconciliation controls | сверить опубликованный DTO #48; не выводить source IDs из текста |

Dependencies: https://github.com/benzocloud/hack-rnd-autumn-2026/pull/47 merged f686ca86438948ca4333f0125513a58c5045ce59; https://github.com/benzocloud/hack-rnd-autumn-2026/pull/48 head f4627f1cfcca5fa2ad01ec7e52a618f8e608d4ea.
Проверки: git fetch origin выполнен; новый isolated worktree /tmp/saldo-financial от свежего main, чужие dirty files не копировались. Финансовые браузерные сценарии ещё не проверены; screenshots отсутствуют. CI не проверен.
Инструменты: GBrain MCP не предоставлен; RTK отсутствует. Sandbox bwrap namespace недоступен, команды разрешены через escalation.
Следующий шаг: закончить матрицу source/API, исправить независимые состояния/формы и опубликовать блок.

## Блок imports/manual/forecast, 10:45 МСК — На ревью

Изменены frontend financial screens/helpers/hooks/dashboard service: stale preview запрещает повтор commit и предлагает новую загрузку; warnings объясняют сохранение обеих manual/import записей. Manual retry сохраняет исходное тело и CAS; после VERSION_CONFLICT доступно явное обновление версий без потери полей. Деньги поддерживают точные signed int64, включая значения выше Number.MAX_SAFE_INTEGER. Ручной остаток пояснён как датированный якорь. Dashboard только 30 дней; bounded detail polling (40 проверок), явные queued/running/terminal states, таблица точных сумм и CSV ограничения. Backend cancellation отсутствует в текущем DTO/RPC: UI обрабатывает неизвестный терминальный статус без бесконечного spinner, серверная cancellation не объявляется реализованной.
Проверки из frontend/: npm ci повторно успешно после переноса с /tmp (512MiB) на /workspace/repo/.financial-worktree; npm test — 34/34, npm run typecheck — passed, npm run lint — passed. Build будет один раз после календаря. Ранний npm test ошибочно запущен из корня (ENOENT), исправлен запуском из frontend; успешные результаты относятся к frontend.
Страницы в настоящем браузере ещё не проверены. Screenshots отсутствуют. Это unit/contract regression, не real backend acceptance.
Следующий шаг: календарь и reconciliation на DTO опубликованного #48; после — полный build и browser проверки при доступных зависимостях.

## Календарь/reconciliation, 10:49 МСК — На ревью

Последний проверенный local/remote SHA: 6a9aa1305ee56f0c3c06e1bd2d459302840cbcb8. Branch/base/PR прежние; этот блок публикуется следующим commit, remote проверяется после push.
Календарь: фильтры active/planned/partial/matched/cancelled; detail/history доступны и после отмены, remaining/provenance/group видимы. Partial разрешает дату/метаданные без отправки запрещённых amount/direction. Cancel отменяет remaining; match не проводит платёж; removed_at из опубликованного #48 показывается как снятая связь, повторный unmatch скрыт. После мутаций перечитываются resource/account и предлагается явный новый прогноз.
Reconciliation: существующий OpenAPI tagged DTO дополнен в frontend; list/create/revoke API, явный выбор additional/replacement без default, окно 30 дней, отдельное подтверждение completeness и выбор точных transaction IDs/versions с пагинацией. Создание группы отдельно от привязки платежа. Несогласованный режим явно выбран, не объявляется дополнительным обязательством. Ошибки API показываются без ложного успеха.
Контрактная зависимость: #48 f4627f1cfcca5fa2ad01ec7e52a618f8e608d4ea. GET/POST /companies/{id}/reconciliation-groups, POST /{group}/revoke, payments GET/POST/PATCH/DELETE и matches GET/POST/DELETE имеют handlers в этом SHA. Main их пока не содержит. Match.removed_at optional добавлен в OpenAPI именно #48; локальный main-контракт ещё без поля.
Проверки: npm run typecheck passed; npx eslint src/features/Calendar src/interfaces/payments src/service/payments_service tests/payment-contract.test.mjs passed; node --import ./tests/register.mjs --test tests/payment-contract.test.mjs — 2/2 passed; npm run build passed (production, developerMode default dev). Adapter regression — fixture responses, НЕ backend acceptance.
Runtime blocker: curl https://saldo.brokebyte.me/api/v1/auth/csrf => HTTP 503; http://127.0.0.1:8080/healthz => connection refused. Go executable отсутствует, выданного runtime/mTLS/test-session в контейнере нет. Не запрашивались чужие credentials. Полный реальный browser/backend lifecycle не пройден.
Независимые caveman-review и ponytail-review запущены отдельно, pending. Следующий шаг: исправления по review, browser UI smoke с явно отмеченными fixture responses, итоговый main/CI audit.

## Review fixes и итоговый независимый frontend срез, 11:04 МСК

Последний проверенный local/remote: 7d6d749c4dcd0547e11d48162527c38021dbb003; base 6c9edd969b3a694bccabaf6980b86531e2722490. PR #49: https://github.com/benzocloud/hack-rnd-autumn-2026/pull/49. Изолированный checkout после ENOSPC: /workspace/repo/.financial-worktree.
- Независимый caveman-review: найден category CAS recovery (account обновлялся, editing.version оставалась старой). Исправлено перечитыванием точного transaction ID с сохранением выбранной категории; повторный reviewer подтвердил исправление. Последующие узкие read-only проверки CSV, pagination и frozen result regressions не нашли.
- Независимый ponytail-review: убрать цикл по [30] в тесте. Исправлено. Ревью не означает owner acceptance.
- CSV: логические записи учитывают quoted multiline и escaped quotes; тест исключает ложную компанию из текста назначения. Preview DOM ограничен 20000 символов, исходный файл передаётся целиком. Показан прогресс upload и отдельное ожидание server acceptance.
- Match transaction picker загружает по 20 операций, не всю историю 300k строк. History показывает removed_at и запрещает повторный unmatch снятой связи; cancelled запрещает match/unmatch.
- Одинаковые понятные ошибки HTTP и сохранённых failed jobs. Неизвестные error codes сохраняют server message.
- ForecastDetail показывает frozen calendar context и платежные components из результата, не из live calendar. Unknown/unavailable и synthetic обозначены; old ID не получает новый контент.
- Reconciliation completeness снимается при изменении окна/режима/source IDs; никаких default additional или текстовых эвристик.

### Матрица маршрутов/DTO на проверенных исходниках

Все пути относительно /api/v1. Main source: 6c9edd9; payments source: #48 ac90fa3bc7118403275c7a2e9d24c5d0eba75e0b (обновление f4627f1, handlers совместимы, добавлен calendar context). Backend commits не включены в frontend PR.

| Method/path | Существующий DTO/service и semantics | Проверка/ограничение |
| --- | --- | --- |
| POST /companies/{c}/imports | multipart IUploadImportParams, Idempotency-Key; imports_service → imports.go/spool | adapter есть; upload progress; реальный runtime не пройден |
| GET /companies/{c}/imports/{i}; GET .../{i}/issues | IImport/IImportIssueList, cursor/limit | unit paging и CSV selection; preview_ready/failed/applied UI |
| POST .../imports/{i}/commit | ICommitImportRequest version/expected_data_version/ack, stable key | stale требует новую загрузку/подтверждение; автоматического recommit нет |
| GET/POST /companies/{c}/transactions | ITransactionList/ICreateTransactionRequest; ledger_manual.go | exact money, CAS/body/key freeze retry; imported duplicate server error; реальный POST не подтверждён |
| PATCH .../transactions/{t}/category | IUpdateCategoryRequest category_id/version | сохранение выбора + явное refresh версии операции |
| GET /companies/{c}/account; PUT .../account/balance | IAccount/ISetBalanceRequest, dated signed anchor | неизвестные дни не заполняются, statement EOD conflict объяснён |
| GET/POST /companies/{c}/forecasts; GET .../{f} | IForecastAccepted/IForecast; 30 days; bounded polling, abort | reuse saved result, stale flag/revision, null probability; cancellation отсутствует в backend контракте |
| GET/POST /companies/{c}/payments; GET/PATCH/DELETE .../{p} | existing IPayment/Create/Update, resource version; #48 | active/history filters, partial metadata only, remaining cancel; настоящего runtime нет |
| GET/POST .../payments/{p}/matches; DELETE .../matches/{m} | IMatch + optional removed_at из #48; payment/transaction versions | match не проводит деньги; removed/cancelled unmatch скрыт; adapter regression |
| GET/POST /companies/{c}/reconciliation-groups; POST .../{g}/revoke | OpenAPI tagged additional/replacement + exact refs + true completeness; idempotency | list/create/revoke подключены к #48; нет выводов по category/text |

### Проверки и фактическая готовность

- Исходный комплект: npm test 34/34; npm run typecheck; npm run lint; npm run build — passed.
- После календаря: 2/2 payment adapter tests, targeted lint/typecheck/build passed.
- После CSV/review fixes: 6/6 affected tests passed; после job errors/pagination/context: 16/16 affected tests, typecheck/build passed. Unnecessary hook dependency warning устранён, узкий eslint повторно passed.
- Chromium fixture smoke: desktop и 390px; partial date PATCH без amount/direction; active/removed matches; cancel remaining + cancelled history; manual amount 9007199254740993 копеек после fixture 503 сохраняет поля и тот же key/body при изменившемся account revision; explicit additional/replacement (смена selected IDs снимает completeness), category CAS refresh сохраняет выбор и обновляет transaction version, payment POST с group ID; forecast graph/30-row exact table/reload. Это CDP fixture interception, НЕ настоящая Go API приёмка. На изображениях отдельная маркировка. Артефакты: frontend/artifacts/financial-fixture/.
- CI на 7d6d749: frontend lint/test/build SUCCESS; deploy SKIPPED. Финальный push запустит новый CI; pending не считается passed.
- Foreign company error и polling abort/terminal error покрыты существующими dashboard tests; CSRF/cookie transport guards сохранены и проходят transport regression. Настоящие session expiry/CSRF rejection/ML unavailable через browser/backend не пройдены.

### Открытые blockers и подхват

1. Real backend acceptance BLOCKED: https://saldo.brokebyte.me/api/v1/auth/csrf HTTP 503; localhost:8080 connection refused; Go executable и выданные mTLS/runtime/test-session отсутствуют. Нужен изолированный работающий API/Auth/Core/ML с #47 + #48 (без merge/deploy этим агентом), затем actual browser import → POST/manual → forecast → payment lifecycle → new result/reload. Не передавать cookies/tokens в Git.
2. #48 остаётся draft/open, dependent backend не включён в этот PR. Не объявлять payment/reconciliation runtime рабочим по одним UI/adapter fixtures.
3. Forecast cancelled отсутствует в OpenAPI/private RPC. UI безопасно останавливает неизвестный terminal status; не выдуман cancellation endpoint. Нужен согласованный серверный контракт, если cancellation входит в дальнейшую приёмку.
4. Runtime categories/regularity analysis не подтверждён; frontend не синтезирует результаты. Probability остаётся unavailable/null, численное качество не заявлено.

Следующий точный шаг: проверить финальный source SHA/remote/CI #49; предоставить агенту C этот draft вместе с pinned #48 для реальной приёмки в отдельном integration checkout. Не менять чужую ветку и не включать backend commits в #49. Этапы 1–4 На ревью; независимые frontend gates/reviews выполнены, реальная часть этапа 5 заблокирована.
