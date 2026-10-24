# Поручение удалённому агенту: самостоятельное создание компании

Это готовый текст поручения. Передай **всё содержимое файла** агенту в другом
чате/на компьютере разработчика. Наличие файла здесь не запускает работу и не
означает, что другой разработчик уже его получил. Поручение самодостаточно:
не требует доступа к нашему неопубликованному общему плану или истории чата.

---

Реализуй самостоятельное создание компании в Saldo в репозитории
`https://github.com/benzocloud/hack-rnd-autumn-2026`.

Я буду недоступен. Разрешаю в границах этого поручения непрерывно выполнять все
этапы ниже, запускать проверки и независимые ревью, исправлять подтверждённые
замечания, коммитить, пушить и создавать/обновлять **один draft PR** без моего
промежуточного подтверждения. Это явное исключение ADR-020: ревью владельца
отложено до общего результата; этапы после твоих проверок остаются «На ревью»,
а не «Принят». Не останавливайся после каждого этапа с вопросом «продолжать?».
Merge, ready-for-review и deployment не разрешены. Main автоматически
развёртывает релевантные изменения, поэтому PR оставляй draft и не сливай.

Главный организационный результат: **вся полезная работа регулярно находится
на GitHub**, так что другие разработчики могут продолжить её без моего
компьютера и без чтения этого чата. Не накапливай всю реализацию до одного
финального push. Не рассчитывай, что перед лимитом/сбоем успеешь написать отчёт.

## 1. Цель и граница

Пользователь регистрируется/подтверждает почту существующим способом, входит,
создаёт компанию из Workspace и получает owner-доступ к единственному новому
счёту без операторской SQL-вставки membership.

Каноническая цепочка:

`POST /companies → Auth.BeginCompanyCreation → Core.CreateCompanyBootstrap
→ Auth.CompleteCompanyCreation → Core.ReadCompany → HTTP 201`.

В Auth также нужен Core-only `AuthorizeCompanyBootstrap` и durable reconciler
PREPARED-команд после restart/потерянного ответа. Core bootstrap/receipt уже
реализованы; сначала проверь свежий код и переиспользуй их.

Работа охватывает Auth, только необходимую совместимость существующего Core,
тонкий API adapter, Workspace и настоящую интеграционную проверку.
Другие разработчики параллельно занимаются импортом/финансами/ML — их изменения
сохраняй, не дублируй и не переноси их незавершённые правки в свою ветку.

Не реализуй imports, платежи, финансовые snapshots/coverage, категории/ML,
приглашения, reset/email delivery, расширение ролей или полный redesign Auth.
Не меняй accepted контракты/ADR ради удобства. При существенном противоречии
зафиксируй конкретный blocker, продолжи независимые согласованные части;
не придумывай новые архитектурные решения за спящего владельца.

## 2. Первое действие — публично доступный checkpoint

1. Прочитай `AGENTS.md`, `.agent/instructions.md`, `.agent/decisions.md`,
   `.agent/workflows/{local-development,issue-planning,git-conventions,automated-review}.md`,
   `.agent/references/go-services.md`. Выполни GBrain recall по проекту/теме;
   при недоступности назови причину и продолжай по источникам.
2. Проверь status/ветки/PR, fetch origin. Используй отдельный worktree от свежего
   origin/main, не stash/reset чужой checkout. Ветка —
   `<проверенный GitHub nickname>/feat/company_onboarding`; nickname не выдумывай.
   Если соответствующий draft уже существует, после проверки продолжи его,
   а не создавай дубликат и не запускай параллельно второго владельца ветки.
3. Прочитай актуальные source/contracts и полные описания/комментарии связанных
   PR/Issue, если они есть. Назначенный Issue этим поручением не задан — не выдумывай.
   Историческая база поручения — main `5c5797e` 24.10.2026, но это не pin для старта:
   используй более свежий main и перепроверь реализацию. Не копируй уже сделанное.
4. Создай собственный план `.agent/plans/company-onboarding.md` со всеми этапами
   ниже и `.agent/handoffs/company-onboarding.md` с текущим состоянием.
   Запиши туда границу, авторизацию этого поручения и режим итогового ревью.
5. Проверь эти документы, сделай небольшой docs-коммит, **сразу push и draft PR**
   с английским названием `feat: implement durable company onboarding`.
   Описание/статус/комментарии PR пиши по-русски. В description сразу укажи:
   «Реализация в работе; owner review pending; не сливать», таблицу этапов
   и ссылки на plan/handoff в текущей ветке.
