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

Draft PR URL: **пока не создан — GitHub API не авторизован**.
Remote checkpoint SHA следует проверять через ls-remote; SHA текущего docs commit
не вставляется внутрь него самого. PR/CI updates должны ссылаться на фактический SHA.

## Состояние

| Этап | Статус | Tests | Review | CI |
| --- | --- | --- | --- | --- |
| A0 | В работе | Docs diff/links перед commit | Self-check; независимые pending | Не проверен, API auth отсутствует |
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

## Последние подтверждённые ограничения среды

1. `gh api user --jq .login` → `command not found: gh`.
   `gh` и GitHub token env отсутствуют, стандартный gh config не найден.
2. Sandbox `git fetch origin` → read-only `.git/FETCH_HEAD`; повтор с разрешённым
   escalated exec успешен, origin/main=fdee0ed. Git mutations могут требовать escalation.
3. Sandbox curl не разрешает DNS; escalated запрос
   `GET https://api.github.com/repos/benzocloud/hack-rnd-autumn-2026/pulls?state=open&per_page=100`
   → HTTP 404 без авторизации. Приватный API недоступен этой сессии.
4. `ssh -T -o BatchMode=yes git@github.com` → успешная GitHub identity
   `Prosteyshiyyy` (exit 1 штатен для GitHub SSH без shell).
5. Plugin discovery: GitHub connector доступен, но `installed=false`.
   Уже предлагался пользователю; повторную suggestion не отправлять.
   Нужен подключённый connector либо gh с авторизацией. Не извлекать browser cookies.
6. Доступен caveman-review; ponytail-review/golang-how-to в проверенных каталогах
   skills не найдены. Закреплённые источники — `.agent/infra/skills/skills.json`.
   Требуемое review пока не запускалось и успешным не объявлялось.

Это ограничения среды, не доказанные продуктовые дефекты. Первый обязательный
gate требует draft PR: A1 до него не начинать, часы неопубликованного кода не писать.

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

1. Восстановить авторизованный GitHub API (connector или gh); прочитать полные
   существующие PR discussions, исключить дубль draft и takeover.
2. Завершить A0: remote docs checkpoint, единственный draft, verify remote SHA/
   isDraft/URL, обновить PR и этот handoff; только затем A1.
3. A1: сопоставить account.checkContext/user locks и app-owned Tx с новым
   feature-local command store; общие fingerprint fixtures и additive migration,
   targeted real PostgreSQL concurrency/replay tests, ранний repository checkpoint.

Продолжать все A0–A5 по поручению, не запрашивать промежуточное разрешение на
согласованную работу. При недоступном первом gate сохранить честный checkpoint
и сообщить blocker, не объявлять задачу завершённой.
