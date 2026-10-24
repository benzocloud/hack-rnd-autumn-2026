# Company onboarding — продолжение работы

Обновлено 2026-10-25. Реализация в работе; owner review pending; не сливать.

## Авторизация и ownership

Прямое поручение владельца разрешает A0–A5, проверки, независимые read-only reviews,
исправления, commit/push и один draft PR без промежуточных подтверждений.
ADR-020 owner review отложен до общего результата; этапы после checks — «На ревью
владельца», не «Принят». Merge/ready-for-review/deploy/provisioning запрещены.
Полный scope/invariants/checklist: [plan](../plans/company-onboarding.md).
Issue не назначен. Не добавлять выдуманные Issue references.

Ветка: `prosteyshiyyy/feat/company_onboarding`.
Начальная база: `fdee0ed` из origin/main, получена git fetch 2026-10-24.
GitHub SSH подтвердил `Prosteyshiyyy`; исходный checkout на `prosteyshiyyy/goapi`
не изменялся. Владелец ветки — текущая Codex-сессия по этому поручению.
Явного takeover нет. Другому агенту продолжать ту же ветку, не cherry-pick и не
создавать второй PR/параллельного владельца без проверки.

Draft PR URL: https://github.com/benzocloud/hack-rnd-autumn-2026/pull/41 (draft, head `6bd70b2` проверен через GitHub API и ls-remote).
Remote checkpoint SHA следует проверять через ls-remote; SHA текущего docs commit
не вставляется внутрь него самого. PR/CI updates должны ссылаться на фактический SHA.