6. Проверь remote HEAD и доступность PR, сообщи его URL в чате. Если есть инструмент
   attach_artifact, прикрепи PR. Это первый обязательный gate до длинной реализации.
   Если push/создание PR недоступны, сообщи точную причину и не уходи в часы
   неопубликованной работы. Не выдавай локальный коммит за сохранение на GitHub.

Весь дальнейший результат — в этом одном draft PR; свежий main не переписывается.

## 3. Источники и неизменяемые условия

Авторитетные документы: `docs/auth/contract.md` (раздел создания компании и
reconciler), `docs/core/{contract,api-adapter}.md`, `docs/core/rest-rpc-map.json`,
`docs/api/openapi.yaml`, `proto/auth/v1/auth.proto`, `proto/core/v1/company.proto`.
Обязательно прочитай соответствующий код и тесты, не только документацию.

Проверенные исходные точки:

- Auth `internal/service/account/`, `internal/repository/{membership,user,postgres}/`,
  `internal/handler/grpc/`, `internal/app/` — минимальный Auth уже работает,
  бизнес-RPC создания компании на исходной базе ещё нет.
- Core `internal/service/company/`, `internal/handler/grpc/company/`,
  `internal/client/auth/`, company repository/migrations — существующие
  bootstrap/profile/receipt. Не копировать Core domain в Auth.
- API `internal/handler/http/{router,financial}.go`, `internal/{app,client}/`,
  `frontend/src/service/companies_service/companies_service.ts`.
- UI `frontend/src/features/Workspace/Workspace.tsx` — форма создания уже есть,
  но companies и отсутствующие invitations связаны одним Promise.all.

Соблюдай существующий контракт:

- Begin: live session, verified email, CSRF; стабильная команда UUIDv4 и
  company UUIDv7, нормализованный spec, fingerprint и scoped Idempotency-Key
  сохраняются один раз. Повтор идентичен; иное тело под тем же ключом — conflict.
  Owner определяется из сессии, не из browser role/user ID.
- Fingerprint v1 по `docs/auth/contract.md` должен побайтно совпадать с Core:
  JSON name,inn,currency,timezone; SHA-256; null/absent inn и escaping не перепутать.
  Тестировать общими fixtures, включая кириллицу/escaping/отсутствующий ИНН.
- Core получает **сохранённый Auth spec**, подтверждает command через Core-only
  Auth RPC, атомарно создаёт company/default account/receipt с теми же IDs.
- Complete сам читает Core receipt по проверенному mTLS и сверяет
  command/company/spec/hash/default account/committed result; затем атомарно
  создаёт owner membership и COMPLETED. Нет boolean «Core succeeded» от API.
  HTTP 201 только после завершения, timeout — не фиктивный успех.
- Reconciler восстанавливает принятые PREPARED без browser cookie: receipt read,
  при отсутствии идемпотентный bootstrap, общий finalizer. Бounded retries/backoff,
  lease/concurrent worker fencing и shutdown — по существующим patterns, без
  нового брокера или универсального orchestration framework.
- Expired/reset session не отменяет уже принятую команду; новые browser requests
  требуют live session. Disabled/deleted initiator или несовпавший receipt —
  QUARANTINED без membership. Finalization синхронизируется user lock.
  Не истекать unresolved команды/key mappings по произвольному TTL.
- Новый счёт пустой: не добавлять manual balance/transactions/source binding,
  fake coverage или финансовые fixtures в рабочий onboarding.
- Сохранять existing server-only demo allowlist/strict default, пароль/session/
  CSRF, consent version/digest/stale handling. В регистрации только checkbox
  и ссылки consent/privacy, без полного текста документа.
- sqlx/app-owned pool/Tx, SQL в .sql/embed, private implementations и
  consumer-side ports, feature-local DTO. Никаких domain→service зависимостей,
  service команд в repository или копирования чужих internal пакетов.
- mTLS identity: Core-only AuthorizeCompanyBootstrap; Auth-only receipt read;
  bootstrap только существующие API/Auth peers. Не расширять peers до wildcard.
  Auth Core client конфигурируется в app, владеющей connection/Close/deadlines;
  отсутствие/недоступность клиента не ломает работающий login/verify.

## 4. Этапы — выполняй последовательно до общего результата

**A0. Контекст, локальный план и первый draft PR.** Уже описан выше.
Критерий: удалённые plan/handoff/PR доступны, проверен remote HEAD.

