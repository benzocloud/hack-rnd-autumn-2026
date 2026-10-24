# Company onboarding — продолжение работы

Обновлено 2026-10-24. Реализация в работе; owner review pending; не сливать.

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
| A1 | Не начат | Не запускались | Pending | Не запускался |
| A2 | Не начат | Не запускались | Pending | Не запускался |
| A3 | Не начат | Не запускались | Pending | Не запускался |
| A4 | Не начат | Не запускались | Pending | Не запускался |
| A5 | Не начат | Не запускались | Pending | Не запускался |

Есть только docs plan/handoff. Код, миграции, PKI/test topology и финансовые
fixtures не менялись. Runtime suite для docs-only slice не запускался.

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
Ревью кода последующих этапов ещё pending. Тот же pinned fetch можно повторить с другого
компьютера; root-owned installer на developer машине запускать не нужно.
Go `GOTOOLCHAIN=go1.27.3 go version` успешен; Docker server 29.9.1 доступен.
Go lint/task/Buf ещё предстоит подготовить; runtime suites пока не запускались.

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

Предстоящие gates (пока не запускались):

- Cwd `services/auth`: Go toolchain `go1.27.3`; golangci-lint `2.15.0`;
  `golangci-lint fmt --diff $(git ls-files -- '*.go')`,
  `golangci-lint run ./...`, `go vet -tags=integration ./...`,
  `go build ./...`, `go test -race -shuffle=on -count=1 -timeout=90s ./...`,
  `go test -race -tags=integration -shuffle=on -count=1 -timeout=90s ./...`.
  `AUTH_TEST_DATABASE_URL` — отдельная PostgreSQL 18 test DB с `_test` suffix,
  случайные disposable schemas по существующему integration helper;
  production/Core DSN не использовать. PostgreSQL CI pin 18.6, sqlx 1.4.0,
  pgx/stdlib 5.11.0, goose 3.29.0. Новая additive миграция ещё не создана.
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

PKI/local topology и config env для нового Auth→Core client пока не реализованы.
Перед A2/A5 взять существующие service TLS/testauth patterns и документировать
точные новые env names/isolated setup в Git. Секреты/ключи/cookies/.env/dumps
не выводить и не коммитить. Живые server/CA/firewall/SSH/чужие containers не менять.

## Следующие действия

1. A0 gate выполнен; продолжать в PR #41. Исторические descriptions/comments
   PR #16/#31 прочитаны; единственный повтор после network timeout — inline #31.
2. Подготовить pinned tools/отдельные test DB; runtime/product defects пока нет.
3. A1: сопоставить account.checkContext/user locks и app-owned Tx с новым
   feature-local command store; общие fingerprint fixtures и additive migration,
   targeted real PostgreSQL concurrency/replay tests, ранний repository checkpoint.

Продолжать все A0–A5 по поручению, не запрашивать промежуточное разрешение на
согласованную работу. При недоступном первом gate сохранить честный checkpoint
и сообщить blocker, не объявлять задачу завершённой.
