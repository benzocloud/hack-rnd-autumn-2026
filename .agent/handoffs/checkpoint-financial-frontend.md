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
