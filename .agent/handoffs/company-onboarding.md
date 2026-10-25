# Company onboarding — продолжение работы

Обновлено 2026-10-25. Реализация в работе; owner review pending; не сливать.

## Поручение, граница и ownership

Прямое поручение владельца разрешает A0–A5, проверки, независимые read-only reviews,
исправления, небольшие commits/push и один draft PR без промежуточных подтверждений.
Исключение ADR-020: промежуточные этапы остаются «На ревью владельца», не «Принят».
Merge, ready-for-review, deployment и production provisioning запрещены.
Scope/invariants/checklist: [plan](../plans/company-onboarding.md). Issue не назначен.

Ветка: `prosteyshiyyy/feat/company_onboarding`, nickname подтверждён GitHub SSH/API.
Владелец ветки — текущая Codex-сессия по поручению Prosteyshiyyy; takeover нет.
Продолжать ту же ветку, не cherry-pick и не создавать второй PR/параллельного owner.
[Draft PR #41](https://github.com/benzocloud/hack-rnd-autumn-2026/pull/41).
Исходный checkout `prosteyshiyyy/goapi` не изменялся; отдельный worktree.
Начальная база `fdee0ed`; main обновляется обычным merge перед каждым push.
Последний включённый main `0f0ff73`, опубликованный recovery head:
`b2f882b5d28e637cb35215246ac83bf1ab7ea7f5` (remote API ref проверен).

## Состояние A0–A5

| Этап | Статус | Tests | Review | CI |
| --- | --- | --- | --- | --- |
| A0 | На ревью владельца | Docs links/diff/fences, draft/remote SHA | Caveman/Ponytail без дефектов | Docs-only, runtime не требовался |
| A1 | На ревью владельца | Полный Auth unit/real PG integration/race/shuffle/vet/build/lint | Два independent reviews `9463e40`, дефектов нет | Auth/Core gates `7f62b72` SUCCESS |
| A2 | На ревью владельца: recovery готов к push | Полный Auth gate, real PG recovery/race, TLS/Buf/drift | Два reviews RPC/client и recovery; clock-skew fix проверен | Новый recovery SHA требует CI |
| A3 | В работе: живая цепочка прогнана локально | Binaries HTTPS/mTLS/Postgres: owner, empty account, replay, CSRF, foreign user, Auth restart. Peer/revocation/timeout/lost reply pending | Adapter reviews stand; this run is not reviewed | Новый SHA ещё не опубликован |
| A4 | Не начат | PR40/43 из main сохранены; onboarding UI ещё не проверен | Pending | Pending |
| A5 | Не начат | Нет полного register→owner→account→reload browser E2E | Pending | Pending |

HTTP create adapter реализован в текущем diff. Durable worker опубликован;
Core в service integration пока boundary fixture, полный Core/API/browser E2E
остаётся A3/A5 и ещё не объявляется пройденным.
Продукт/импорт/ML/MVP готовыми не объявлять.

## Что реализовано и чем подтверждено

A1 repository `599d068`, service `9463e409af668dc13cc9930c1ecbf8785815efa4`.
Новая additive migration3 (старые migrations не менялись), schema version3.
Auth-owned UUIDv4 command/UUIDv7 company, scoped key/spec/hash, states
PREPARED/COMPLETED/QUARANTINED, receipt account/commit и DB-time lease/fencing/backoff
metadata. Initiator UUID сохраняется после удаления user, без cascading FK/TTL.
SQL/embed/record, private repositories и app-owned sqlx pool/Tx сохранены.

Begin использует существующий checkContext: user→session→command, live/verified
session/CSRF; ID создаются после key lookup, timestamp PostgreSQL microseconds.
Authorize проверяет durable tuple и enabled/verified initiator. Complete сам
читает receipt вне SQL Tx, затем общий finalizer берёт user→command locks и
атомарно создаёт owner/COMPLETED. Invalid receipt или disabled/deleted user →
committed quarantine без owner. Completed replay не восстанавливает revoked access.
Owner insert не повышает financier и не заменяет другого owner.
Fingerprint v1 общий fixture Auth/Core: `testdata/company-bootstrap-fingerprint-v1.json`
(кириллица, absent/null INN, leading zeros, escaping, trim).

A1 real tests: concurrent scoped insert/reopen, migration2→3→2→3/up replay с
сохранением user, atomic rollback, lease reclaim/stale worker fencing,
initiator deletion, owner conflicts. Service tests: две replicas/pools и restart,
body conflict, missing receipt/PREPARED, disabled user во время read, отсутствие
user lock через network boundary (SQL NOWAIT), revoked membership replay.
Core boundary здесь fixture; это не полный Core runtime E2E.

A1 два read-only reviews exact `1ffd947..9463e40` завершены без доказанных дефектов.
В repository review исправлен CHECK nullable account_id до публикации, реальные
integration повторены. Первоначальный quota failure review/Buf устранён после reset;
оба reviews повторно завершены, Buf1.75.0 установлен. Ничего не обходилось.
После merge main Auth tree не изменился (`git diff --exit-code 9463e40 7f62b72 -- services/auth`).

A2 текущий снимок: `7f62b72` + RPC/client/config/generated/tests/docs diff.
Три Auth RPC зарегистрированы, owner из service/session, feature-local mapping,
UUID/spec presence и безопасные typed 409/503 errors. Реальный TLS Auth listener
пускает bootstrap authorization только Core; API/Auth/ML/frontend отклонены до state.

Auth Core client: app-owned connection/Close, TLS13, обычные CA+hostname проверки
и exact Core URI, local Auth URI; no retries, per-call deadline50ms–3s/default2s.
Receipt codec сверяет canonical IDs/spec/hash/account/committed timestamp;
peer denial без business detail остаётся dependency failure. Error text безопасен,
cause сохраняется. Missing/unavailable optional client не ломает login/verify.

A2 проверки (exit0, не skipped): full Auth unit/race/shuffle и real PostgreSQL
integration/race/shuffle, vet/build/full-module lint0issues; actual mTLS client
положительный и wrong URI/hostname/local identity; protobuf adapters/Core-only
Auth authorization; deadline/receipt/error codec; register/verify/login при
client init failure; Buf lint/format/build/breaking against origin/main snapshot;
exact generated drift, shell syntax, Python provision template compile и
Ruff0.17.1 check/format для изменённого deploy template.
Два read-only Caveman/Ponytail reviews RPC/client diff поверх7f62b72 — доказанных
дефектов нет. Применены pinned Go grpc/security/context/testing/error skills.
RPC/client опубликованы в `382fed6`; recovery завершён следующим diff ниже.
Real Core→PostgreSQL chain и полный browser E2E ещё pending.

## Remote CI и предыдущий контекст

Последний полностью проверенный CI — `7f62b72`: Auth lint/tests/build SUCCESS
([run40136094127](https://github.com/benzocloud/hack-rnd-autumn-2026/actions/runs/40136094127));
Core contracts/lint/tests/build SUCCESS, GHCR publish SKIPPED (не тест)
([run40136094115](https://github.com/benzocloud/hack-rnd-autumn-2026/actions/runs/40136094115));
infra safety/ml-tests SUCCESS
([run40136094215](https://github.com/benzocloud/hack-rnd-autumn-2026/actions/runs/40136094215)).
Новый A2 SHA требует нового чтения CI; queued/in-progress не passing.

Полные descriptions/conversation/reviews/inline replies с pagination прочитаны
для связанных PR16/17/25/30/31/34–40/43. GBrain recall выполнен; Git/contracts
авторитетны. PR40 уже разделил companies/invitations; PR43 меняет validation/
useRequest/Workspace: их изменения сохранены, не дублировать и не откатывать.
Никаких чужих незавершённых изменений не переносилось.

## Воспроизведение и tooling

На другом компьютере с Git SSH и авторизованным gh:

```sh
git fetch origin
git switch --track origin/prosteyshiyyy/feat/company_onboarding
cat .agent/handoffs/company-onboarding.md
git status --short
gh pr view 41 --repo benzocloud/hack-rnd-autumn-2026 --json isDraft,headRefOid,url
```

Если ветка уже локальная — обычный git switch; для parallel checkout отдельный
worktree. Перед push fetch origin/main + обычный merge, без force/rewrite/stash чужого.
Check diff/index/secrets, обновить этот handoff в том же commit, push, проверить
remote SHA и draft/CI. SHA текущего commit внутрь него не вставлять.

Pinned: Go1.27.3; golangci-lint2.15.0; Buf1.75.0; protoc-gen-go1.36.12 и
protoc-gen-go-grpc1.6.2 из существующего buf.auth-core.gen.yaml.
Review sources/pins: `.agent/infra/skills/skills.json`; pinned fetch в отдельный
каталог, root-owned runner installer на developer машине не запускать.
Инструкции skill не равны проведённому review.

Пример установки tooling без изменения go.mod:

```sh
GOTOOLCHAIN=go1.27.3 GOBIN=/tmp/saldo-onboarding-tools go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
GOTOOLCHAIN=go1.27.3 GOBIN=/tmp/saldo-onboarding-tools go install github.com/bufbuild/buf/cmd/buf@v1.75.0
```

Добавить tool directory в PATH. Из services/auth:

```sh
bash scripts/generate.sh --check
golangci-lint fmt --diff ./...
golangci-lint run ./...
GOTOOLCHAIN=go1.27.3 go test -race -shuffle=on -count=1 -timeout=90s ./...
GOTOOLCHAIN=go1.27.3 go test -race -tags=integration -shuffle=on -count=1 -timeout=90s ./...
GOTOOLCHAIN=go1.27.3 go vet -tags=integration ./...
GOTOOLCHAIN=go1.27.3 go build ./...
```

Generation: `bash scripts/generate.sh` (generator + agreed formatter only),
canonical Auth/Company/Errors subset, no manual generated edits. Auth CI includes
exact drift and specific framework-generated dispatch/comment lint exceptions,
without blanket exclusions. Canonical Buf lint/format/build/breaking from root.
Full Core gate из services/core: task check / task integration (task3.48.0,
separate marked Core DB по README). API из services/api: bash scripts/check.sh.
Frontend только из frontend/: npm ci / npm test / npm run typecheck / npm run lint /
npm run build, затем настоящий browser с financial mocks off. Эти remaining gates
ещё не объявлялись выполненными для полного onboarding.

Отдельная Auth DB fixture из root repo:

```sh
docker run --rm --name saldo-onboarding-test-pg -e POSTGRES_USER=auth_test -e POSTGRES_DB=auth_test -e POSTGRES_HOST_AUTH_METHOD=trust -p 127.0.0.1::5432 -d postgres:18.6@sha256:edb0c08dc33befd0262ab3b5cf1c4f3f8cfd8b09e88c05b45adee29d6761ddb6
docker port saldo-onboarding-test-pg 5432
docker exec saldo-onboarding-test-pg psql -U auth_test -d auth_test -c "ALTER DATABASE auth_test SET saldo.test_database = 'saldo-auth-integration';"
```

Задать AUTH_TEST_DATABASE_URL на выданный loopback port, user auth_test/database
auth_test/sslmode=disable (disposable trust fixture). Helpers ограничены *_test и
создают/удаляют только свои random schemas. В текущей среде port32769; считать его
локальным значением, на другом компьютере читать docker port. После проверок
остановить только свой fixture: docker stop saldo-onboarding-test-pg (--rm).
Не трогать production/Core DB, чужие containers/worktrees или persistent volumes.

## Config и будущие operator действия

New env: AUTH_CORE_ADDRESS, AUTH_CORE_SERVER_NAME, AUTH_CORE_TIMEOUT. TLS cert/key/CA
из AUTH_GRPC_CERT_FILE/AUTH_GRPC_KEY_FILE/AUTH_GRPC_CA_FILE; Auth cert needs clientAuth
и exact Auth URI. Existing provision template cert уже dual serverAuth/clientAuth.
У нового stack шаблон задаёт Core dependency; в существующей установке оператор
позднее должен отдельно обновить auth.env этими тремя env и проверить existing PKI.
provision.py отказывается перезаписывать существующую установку — не запускать его
для обновления и не менять live secrets/CA/server/firewall/SSH. Никакой deployment
этим поручением не разрешён. Tests используют ephemeral PKI, файлы ключей не публикуются.

## Следующие действия

1. A3 живая цепочка прогнана локально: `cd services/api && GOTOOLCHAIN=go1.27.3 go test -tags=onboarding -count=1 -timeout=20m -run '^TestRealCompanyOnboardingChain$' ./integration`. Отдельные бинарники Auth/Core/API, HTTPS и mTLS, disposable Postgres с marker. Register/verify/login, owner, пустой счёт, replay, конфликт ключа, отказ без CSRF, чужой пользователь, чтение после перезапуска Auth. Не закрыто: чужой peer, revocation, timeout, потерянный ответ bootstrap.
2. A4: ключ создания компании сохраняется до подтверждённого ответа, поэтому повтор и обновление страницы не создают вторую компанию. `node --import ./tests/register.mjs --test tests/company-create-key.test.mjs` из frontend/. Браузер с выключенными financial mocks ещё не прогнан.
3. Дальше: недостающие отказы A3, браузер A4/A5, два ревью и CI. Не принимать этапы за владельца. Merge, ready-for-review и deployment по-прежнему запрещены.

Known blockers продукта не подтверждены; remaining work выше не выполнен.
Не создавать heartbeat/resume jobs; при явном takeover прекратить изменения/push.


## A2 recovery checkpoint — проверенный diff поверх382fed6

ReconcileOne берёт одну PREPARED-команду через DB-time 60s lease, commit claim,
preflight user→command locks, затем вне Tx читает receipt; при отсутствии вызывает
идемпотентный bootstrap и повторно читает receipt. Общий finalizer выполняет тот
же атомарный owner/COMPLETED или quarantine. Cookie не нужна. Истечение/отзыв
сессии и credential reset не отменяют принятую команду; disabled/deleted user
карантинится до Core. При cancel остаётся durable lease для restart/reclaim.

App владеет одним worker, ограничивает проход10s, poll1s, transient backoff1–60s;
per-command backoff также1–60s. Root cancellation останавливает и join worker
до RPC/client/DB Close; временный Core outage не останавливает Auth login.
Lease token/presence проверяются в service, expiry — только SQL clock_timestamp
CAS; неуспешный Complete откатывает InsertOwner той же Tx.

В двух независимых Caveman/Ponytail reviews найден и исправлен MAJOR clock-skew:
при Auth clock>=DB+60s свежая lease ошибочно считалась потерянной. Локальные expiry
проверки удалены; initial next_attempt_at теперь DB clock_timestamp. Никаких
published migrations задним числом не менялось (только repository INSERT).
Новый unit regression при Auth clock+2min проходит; negative Go overlay со старым
условием ожидаемо падает с PREPARED/no owner. Real PG regression доказывает
немедленный claim при future CreatedAt и отказ same-token после DB lease expiry.
Открытых доказанных замечаний после повторного review не осталось.

На финальном recovery/clock-fix diff поверх382fed6 прошли exit0:
full Auth unit/race/shuffle, full realPG integration/race/shuffle, vet/build,
fmt/import/full-module lint0issues. Новые tests проверяют lost bootstrap reply,
fresh service/pool restart, две replicas, expired/reset browser session при
действующем recovery, disabled/deleted initiator, отсутствие membership при stale
worker, re-read после bootstrap, cancellation и bounded lifecycle/backoff.
Реальный Core receipt runtime всё ещё A3/A5 gate: здесь fixture Core boundary.

Последний прочитанный CI382fed6: Auth lint/tests/build и Core contracts/lint/tests
SUCCESS; Core build ещё IN_PROGRESS. Stack verify и image(api/auth/core/ml/
frontend-real) SUCCESS, deploy SKIPPED. Эти image checks не означают deployment.
Источники: Auth40137448722, Core40137448706, stack40137448719. Recovery diff ещё
не покрыт этим старым CI и требует нового чтения после push.


## A3 adapter checkpoint — проверенный diff поверхb2f882b

Новый feature-local handler companies.go оркестрирует Begin→Core bootstrap→
Complete→ReadCompany→201. Core получает только сохранённый Auth spec/hash/IDs;
API не нормализует его повторно и не принимает owner/command IDs от браузера.
CompanyDTO переиспользован. Проверяются корректность ответов зависимостей и
COMPLETED; malformed/quarantined/pending/error не выдаются за успех. Origin/CSRF,
scoped original key и request ID сохранены. Этот POST не требует P1 Ledger/Forecast
clients. Create endpoint использует Auth command 503 timeout semantics (Auth
contract367), не меняя общий Core financial deadline504 mapping (Core contract144).

Полный literal gate из services/api завершился exit0:
`PATH=<pinned-tools>:$PATH PYTHON=<isolated-venv>/bin/python GOTOOLCHAIN=go1.27.3 bash scripts/check.sh`.
Canonical generation/imports, build/unit/race/vet/format/full module lint0issues,
Ruff/script tests, process HTTPS API→mTLS testAuth smoke/graceful shutdown,
OpenAPI0.2.1 validation41 responses (новые company201/409/503/403/422 включены).
Own statements90.31%, executable block lines90.89%, только internal/gen исключён
по прежнему gate, blanket exclusions не добавлялись.

Эти fixtures/conformance/process testAuth не доказывают actual Auth/Core/PG
onboarding: A3 остаётся «В работе». Новые boundary tests доказывают saved spec,
точный порядок RPC, отсутствие premature201, conflict/timeout/quarantine/revocation,
strict JSON/IDs, отсутствие diagnostics/cookie в публичном response.
Два independent pinned Caveman/Ponytail reviews diff поверхb2f882b: доказанных
дефектов нет; full runtime gate выполнял parent, reviewers не выдавали его за свой.

Tooling для API подготовлено в изолированных каталогах: Go1.27.3,
protoc-gen-go1.36.12/protoc-gen-go-grpc1.6.2, golangci-lint2.15.0,
Task3.48.0; Python deps из services/api/requirements-dev.txt и
scripts/requirements-contracts.txt. Воспроизвести pip install в отдельной venv,
plugins/task через GOBIN=<tools> go install с этими pinned версиями. Repo npm
зависимости/production данные не затрагивались. Новый company report генерируется
API_COMPANY_REPORT при TestCompanyCreationContractResponses; check.sh добавляет
его к прежним smoke/financial OpenAPI captures.