Первый docs commit `9f90a80` опубликован после обычного merge нового main
`ea433dd` (PR #40); push checkpoint `d9679eb` успешен. Ветка доступна:
[GitHub branch](https://github.com/benzocloud/hack-rnd-autumn-2026/tree/prosteyshiyyy/feat/company_onboarding).
Продолжать в существующем draft PR #41; новый PR не создавать.

## Состояние

| Этап | Статус | Tests | Review | CI |
| --- | --- | --- | --- | --- |
| A0 | На ревью владельца | Docs diff/links/stage markers/fences и staged diff --check прошли | Caveman/Ponytail A0: подтверждённых замечаний нет | Checks пусты для docs SHA `6bd70b2` |
| A1 | В работе: service/finalizer slice | Полный Auth unit/race + real PG integration/race/vet/build/lint прошли | Repository reviewed; service final reviews interrupted by usage limit | Последний проверенный CI — `1ffd947`, детали ниже |
| A2 | Не начат | Не запускались | Pending | Не запускался |
| A3 | Не начат | Не запускались | Pending | Не запускался |
| A4 | Не начат | Не запускались | Pending | Не запускался |
| A5 | Не начат | Не запускались | Pending | Не запускался |

A0 и A1 repository опубликованы. В текущем A1 service diff подключены command
store/app-owned Tx, Begin/Authorize business methods и Complete/common finalizer.
RPC/Core client/reconciler ещё не подключены, HTTP create отсутствует.
Финансовых fixtures нет. Runtime suite docs-only A0 не требовался; A1 проверен ниже.

Проверено на `fdee0ed` + docs diff:

- GBrain recall выполнен по проекту/onboarding; вывод — вспомогательный контекст.
- Auth RPC declarations есть, implementation Begin/AuthorizeBootstrap/Complete
  в handler/service нет. Existing login/verify/membership auth сохранять.
- Core bootstrap/receipt/service tests уже есть, переиспользовать их.
- API financialRoute выбирает GET для /companies; POST сейчас 405.
- Workspace Promise.all связывает companies и unsupported invitations;
  create использует companiesService, затем settings redirect.
- Fresh main включает PR #39 stale-company-selection; исходная историческая
  база `5c5797e` уже не самая свежая.
- Ни CI, ни reviews/PR discussions не проверены: Git SSH не даёт GitHub API.

После initial self-check выполнен новый fetch: main=`ea433dd`, PR #40 уже
разделил companies/invitations resources и их DataState. Обычный merge прошёл
без конфликтов; изучен полный Workspace diff, своих runtime changes нет.
Текущий diff относительно main содержит только два onboarding Markdown-файла.
Не дублировать эту часть A4; UI tests/typecheck/browser для чужого изменения
здесь не запускались и не считаются прошедшими.

## Среда и восстановление доступа

GitHub API auth восстановлен пользователем: `/usr/bin/gh`, `gh api user --jq .login`
→ `Prosteyshiyyy`; список открытых PR до создания #41 пуст. PR #41 draft создан
и проверен, SHA `6bd70b23e65fd924c6c2c5a76b26281972272496` совпал с remote.
Все доступные descriptions/conversation/reviews/inline replies с пагинацией
прочитаны для связанных PR #17/#25/#30/#34–#40; Issue не назначен.
Исторические ошибки `gh not found` и API 404 устранены; не повторять диагностику
без новой причины. Git mutations/network требуют sandbox escalation.

Pinned review skills получены в изолированном каталоге из
`.agent/infra/skills/skills.json`: caveman-review `bad48a0`, ponytail-review
`09c434a`, cc-skills-golang `19a0626`. Чтение skill не равно проведённому review;
A0 reviewed двумя независимыми read-only агентами: caveman-review и
ponytail-review, подтверждённых замечаний нет (HEAD `6bd70b2` + A0 docs diff).
Repository slice A1 также reviewed; service/RPC/recovery reviews ещё pending. Тот же pinned fetch можно повторить с другого
компьютера; root-owned installer на developer машине запускать не нужно.
Go `GOTOOLCHAIN=go1.27.3 go version` успешен; Docker server 29.9.1 доступен.
Go lint 2.15.0 подготовлен, A1 runtime результаты ниже; task/Buf ещё предстоит подготовить.

## Воспроизводимые команды

С другого компьютера с настроенными Git SSH и авторизованным gh/connector:

```sh
git fetch origin
git switch --track origin/prosteyshiyyy/feat/company_onboarding
cat .agent/handoffs/company-onboarding.md
cat .agent/plans/company-onboarding.md
git status --short
git ls-remote origin refs/heads/prosteyshiyyy/feat/company_onboarding
```

Если ветка уже локальная, использовать `git switch prosteyshiyyy/feat/company_onboarding`;
для параллельного checkout — отдельный worktree, не stash/reset чужого.
Проверить все существующие PR, выбрать прежний draft при наличии.

До нового push: fetch origin/main и обычный merge в эту ветку, без force/rewrite.
PR title: `feat: implement durable company onboarding`; русское description:
«Реализация в работе; owner review pending; не сливать», таблица A0–A5 и ссылки
на plan/handoff в этой ветке. Проверить `isDraft`, head SHA и доступность URL.
Read full PR descriptions/conversation/reviews/inline replies с пагинацией;
linked Issues читать только если действительно связаны, номер не выдумывать.

Полный набор gates; фактически выполненные проверки и ограничения A1 указаны ниже:

- Cwd `services/auth`: Go toolchain `go1.27.3`; golangci-lint `2.15.0`;
  `golangci-lint fmt --diff $(git ls-files -- '*.go')`,
  `golangci-lint run ./...`, `go vet -tags=integration ./...`,
  `go build ./...`, `go test -race -shuffle=on -count=1 -timeout=90s ./...`,
  `go test -race -tags=integration -shuffle=on -count=1 -timeout=90s ./...`.
  `AUTH_TEST_DATABASE_URL` — отдельная PostgreSQL 18 test DB с `_test` suffix,
  случайные disposable schemas по существующему integration helper;
  production/Core DSN не использовать. PostgreSQL CI pin 18.6, sqlx 1.4.0,
  pgx/stdlib 5.11.0, goose 3.29.0. Additive миграция 3 создана и проверена, подробности в A1-разделе.
- Cwd `services/core`: `task check`, `task integration`; предварительно
  подготовить отдельную marked Core test DB по README/current integration helper.
  Не подменять skipped integration успешной проверкой.
- Cwd `services/api`: `bash scripts/check.sh` (generation/build/unit/race/
  coverage/vet/fmt/lint/data contracts/smoke/conformance).
- Canonical Buf lint/build/breaking, drift/generation/consumer import checks
  по актуальным scripts/configs при изменениях bindings. Generated code вручную
  не редактировать; base для breaking выбрать из свежего main.
- Cwd `frontend`: `npm ci`, `npm test`, `npm run typecheck`, `npm run lint`,
  `npm run build`; Node >=20.9.0, точные npm dependencies из package-lock.json.
  Настоящий браузер, financial mocks off, HTTPS API/mTLS services/real PostgreSQL.

Изолированный Auth test DB fixture реализован и описан ниже; mTLS topology и
config env для нового Auth→Core client пока не реализованы.
Перед A2/A5 взять существующие service TLS/testauth patterns и документировать
точные новые env names/isolated setup в Git. Секреты/ключи/cookies/.env/dumps
не выводить и не коммитить. Живые server/CA/firewall/SSH/чужие containers не менять.

## Следующие действия

1. Завершить два независимых review A1 service/finalizer diff: оба были прерваны
   usage limit, итоговый успех не заявлен. Проверенный parent — `1ffd947` + service diff.
2. Опубликовать service checkpoint и проверить remote SHA/CI; при review findings
   исправить, повторить относящиеся checks и сразу push.
3. A2: реальные Auth RPC/Core client/generated bindings, app lifecycle и durable
   reconciler. Buf ещё не установлен: предыдущий approval review завершился quota error.

Продолжать все A0–A5 по поручению, не запрашивать промежуточное разрешение на
согласованную работу. При недоступном первом gate сохранить честный checkpoint
и сообщить blocker, не объявлять задачу завершённой.

## A1 — repository checkpoint (2026-10-24)

Проверенный снимок: `0bde7e61b7b5c0fda152875e55da624898518b7e` + A1 storage diff.
Новая additive миграция `00003_company_creation.sql`; schema version 3, старые
миграции не изменены. Команда UUIDv4, company UUIDv7, scoped key, normalized spec/
fingerprint, PREPARED/COMPLETED/QUARANTINED, account/commit result и DB-time
lease/backoff metadata. Нет каскадного удаления команды при удалении initiator.
Новый command repository принимает DB/Tx, SQL/embed/record приватны; InsertOwner
не повышает financier и не заменяет другого owner. Общие Auth/Core fixtures —
`testdata/company-bootstrap-fingerprint-v1.json`, оба CI реагируют на их изменения.

Пройдено (в cwd services/auth, GOTOOLCHAIN=go1.27.3):

- `go test -race -shuffle=on -count=1 -timeout=90s ./...`.
- `go test -race -tags=integration -shuffle=on -count=1 -timeout=90s ./...`:
  настоящая marked PostgreSQL18.6, все integration прошли, не skipped.
- `go vet -tags=integration ./...`, `go build ./...`.
- `golangci-lint fmt ./...`, `golangci-lint run ./...`: 0 issues.
- Дополнительный focused `./integration -run '^TestCompanyCreation'` с race/shuffle
  проверяет owner uniqueness/role preservation после дополнения regression.
- Cwd services/core: полный `golangci-lint fmt --diff ./...`, полный
  `golangci-lint run ./...` (0 issues), `go test -race ./internal/domain/company`.
  Полный Core runtime/PG gate — на A2/final, Core runtime здесь не менялся.

Два независимых read-only reviewers (pinned caveman/ponytail + golang-how-to,
database/error-handling/security/testing) нашли один blocker: nullable account_id
CHECK запрещал PREPARED INSERT. Исправлен до публикации, реальные integration
повторно прошли; других подтверждённых замечаний repository slice не осталось.
Это не приёмка A1 целиком: production service finalizer ещё отсутствует.

Воспроизведение isolated DB из корня repo (без production/чужих контейнеров):

```sh
docker run --rm --name saldo-onboarding-test-pg -e POSTGRES_USER=auth_test -e POSTGRES_DB=auth_test -e POSTGRES_HOST_AUTH_METHOD=trust -p 127.0.0.1::5432 -d postgres:18.6@sha256:edb0c08dc33befd0262ab3b5cf1c4f3f8cfd8b09e88c05b45adee29d6761ddb6
docker port saldo-onboarding-test-pg 5432
docker exec saldo-onboarding-test-pg psql -U auth_test -d auth_test -c "ALTER DATABASE auth_test SET saldo.test_database = 'saldo-auth-integration';"
```

Задать AUTH_TEST_DATABASE_URL на loopback port из docker port, user auth_test,
database auth_test, sslmode=disable (это disposable trust fixture, не credentials
живой БД). Helper создаёт/удаляет только собственные random auth_test_* schemas.
Проверено 2→3→2→3 и повторный up с сохранением существующего user; rollback
schema 3 удаляет command table, использовать только в isolated test DB.
Удаление своего fixture после проверок: `docker stop saldo-onboarding-test-pg`;
`--rm` убирает только этот container, persistent volumes не создавались.

Tooling воспроизводится без изменения go.mod:
`GOTOOLCHAIN=go1.27.3 GOBIN=/tmp/saldo-onboarding-tools go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0`.
Добавить этот каталог в PATH либо вызвать установленный binary по пути.
Текущий worker/PKI/client config не создан; runtime CI надо читать после push,
исторические green checks не доказательство нового SHA.

## A1 — service/finalizer checkpoint (2026-10-25)

Снимок: `1ffd9473d7f32f82262710e5751df4f2c901ea34` + service/app/tests diff.
account.Work расширен feature-local command store; app.New связывает repository
с тем же sqlx.Tx. Begin использует live/verified session/CSRF, user→session→command
locks, UUIDv4/UUIDv7 создаются только после scoped key lookup. CreatedAt усечён
до PostgreSQL microseconds, чтобы первый ответ совпадал с durable replay.
Authorize bootstrap проверяет durable command и verified/enabled initiator;
чужой input не карантинит корректную команду. Complete читает Core через
consumer-side port после commit первой Tx; общий finalizer повторно берёт
user→command locks, сверяет receipt и атомарно пишет owner+COMPLETED.
Disabled/deleted user и mismatch дают committed QUARANTINED без membership;
completed replay не восстанавливает удалённую membership.

Core port пока тестовый boundary, не реальный mTLS client; business methods
не зарегистрированы как RPC. Это не доказательство полного onboarding/E2E.
Существующие login/verify не требуют Core клиента и прошли повторный regression.

Проверено на этом diff в services/auth, pinned Go1.27.3:

- Полный `go test -race -shuffle=on -count=1 -timeout=90s ./...` — exit0.
- Полный `go test -race -tags=integration -shuffle=on -count=1 -timeout=90s ./...`
  на том же отдельном marked PostgreSQL18.6 fixture — exit0, не skipped.
- `go vet -tags=integration ./...`, `go build ./...`, full-module
  `golangci-lint run ./...` — exit0, lint0issues; formatting/import применены.
- Реальные две service replicas/pools: concurrent Begin даёт один command,
  same key/different spec conflict, missing receipt оставляет PREPARED;
  fresh service/pool после restart завершает owner, completed replay сохраняет
  отзыв доступа. SQL NOWAIT regression подтверждает отсутствие user lock через RPC.
- Disabled user во время receipt read даёт durable QUARANTINED/no membership.
  Unit cases дополнительно покрывают mismatch всех receipt fields, invalid/missing
  account/commit, deleted user, foreign command, CSRF/expired session и rollback.

Два service reviewers были запущены, но оба завершились usage limit до финального
отчёта. Ponytail успел сообщить предварительное отсутствие доказанных дефектов;
это не завершённое review. Repository reviews выше действуют только на старый slice.

Последний прочитанный GitHub CI (`1ffd947`): Auth lint/tests, Core contracts/lint,
infra safety/ml-tests — SUCCESS; Auth build и Core tests — IN_PROGRESS, общий успех
не заявлен. Источники: Auth run40132488413, Core run40132488424, infra run40132488402.

Подтверждённый environment failure: установка Buf1.75.0 через pinned go install
не запущена — automatic approval reviewer сообщил usage limit и retry at12:50AM.
Это сбой проверки разрешения, не признание команды небезопасной. Не обходить
reviewer и не выдавать Buf установленным; восстановить quota/approval service,
затем повторить обычную разрешённую проверку. Последний remote checkpoint до
service публикации — `1ffd947`; весь service diff пока локален, пока новый push
и remote SHA не подтверждены.
