# Самостоятельное создание компании

## Поручение и режим работы

Прямое поручение владельца от 2026-10-24: реализовать путь register/verify/login
→ Workspace → создание компании → owner membership → чтение единственного
пустого default account. Issue не назначен, номер не выдумываем.

Разрешены последовательное выполнение A0–A5, проверки, независимые read-only
ревью, исправления, небольшие scope-only commits, обычные push и один draft PR.
Это явное исключение ADR-020: промежуточного согласования не требуется;
после проверок этапы остаются «На ревью владельца», только владелец принимает их.
Merge, ready-for-review, deployment и production provisioning запрещены.

Название PR: `feat: implement durable company onboarding`.
Владелец ветки: текущая Codex-сессия по поручению Prosteyshiyyy.
Ветка: `prosteyshiyyy/feat/company_onboarding` (nickname подтверждён GitHub SSH).
Исходная база: `fdee0ed`, свежий `origin/main` при старте.
Продолжение и ограничения: [handoff](../handoffs/company-onboarding.md).

## Проверенное исходное состояние

На базе `fdee0ed`:

- [Auth contract](../../docs/auth/contract.md) определяет durable command,
  fingerprint v1, finalization и recovery; RPC уже объявлены в
  [auth.proto](../../proto/auth/v1/auth.proto).
- `services/auth/internal/service/account/company.go` реализует authorization
  и membership listing, а `internal/handler/grpc/server.go` не реализует
  Begin/AuthorizeBootstrap/Complete (унаследованный Unimplemented).
- `services/core/internal/service/company/bootstrap.go` и `bootstrap_test.go`
  уже содержат bootstrap/replay/receipt; переиспользовать, не копировать в Auth.
- `services/api/internal/handler/http/financial.go` обслуживает GET companies,
  POST companies сейчас отклоняется методом; typed boundaries — `router.go`.
- `frontend/src/features/Workspace/Workspace.tsx` связывает companies/invitations
  через Promise.all, форма вызывает существующий companiesService и ведёт settings.
- GBrain recall выполнен; память вспомогательна, состояние подтверждается Git.
- PR/Issue descriptions и полные discussions пока недоступны: нет API auth.
  Нельзя считать проверенными отсутствие пересечений, CI или ревью.