**A1. Durable Auth command и repository.**
Нужная миграция, feature-local storage/SQL/record, spec validation/fingerprint,
idempotency/concurrency, PREPARED/COMPLETED/QUARANTINED, atomic finalization
membership. Новые индексы/locks и совместимость миграций проверены на реальной
отдельной marked Auth test DB. Не меняй существующие migration задним числом.

**A2. Auth RPC, Core client и recovery.**
Begin/Authorize/Complete, exact peer checks, проверенный Core client, переиспользование
existing Core bootstrap/receipt, reconciler/lifecycle. Не держать SQL locks/Tx
во время исходящего RPC; после сети повторно проверить состояние перед commit.
Тесты missing/mismatched receipt, lost reply, restart на каждом переходе,
две реплики, disabled user, истёкшая сессия, quarantined command и зависимости.

**A3. API вертикальный сценарий.**
Реальный `POST /companies`, typed clients/mapping/error codes; идемпотентная
оркестрация по принятому контракту, безопасные русские сообщения и request ID,
201/503/conflict semantics. Никаких дублирующих Company-моделей в API.
Conformance и реальный HTTPS API→mTLS Auth/Core→PostgreSQL.

**A4. Workspace и путь пользователя.**
Используй существующие форму/companiesService; отключи зависимость P0 companies
от P1 invitations — отдельное состояние/честная недоступность, без fake success.
После создания обновляются company/membership context; refresh и повтор после
потерянного ответа восстанавливают ту же компанию. Проверь actual redirect:
если settings/company page запрашивает неподдержанные P1 endpoints, устрани
только зависимость, блокирующую этот путь, не реализовывай весь settings/P1.
Изолированные UI tests и настоящий браузер с financial mocks выключенными.

**A5. Интеграция, ревью и финальный handoff.**
Из новой test user без SQL membership: existing register/verify/login → create
company → active owner membership → company/default account read → reload/logout/login.
Одна company/account/membership при double click, повторе ключа, Core commit
с потерянным ответом и restart. Чужой command, неверный peer, CSRF и revoked
доступ отказаны. Два независимых ревью, исправления, полный итоговый gate и CI.
Не заявляй готовность импорта/ML/всего MVP и не запускай production provisioning.

Каждый этап после реализации/проверок/ревью фиксируй «На ревью владельца» и
переходи к следующему без ожидания ответа. Принятие владельцем не имитировать.
Технический blocker одной проверки не повод молча бросать остальные независимые
работы; но небезопасный переход/неясную архитектуру не обходить догадкой.

## 5. Публикация после каждой законченной части

1. Делай маленькие проверяемые slices внутри этапов. После каждого законченного
   slice: targeted checks, diff review, обновление plan/handoff, scope-only commit,
   **сразу push в ту же ветку**, проверка remote SHA и актуализация draft PR.
   Не жди полного A1/A2, большого E2E или финального ревью для первой публикации.
   В частности repository, RPC/client, recovery и API/UI — отдельные checkpoint-ы.
2. Перед каждым разрешённым push fetch origin/main; включи свежий main обычным
   merge без rewrite опубликованной истории. Разрешай конфликты по смыслу,
   сохраняя чужие изменения; повторяй проверки затронутой интеграции. Если
   пересечение требует нового решения, запиши blocker в PR, не теряй уже
   опубликованные checkpoint-ы. Никогда force push/reset/clean чужой работы.
3. На окончании содержательного этапа проведи отдельные read-only
   caveman-review и ponytail-review, желательно двумя независимыми агентами;
   для Go примени golang-how-to и нужные secondary skills. Reviewer не коммитит.
   Подтверждённые замечания исправляй, повторно проверяй и сразу публикуй fix.
   Не называть отсутствующее/непроведённое ревью успешным; pending записать явно.
4. Не жди полного CI перед сохранением следующего готового slice в draft:
   фиксируй current CI status, проверяй новые failures и приоритетно исправляй
   свои регрессии. In-progress/queued — не passing. Длительный CI не должен
   оставлять часы готовой работы только на твоём компьютере.
5. Не реже одного небольшого рабочего slice делай checkpoint; если исследование
   или интеграция затягиваются, публикуй промежуточный docs/handoff с находками.
   Практический ориентир — обновление GitHub примерно каждые 20–30 минут
   активной работы, а не скрытый долгий цикл до конца задачи. Это ориентир,
   а не обещание фонового таймера или гарантированного финального вызова.
6. Допускается честно помеченный **WIP checkpoint** незавершённого своего кода
   в draft, если полезная работа иначе остаётся только локально. Проверить
   состав diff на секреты/посторонние файлы, сохранить всё своё для подхвата,
   записать точно, что не собирается/падает и почему. Не заявлять WIP проверенным.
   Не маскировать ошибки через skip/blanket exclusions и не пушить чужие изменения.