Перед первым push fetch обнаружил новый main `ea433dd` (PR #40).
Он включён обычным merge: Workspace теперь использует независимые companies/
invitations resources и DataState. Эту часть A4 не дублировать; state refresh,
retry/redirect и реальный сценарий ещё предстоит проверить. Это стороннее
согласованное изменение main, не собственная реализация onboarding.

## Граница и неизменяемые условия

Каноническая цепочка: POST /companies → Auth.BeginCompanyCreation →
Core.CreateCompanyBootstrap → Auth.CompleteCompanyCreation → Core.ReadCompany → 201.

Auth владеет initiator/session/command/membership, Core — company/account/receipt.
Begin требует live session, verified email, CSRF; сохраняет UUIDv4 команды,
UUIDv7 компании и `(initiator, create_company, key)` ровно один раз.
Spec: trim name; INN null/absent → null либо 10/12 ASCII digits;
RUB, Europe/Moscow. Fingerprint: SHA-256 UTF-8 Go JSON struct полей
name,inn,currency,timezone, стандартный escaping, без LF. Общие fixtures с Core.
Повтор идентичен; иное тело с тем же ключом — conflict. Owner только из сессии.

Complete читает receipt по mTLS самостоятельно, проверяет command/company/spec/
hash/account/committed result; membership и COMPLETED записываются атомарно.
После сетевого вызова состояние перепроверяется под lock; SQL Tx/locks через
RPC не удерживаются. Disabled/deleted initiator или mismatched receipt →
QUARANTINED без membership. Finalization синхронизируется user lock.

Reconciler восстанавливает PREPARED после restart/потери ответа, без browser
cookie; сначала receipt, затем при отсутствии идемпотентный bootstrap и общий
finalizer. Lease/fencing, bounded retries/backoff и shutdown по текущим patterns.
Session expiry/reset не отменяет принятую команду. Unresolved keys/commands
не удаляются по произвольному TTL; completed mapping живёт с компанией.

Точные peers: Core-only AuthorizeCompanyBootstrap, Auth-only receipt read,
bootstrap только существующие API/Auth identities. App владеет Core connection,
Close и deadlines; недоступный/необязательный Core client не ломает login/verify.
sqlx/app-owned Tx, SQL/embed, private implementations, consumer-side ports,
feature-local DTO; никаких domain→service и repository→service зависимостей.

Сохранить server-only demo allowlist/strict default, пароль/session/CSRF,
consent version/digest/stale handling; регистрация содержит checkbox и ссылки,
без полного consent text. Новый account пуст: без balance/transactions/source
binding/coverage. Вне scope: imports, платежи, финансовые snapshots, категории/ML,
invitations, reset/email delivery, новые роли и полный redesign Auth.

Авторитетные источники также: [Core contract](../../docs/core/contract.md),
[API adapter](../../docs/core/api-adapter.md),
[REST/RPC map](../../docs/core/rest-rpc-map.json),
[OpenAPI](../../docs/api/openapi.yaml),
[company.proto](../../proto/core/v1/company.proto), актуальные код и тесты.
Архитектуру и accepted contracts не менять ради удобства: конфликт документировать
как blocker, продолжать только независимые согласованные части.

## Этапы

| Этап | Статус | Проверяемый результат |
| --- | --- | --- |
| A0 | На ревью владельца | Удалённые plan/handoff и один проверенный draft PR |
| A1 | На ревью владельца | Durable Auth command/repository и atomic owner finalization |
| A2 | На ревью владельца | RPC, Core client и recovery после restart |
| A3 | На ревью владельца | Настоящий POST /companies, conformance и actual HTTPS/mTLS/PG process gate |
| A4 | Не начат | Workspace без блокирующей зависимости от invitations |
| A5 | Не начат | Реальный полный сценарий, два ревью, CI, итоговый handoff |

### A0 — Контекст и публичный checkpoint

Зависимости: доступ Git и авторизованный GitHub API.
Компоненты: этот plan, handoff, один draft PR.

- [x] Прочитать AGENTS, instructions, decisions, workflows, Go references;
  GBrain recall; проверить исходный checkout, fetch origin и реальный nickname.
- [x] Создать отдельный worktree от свежего main, не stash/reset исходный checkout.
- [x] Прочитать PR/Issue descriptions, все conversation/review/inline comments;
  проверить существующий draft и исключить второго владельца ветки.
- [x] Проверить docs diff/links, docs commit, fetch/merge нового main, сразу push.
- [x] Создать/проверить единственный draft, таблицу этапов и ссылки plan/handoff;
  проверить remote SHA, доступность PR, сообщить URL; attach_artifact при наличии.

Gate: доступные remote документы и draft PR до реализации A1.
Если публикация/PR недоступны — точная причина и handoff, без долгой реализации.

### A1 — Durable command и repository

Зависимость: A0 gate. Компоненты: Auth feature service/storage, новая миграция,
repository SQL/embed/record, membership/user locks, общие fingerprint fixtures.
Ориентир: принятый cosmohack sqlx layout из Go references; исходники референсов
не копировать, наличие локального checkout перед адаптацией проверить отдельно.

- [x] Реализовать validation/spec/fingerprint и persistent scoped idempotency;
  states PREPARED/COMPLETED/QUARANTINED, constraints/indexes/locks.
- [x] Atomic membership + completion; concurrency/replay и rollback без утечки result.
- [x] Проверить additive migration на отдельной marked Auth test DB;
  совместимость/up/repeated-up/rollback по действующим правилам, старые SQL не менять.
- [x] Unit/real PostgreSQL/race, два read-only review repository slice.
- [x] Repository `599d068`, service/finalizer `9463e40` опубликованы; full Auth gates и два read-only reviews A1 пройдены. Remote после merge свежего main: `7f62b72`. Владелец ещё не принимал A1.

Критерий: одинаковый ключ даёт одну команду/company, конфликт без новой записи;
финализация атомарна и безопасна при конкуренции/disabled initiator.

### A2 — Auth RPC, Core client, recovery

Зависимость: A1. Компоненты: Auth handler/service/app/config/client/lifecycle,
canonical generated Core bindings только через generator; Core только совместимость.

- [x] Begin/Authorize/Complete и точные mTLS peer checks, typed errors (RPC/client slice).
- [x] Core client с app-owned connection/deadlines/Close и receipt validation.
- [x] Общий finalizer; сеть вне locks/Tx, перепроверка после сети.
- [x] Durable worker: receipt-first, bootstrap fallback, lease/fencing,
  bounded backoff/retries/shutdown, quarantine, без cookie и произвольного TTL.
- [x] Missing/mismatched receipt, lost reply, restart/rollback переходов,
  две реплики, disabled/deleted user, expired/reset session, quarantined command,
  unavailable dependencies и сохранённый login/verify.
- [x] RPC/client checkpoint `382fed6`; recovery `b2f882b` опубликован. Два read-only reviews обоих slices завершены, clock-skew defect исправлен и проверен.

Критерий: восстановление принятой PREPARED без браузера выдаёт ровно одну
membership, небезопасный receipt/инициатор не выдаёт доступ.

### A3 — Тонкий API adapter

Зависимость: A2. Компоненты: API handler/client/app, существующий Company mapping,
REST conformance/security tests; минимальные config examples/deploy templates.

- [x] POST orchestration по canonical chain, Core получает сохранённый Auth spec.
- [x] Typed clients, safe Russian errors/request ID, key conflict и 503/in-progress;
  201 только после COMPLETED и успешного read. Не дублировать Company domain.
- [x] OpenAPI conformance и adapter boundary regressions (201/409/503, Origin/CSRF, owner injection, revoked/malformed dependencies).
- [x] Реальный HTTPS API → mTLS Auth/Core → PostgreSQL: чужой command/peer/CSRF/revocation/timeouts/replay.
- [x] Полный API check и два reviews adapter slice пройдены; adapter49938a6 опубликован.
- [x] Реальная интеграция A3 и её отдельные Caveman/Ponytail reviews; checkpoint подготовлен к публикации.

### A4 — Workspace и пользовательский путь

Зависимость: A3. Компоненты: Workspace, companiesService, existing auth/company
context/request hooks; только блокирующая часть settings redirect.

- [ ] Проверить уже включённое из PR #40 разделение companies/invitations
  loading/error states; P1 честно недоступен, без fake success или реализации
  invitations. Повторную реализацию разделения не делать.
- [ ] После create обновить company/membership context; стабильный retry key,
  double click, lost response, refresh сохраняют ту же компанию.
- [ ] Проверить фактический redirect/settings unsupported endpoints;
  убрать только зависимость, блокирующую onboarding.
- [ ] Изолированные UI tests, test/typecheck/lint/build, настоящий браузер
  с financial mocks off; review и checkpoint.

### A5 — Интеграция и итоговая передача

Зависимость: A1–A4. Компоненты: воспроизводимая isolated test topology/fixtures,
security/failure E2E, plan/handoff/PR.

- [ ] Новая user через register/verify/login, без SQL membership;
  create → owner → company/default account read → reload/logout/login.
- [ ] Одна company/account/membership при double click, key replay,
  Core commit/lost reply/restart; чужой command/peer/CSRF/revoked access отказаны.
- [ ] Полные Go/contract/frontend gates, реальные marked Auth/Core test DB,
  два независимых SHA-bound caveman-review и ponytail-review;
  Go review с golang-how-to и нужными secondary skills; fixes и повторные checks.
- [ ] Проверить актуальный CI, pending/queued не считать passing;
  при доступном runner один /agent-review на финальный SHA.
- [ ] Финальный remote checkpoint, draft URL/head SHA, остающиеся дефекты,
  точные команды и следующий шаг владельца; всё остаётся на ревью владельца.

## Проверки и публикация

Команды/cwd, env и pinned tooling: [handoff](../handoffs/company-onboarding.md).
Skipped integration, недоступный CGO/race, отсутствующее ревью не равны успеху.
Для docs-only checkpoint runtime suite не нужен; проверяем diff/links/scope.

Каждый небольшой законченный slice: targeted checks → diff review → обновить
plan/handoff в том же commit → scope-only commit → fetch origin/main → обычный
merge свежего main → проверки затронутой интеграции → push → remote SHA → PR status.
Без force/rewrite; не переносить чужие незавершённые изменения.
При затянувшемся исследовании публиковать docs/handoff checkpoint; ориентир
20–30 минут активной работы. Честный WIP допустим с точными failures и next steps.
Не ждать CI ради сохранения следующего slice. При takeover остановить edits/push;
не создавать heartbeat/resume jobs. Секреты и приватные материалы не публиковать.

## Текущие риски и вопросы

- GitHub API auth восстановлен; draft [PR #41](https://github.com/benzocloud/hack-rnd-autumn-2026/pull/41) и remote head `6bd70b2` проверены. A0 gate выполнен.
- Pinned caveman-review/ponytail-review/golang-how-to получены из `.agent/infra/skills/skills.json`; A0 docs проверены двумя независимыми read-only reviewers без подтверждённых замечаний; code review A1–A5 ещё pending.
- Параллельные изменения Workspace/Core/API требуют проверки свежего main и PR;
  не дублировать уже сделанное. Исторический `5c5797e` не является pin.

A3 actual runtime поверх49938a6: -race process/PG security/replay/lostreply/restart PASS, full tag lint0. Core task check и real markedPG task integration/race PASS. Два независимых runtime reviews закрыты после readiness/PATH fixes; checkpoint публикуется. Браузер/A4 не объявлены завершёнными.

Свежий main0abae4f включается обычным merge; actualruntime/Core gates повторно PASS. API fullgate послеmain FAILcoverage83.14<85, unchangedmainbaseline82.48<85 подтверждён; thresholdнеизменён, см.handoff. Независимые A4/A5 продолжаются, готовность полногоgate не утверждается.