Комментарии/описание PR входят в разрешённую публикацию. При наличии рабочего
автоматического reviewer можно отправить `/agent-review` на финальный SHA;
push сам его не запускает. Не запускать повторный server review на каждый WIP.
Указывать head SHA и не считать stale review подтверждением нового diff.

## 6. Handoff, который позволяет продолжить с другого компьютера

`.agent/handoffs/company-onboarding.md` обновляется **в том же commit, что и
изменение кода**, и пушится вместе с ним. Там постоянно должны быть:

- scope/разрешённые действия, branch/base, URL draft PR, стадия и краткий результат;
- таблица A0–A5: не начат/в работе/на ревью; отдельные tests/review/CI состояния;
- что реализовано и что ещё отсутствует, существенные выбранные решения;
- точные команды запуска/генерации/проверок, cwd, pinned tooling и необходимые
  env **имена без значений**; нет ссылок только на локальный /tmp/desktop файл;
- для каждой проверки результат/ограничение и идентичность проверенного кода
  (commit SHA либо base SHA + проверяемый diff); ссылки на CI/review конкретного SHA;
- последние подтверждённые failures: команда, короткая очищенная ошибка,
  место причины, что уже проверялось и не надо повторять вслепую;
- **следующие 1–3 конкретных действия** с файлами/символами и проверкой завершения;
- миграции, способы подготовить отдельные test DB/PKI и воспроизводимые fixtures;
  всё необходимое воспроизводится из Git или документированных входов;
- незавершённые изменения и known blockers; отличить ошибку продукта от среды;
- кто сейчас владеет веткой. PR handoff по умолчанию для продолжения в той же
  ветке; не cherry-pick наугад и не создавать второй PR той же работы.

После push проверь remote HEAD и в обновлении PR укажи фактический SHA:
не пытайся вписать SHA самого текущего commit внутрь этого commit.
README/PR должен позволять новому агенту без истории чата выполнить:
`fetch → checkout PR branch → read handoff → reproduce checks → continue next step`.

Если есть признаки приближающегося лимита/неустойчивой среды, сначала checkpoint
и push, затем новые эксперименты. Если лимит/сбой пришёл внезапно, последний
remote checkpoint уже должен содержать код и понятное следующее действие.
Если push заблокирован, явно сообщи remote SHA и перечень только локальной работы;
не делай вид, что она сохранена. Удалённый компьютер не является нашим backup.

При явном takeover другим разработчиком прекрати изменения/push в эту ветку,
зафиксируй передачу; не возобновляй конкурентную работу после сброса лимита
без нового согласования. Никаких автоматических resume/heartbeat задач не создавай.

## 7. Проверки, результат и запреты

Go: pinned module toolchain, fmt/import/lint по всему затронутому модулю,
unit/race/vet/build; Auth и Core real PostgreSQL integration, concurrency/replay,
security/peer/timeout/restart. API `bash scripts/check.sh` из `services/api/`;
Core `task check` и `task integration`; Auth команды из README/current CI.
Реальные отдельные test DB с предусмотренным marker, не production БД.
Не считать skipped integration/race отсутствующего CGO успешными.

Контракты: canonical Buf lint/build/breaking и drift/generation/consumer import
checks при необходимости; generated bindings не редактировать вручную.
Frontend команды только из `frontend/`: npm test/typecheck/lint/build,
реальный браузер и HTTPS/mTLS chain. При изменении только docs не запускать
весь runtime suite без причины; на code slices targeted, на stage/final full gates.

Новый Auth→Core client может потребовать конфигурацию; примеры/тесты/минимальные
репозиторные deploy templates обновить можно в пределах onboarding. Не менять
живой сервер/CA/secrets/firewall/SSH/permissions и не запускать deployment.
Документируй необходимые будущие operator действия, не выполняя их.
Секреты, приватные ключи, cookies, DB dumps, локальные .env и credentials не коммитить
и не показывать. Не трогать Fadwix/GBrain/Grafana/чужие containers/worktrees.

В финале: draft PR URL/head SHA, выполненные этапы, реальные checks/CI/review,
незакрытые дефекты и следующий шаг. PR остаётся draft на ревью владельца,
даже если все тесты зелёные. Если не закончил — тот же отчёт по текущему checkpoint;
важнее сохранить честный доступный результат, чем написать «всё готово».
