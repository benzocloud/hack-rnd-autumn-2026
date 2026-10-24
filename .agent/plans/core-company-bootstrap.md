# Core: профиль компании и durable bootstrap

Дата: 2026-10-24. Основание: поручение владельца продолжить Go Core параллельно
с синхронизацией контрактов/ML. Изолированная ветка от `18c45de`; основную рабочую
копию и её незавершённый merge не менять. Принятые ADR-022/024/025/027 сохраняются.

Цель: Company profile + единственный BankAccount + неизменяемый bootstrap receipt
создаются одной транзакцией; повтор Auth-команды возвращает те же данные без
дублирования. Auth владеет инициатором/membership; никакой owner из запроса Core.
Контракт company.proto и Auth bootstrap стабильны; сами proto здесь не меняются.
Референс: текущие service/feature Work и app/postgres.WithinTx/sqlx; SQL отдельно,
embeds queries.go, row mapping отдельно. Новые зависимости не требуются.

## C1 — внутреннее создание и хранение

Статус: Принят владельцем 2026-10-24 в боковом обсуждении после повторных build/unit/race/vet/lint. Зависимости: существующие account/Tx, Auth durable command/hash.
Файлы: domain/company, service/company, repository/postgres/company, migration00004,
app/companies.go и unit/integration tests. Отдельная новая profile-таблица сохраняет
legacy ID-only компании без фиктивных имён/ИНН/остатков; профиль не синтезируется.

- Добавить валидируемые CompanySpec/Profile и неизменяемый bootstrap record.
- Проверить SHA256 canonical Go JSON name,inn,currency,timezone, null INN.
- Обязательный authorize callback получает Auth durable command; сверить все поля
  до storage/replay. Nil/сбой callback не разрешает bootstrap. Сеть пока не подключать.
- Резервировать command ID, создать company/profile/account/receipt атомарно;
  повтор с совпадающими данными возвращает receipt, иной payload — conflict.
- Сохранять результат только после commit; default account UUIDv7, data_version=1.
- Дать внутренние read-profile/read-receipt и проверить corruption/missing scope.
- Проверки: unit/race/full lint; отдельная одноразовая loopback PostgreSQL:
  fresh/up/down/up, replay/conflict/concurrency/rollback/immutable receipt.

Критерий: нет повторных компаний/счетов, частичных записей или выдачи результата
при ошибке commit/Auth. Этап завершить отчётом и остановкой на ревью владельцем.
Commit/push/PR/deployment в этой боковой задаче отдельно не разрешены.

## Следующий этап — после ревью C1

Сетевой Auth client/mTLS → CreateCompanyBootstrap/GetCompanyCreationResult и
Read/UpdateCompany handlers, real Auth recovery. C1 не объявляет их готовыми.
Основное app.New связывает внутренние порты, bootstrap по умолчанию fail-closed
до подключения настоящего authorize callback. Existing company read не означает
публичный доступ: transport должен проверять Auth READ; receipt доступен только Auth.

## Результат C1

Статус: Принят владельцем 2026-10-24; ниже исторический результат C1 до C2.
Основа `18c45de1a4168ffc3cd942767fc5d94ac0de0ecd`, ветка
`globalarray/feat/core_company_bootstrap`, отдельный checkout
`/private/tmp/saldo-core-company-bootstrap`. App registration вернул thread not
found, поэтому checkout размещён в разрешённом временном каталоге и используется
явно; основная рабочая копия осталась в своей ветке/merge.

Реализованы profile/receipt/канонический fingerprint, feature-local bootstrap и
чтения, sqlx-хранилище и migration00004. Service последовательно сохраняет
company → account → profile внутри одной app-owned Tx; repository не принимает
service DTO и не управляет сценарием/Commit. Уникальное reserve по Auth command ID
сериализует повторы; deferred FK исключает commit незавершённого receipt.
Существующие ID-only компании остаются без фиктивных метаданных.

Проверено локально:

- Core build/fmt/unit/race/vet/lint; full-module lint с integration — PASS.
- Полный PostgreSQL integration/race/shuffle всего Core — PASS на отдельной
  loopback test DB с обязательным marker saldo-core-integration.
- Fresh Goose up всех четырёх миграций; down/up00004 и повтор bootstrap integration
  — PASS. 8 конкурентных одинаковых команд вернули один receipt/account.
- Replay из другой app/connection, неизменность receipt после rename профиля,
  conflicting command/hash, отказ Auth и nil authorizer, ID-only profile missing,
  rollback после ошибки вставки и после deferred FK на COMMIT — PASS.
- Auth canonical JSON hash: null INN, leading zeros, UTF-8 и стандартное Go
  escaping; ошибки авторизации безопасны в Error(), причины доступны errors.Is.
- Caveman-review: устранён raw Auth error наружу; service управляет порядком
  сохранения сущностей, результат не возвращается при ошибке commit.
- Ponytail-review: нет DI framework, центрального Store/реестра репозиториев,
  дополнительных wrappers для каждой операции или нового пакета request/result.

Ограничения: настоящий Auth/mTLS callback пока не подключён; app.New использует
nil и намеренно запрещает создание. ReadProfile/GetBootstrapResult — внутренние
методы; транспорт обязан проверить Auth READ / service-only recovery. Сетевых
handlers, UpdateCompany, membership finalization, CI публикации и deployment
эта поставка не объявляет готовыми. После merge контрактов нужны синхронизация
с main и проверка номера миграции перед общей поставкой. Commit/push не выполнялись.

После проверок собственные тестовый контейнер и сеть удалены. Worktree с
исходниками сохранён для ревью; файлы основной синхронизации не изменялись.

## C2 — Auth client и приватный CompanyService

Статус: Принят владельцем 2026-10-24 командой «пуш и давай дальше» после отчёта
о реализации и локальном ревью. Разрешены commit/push C1/C2 и следующий подэтап;
merge и deployment не разрешены. Владелец явно принял C1 и разрешил переход к Auth/gRPC.
Зависимости: C1, существующие canonical Auth/Core proto и ADR-027.
Компоненты: internal/client/auth, handler/grpc/company, app/server, transport/mtls,
config RPC, cmd/core, company Update и PostgreSQL CAS.

- Реальный gRPC Auth client с deadline <=3s; проверять durable bootstrap command
  и actor/role, не принимать caller-selected пользователя. TLS/Close у app.
- mTLS: operator CA, точные URI SAN api/core/auth, методная allowlist. API не
  читает recovery receipt; Auth не читает/меняет обычный company profile.
- Create/GetCreationResult/Read/Update handlers, безопасный CoreErrorDetail;
  Auth authorization/CSRF предшествует SQL, в том числе replay. Update использует
  CAS профиля и не меняет immutable bootstrap receipt или версию финансовых данных.
- Отдельный runtime constructor/entrypoint с явной конфигурацией и shutdown;
  foundation app.New сохраняется для внутренних integration tests, fail-closed.
- Проверки: Core build/fmt/unit/race/vet/lint, реальная loopback PostgreSQL,
  grpc mTLS tests с Auth test service, peer denial, revoked/CSRF/deadline,
  replay/recovery/CAS/nullable INN. Proto не менять.

Готовность реального Auth не предполагать: в текущем checkout есть только
его foundation. При отсутствии Auth handlers подтвердить транспорт через
contract-compatible test service и отдельно указать, что E2E с настоящим Auth
и membership finalization ещё не проверен. Следующий этап — после ревью C2.
Публикация и deployment не разрешены.

### Результат C2 — 2026-10-24

Реализованы четыре RPC CompanyService, Core→Auth gRPC client, строгая методная
allowlist по проверенному URI SAN, конфигурация mTLS и cmd/core. Runtime владеет
connection/pool/listener и ограниченным graceful shutdown; reflection выключен.
Auth проверяется до SQL; bootstrap re-authorizes даже replay; recovery receipt
доступен только Auth без browser session. Actor/role берутся только из Auth.
Update имеет feature-local command, domain validation и SQL CAS по версии;
omitted INN сохраняется, clear_inn удаляет, receipt/account data_version неизменны.
Ошибки сериализуются в безопасный CoreErrorDetail; сырые Auth/SQL сообщения,
cookie/CSRF не выходят наружу. Typed rate limit сохраняет retry hint.

Проверено: task check (build/fmt/unit/race/vet/lint), full-module integration lint,
PostgreSQL integration/race/shuffle, fresh Goose up00001–00004. Проверки покрывают
два реальных gRPC mTLS перехода с contract-compatible Auth double, durable replay,
recovery через другой pool/server, конкурирующий PATCH, неверный CSRF/revoked,
запрещённые peer/method/metadata identity, отсутствие client cert, чужую CA,
неверный server URI. Настоящий app/server.Run проверен через TCP и PostgreSQL;
завершение по cancel дополнительно проверено 20 раз с race detector.

Локальное caveman-review: исправлена гонка результата shutdown при отмене до
начала Serve; peer/configuration denial Auth без typed detail остаётся 503,
не превращается в пользовательский 403. Проверены nullable INN и независимость
версии профиля/финансовых данных. Ponytail-review: убран искусственный Spec
для проверки PATCH; общие проверки полей принадлежат domain. Новых библиотек,
универсальных DTO/реестров репозиториев и автоматических retry writes нет.
Ревью выполнено локально последовательно, не отдельными агентами.

Ограничения: в исходном checkout Auth handlers ещё нет; проверка с Auth double
не доказывает настоящий onboarding/membership finalization. Финансовые handlers,
worker, интеграция ML и frontend не реализованы этим этапом. Docker image/CI/
deployment не проверены этой поставкой. Worktree по-прежнему от 18c45de; перед
публикацией синхронизировать с актуальным main, сохранив C1/C2: туда уже включены
единый Predict и исправления зависимостей. В основной рабочей копии изменений
не делалось. На момент отчёта C2 ожидал ревью, commit/push/PR не выполнялись.
Затем владелец принял C2 и разрешил публикацию ветки и продолжение.

## C3 — чтение основного счёта и известного остатка

Статус: Принят владельцем 2026-10-24 командой «апрув давай дальше» после отчёта C3.
Владелец разрешил следующий подэтап; публикация C3/C4 отдельно не поручена.
Основа: 39d0f41, включает main 6afa1dc; C1/C2 опубликованы в той же ветке.
Цель: GetDefaultAccount даёт API реальный account ID, data_version и factual
source для дальнейших финансовых команд; balance отсутствует, пока нет принятого
датированного наблюдения. Референс: существующая service→store/app-owned Tx граница.

Компоненты: service/ledger read DTO, PostgreSQL latest balance query, Ledger gRPC
handler и общая транспортная регистрация/peer allowlist/errors, Auth FINANCIAL_READ.
Proto/схема БД не меняются. Company profile определяет неизменяемый default account.

- Сначала Auth FINANCIAL_READ с исходной BrowserContext, затем company/profile
  и tenant-scoped account; чужой account ID клиент вообще не передаёт.
- Читать account, bindings и последнее EOD <= сегодняшней даты Europe/Moscow
  согласованно внутри одной Tx, без повышения data_version. Использовать
  существующий lock счёта, соблюдаемый всеми финансовыми writers; Auth вне Tx.
- Явные RUB/дата/source, nil balance для неизвестного остатка; не рассчитывать
  текущий остаток из фактов и не выдавать старое EOD за сегодняшнее наблюдение.
- Общая grpcserver factory владеет mTLS/peer policy; feature handlers только
  регистрируют свои RPC. Реализован только GetDefaultAccount у Ledger, остальные
  Ledger RPC этим этапом не объявляются готовыми.
- Проверки: build/fmt/unit/race/vet/full lint, integration lint, PostgreSQL+
  mTLS/Auth double: новый счёт без balance, manual/statement EOD, отрицательный
  остаток, latest date/cutoff, tenant scope, отказ Auth/peer и неизменность версий.

Готовность: API получает честную проекцию основного счёта. Следующая поставка
RecordManualBalance отдельно должна сохранить полный исходный RPC replay result;
нынешний date/version receipt нельзя гидратировать из изменившегося account.
После C3 остановиться на ревью. C3 commit/push отдельно не разрешён.

### Результат C3 — 2026-10-24

Реализован GetDefaultAccount и application ReadAccount, latest balance SQL/row
mapping. Финансовый доступ проверяется до company/profile/account чтений.
Account ID берётся из профиля компании; источник истории отдельно от EOD source.
Чтение не создаёт receipt/новую версию. Неподтверждённый остаток отсутствует;
нулевой/отрицательный принят как наблюдение, future EOD исключён. Дата среза
определяется Europe/Moscow независимо от timezone процесса. Согласованность
account/bindings/EOD обеспечена существующим account lock внутри одной Tx.

Company-specific server factory разделена на feature Register и общий grpcserver:
методная mTLS allowlist/лимиты/deadline сохранены; GetDefaultAccount только API.
Общий rpcerror используется двумя handlers, сохраняет безопасные Auth/Core codes.
Существующие Company RPC/recovery/runtime tests проходят после переноса.

Проверки: полный task check, integration-tag lint и PostgreSQL integration/race/
shuffle — PASS. Проверены RPC через настоящий mTLS с Auth double: новая компания
без balance; ручной отрицательный EOD; неизвестная factual authority после manual
EOD; manual_v1 после первого факта; более старый/future EOD не подменяют latest;
read не меняет data_version; revoked/foreign company/Auth peer отвергаются.
Statement EOD с нулём и imported namespace проверен на PostgreSQL, его exact
protobuf mapping — unit test. Проверены отказ storage/commit и скрытие результата,
tenant scope, malformed data, дата в Москве на следующем дне относительно UTC.

Локальные caveman-review/ponytail-review выполнены последовательно без отдельных
агентов. Проверены сохранение peer policy при extraction, Auth-before-storage,
optional/zero money и отсутствие лишних зависимостей/универсальных DTO. Блокирующих
замечаний не осталось. Proto/схема БД не менялись. Настоящий Auth и весь browser
E2E по-прежнему требуют проверки после реализации Auth runtime другой командой.

C1/C2 опубликованы: db1f3e6; синхронизация main/published head — 39d0f41,
remote SHA проверен после push. C3 принят владельцем и остаётся локальным.
Новый PR/merge/deployment не выполнялись. Дальше — RecordManualBalance с полным
durable replay ответа и FINANCIAL_WRITE/WriteContext; до принятия C3 не начат.

## C4 — ручной датированный остаток и полный replay ответа

Статус: Принят владельцем 2026-10-24 командой «давай дальше» после отчёта C4.
Цель: RecordManualBalance принимает RUB/EOD, проверяет Auth FINANCIAL_WRITE с
исходным WriteContext/CSRF и возвращает честный Account. Тот же ключ/тело после
рестарта и дальнейших изменений возвращает первоначальный Account целиком.

Компоненты: service/ledger manual balance command/result/receipt codec, Ledger
handler, Auth permission guard, error mapping, unit/PG/mTLS integration tests.
Референс: существующий feature Work и opaque commandStore; SQL lifecycle у app,
координация у service. Proto/схема БД/старые receipts не меняются.

- Только API mTLS peer; Auth перед любым SQL/replay. Проверить scope относится
  к default account компании; ActorID только из ответа Auth. Отозванное членство,
  неверный CSRF и недоступный Auth запрещают также replay.
- Проверить key 8–128 ASCII [A-Za-z0-9._:-], обязательные note/даты/RUB/version,
  дата не позже server-local Today Europe/Moscow; ноль/отрицательный EOD допустимы.
- Новый operation namespace для публичного RPC сохраняет старые JSON/digests.
  fingerprint содержит финансовый payload/исходную версию/note, без cookie/CSRF/
  clock. Порядок key reservation → account lock → EOD. Повтор ключа до CAS;
  новая команда со stale version отклоняется даже при совпадении старого EOD.
- Применить существующую доменную запись EOD в одной Tx. Снимок account/source/
  latest dated balance и receipt сохранить до commit, ответ выдать после commit.
  Нет nested Tx, Auth под lock или повторной гидрации mutable state при replay.
- Receipt имеет явную версию и собственное persisted representation; проверить
  scope/дату/суммы/источник/версии при восстановлении. Повреждённый receipt — отказ.
- Проверки: full check/lint, PG integration/race/mTLS, concurrency/reconnect,
  replay после нового факта/EOD/смены московского дня, key/body conflict, stale,
  cross-account, CSRF/revoked/peer, future date, zero/negative и rollback/commit.

После C4 — отчёт и остановка на ревью. Commit/push/PR/merge/deploy не входят.

### Результат C4 — 2026-10-24

Реализован LedgerService.RecordManualBalance: исходный WriteContext передаётся
Auth с FINANCIAL_WRITE до SQL и replay; actor берётся только из ответа Auth,
account проверяется по профилю компании. Дата ограничена сегодняшним днём
Europe/Moscow, принимаются RUB, обязательный note и допустимый idempotency key.
Нулевой и отрицательный EOD сохраняются как наблюдения; ручной остаток не выбирает
и не меняет factual authority.

В одной app-owned транзакции выполняются reservation ключа, account lock,
проверка исходной версии, запись EOD и полного versioned receipt результата.
Ответ возвращается после commit. Новый operation namespace не меняет старые
receipts; fingerprint не включает cookie, CSRF или текущую дату сервера.
Повтор возвращает исходные account/version/source/balance из сохранённой
проекции, без чтения изменившегося счёта. При восстановлении проверяются schema,
scope, версии, даты, суммы и источник; повреждённый результат отвергается.

Повторная проверка после исправлений scope наблюдения и версии receipt:

- Полный `task check` (build, fmt, unit/race, vet, lint) — PASS.
- Полный lint с `--build-tags=integration` — PASS, 0 issues.
- Полный `go test -tags=integration -race -shuffle=on -count=1 -timeout=150s ./...`
  на отдельной маркированной PostgreSQL 18.6 — PASS. Использован существующий
  одноразовый Compose project `saldo-core-c4`; другие сервисы не затронуты.
- Настоящий mTLS с Auth double: восемь одновременных одинаковых запросов дают
  одну запись EOD и один receipt. Новый экземпляр приложения/соединения после
  изменения фактов, версии, source и более позднего EOD возвращает исходный
  protobuf-ответ. Отдельный unit test проверяет replay при смене дня.
- Проверены key/body conflict, новый stale key при уже существующем EOD,
  неверный CSRF, revoked session, чужой account и недопустимый mTLS peer;
  ноль/отрицательный остаток, imported authority и конфликты даты/источника.
- Ошибка commit откатывает EOD, version и receipt, не выдаёт результат и не
  отравляет следующий повтор. Unit tests проверяют чужой scope и повреждённую
  проекцию receipt. `git diff --check` — PASS.

Caveman-review транзакционной границы выполнен независимым агентом: новых
подтверждённых замечаний нет. Локальный Ponytail-review: дополнительных
абстракций и библиотек нет; используются существующие feature Work, domain EOD
и opaque commandStore. Схемы БД, protobuf и старые operation/digest не менялись.

Ограничения: Auth в интеграционных тестах — double; реальная проверка CSRF/
сессий/членств остаётся за Auth runtime. Это не browser E2E и не deployment.
C4 принят владельцем; commit/push/новый PR/merge не выполнялись.
Worktree остаётся на `39d0f41` с локальными C3/C4. Перед будущей публикацией
синхронизировать с актуальным main (после PR #30 — `50becdc`), сохранив изменения.

## C5 — ручная фактическая операция через согласованный RPC

Статус: Принят владельцем 2026-10-24 командой «апрув» после отчёта C5.
Реализация была разрешена после принятия C4.
Цель: CreateManualTransaction создаёт ровно одну фактическую операцию из формы,
возвращает полный Transaction и исходный data_version, устойчивые к повтору.
Компоненты: service/ledger и private receipt codec, Ledger handler/mapping,
существующая app-owned Tx, SQL metadata/category check, unit/PG/mTLS тесты.
Референс: существующие CreateTransaction и C4; без новых библиотек/сервисов.

- Auth FINANCIAL_WRITE с исходным WriteContext до SQL/replay; actor от Auth,
  company/default account проверяются как в C4, mTLS peer только API.
- Только manual source, UUIDv7/время создаёт Core, положительная RUB сумма,
  направление/бизнес-дата не позже Europe/Moscow Today, валидные INN/строки и
  optional category; категория должна принадлежать этой компании.
- Отдельный operation namespace и receipt не меняют старые ключи/digests.
  key → account lock → category check → факт/source/version/receipt в одной Tx;
  повтор до CAS возвращает первоначальные UUID/created_at/category/версии,
  без повторного чтения изменившейся операции. Новый stale key отклоняется.
- Существующий createTransaction используется для доменной записи; imported
  factual authority запрещает ручную историю. Ручной факт не меняет EOD.
- Добавить nullable persisted created_at для старых операций без выдуманного
  backfill; новое время задаётся при записи, никогда при чтении. Старые domain
  restore и receipts остаются совместимыми. Proto/REST не меняются.
- Проверки: full check/integration lint, PostgreSQL/mTLS/race, concurrent retry,
  reconnect/full replay, отказ commit/Auth/CSRF/peer, stale/body conflict,
  invalid/future/zero/INN, чужой account/category и source conflict без записи.

После C5 — отчёт и остановка на ревью; commit/push/merge/deploy не входят.

### Результат C5 — 2026-10-24

Реализован LedgerService.CreateManualTransaction. Auth FINANCIAL_WRITE получает
исходный WriteContext до SQL/replay; scope проверяется по default account,
actor берётся из Auth. Проверяются дата в Europe/Moscow, положительная RUB сумма,
направление, INN с ведущими нулями, длины/UTF-8 строк, key и категория компании.
Optional INN/category сохраняют presence/null; malformed/пустые значения
и неизвестное enum отвергаются. Экономические инварианты остаются в domain.

CreateTransaction разделён на account load и общий createTransactionOnAccount;
legacy namespace/digests/receipts не менялись. Новый public use case проверяет
CAS и tenant-scoped category под account lock, затем использует этот writer
в одной app-owned Tx. Первый факт выбирает manual_v1, imported authority даёт
FACTUAL_SOURCE_CONFLICT. Факт не меняет и не создаёт датированный EOD.

Отдельный create_manual_transaction_v1 receipt сохраняет полный payload,
UUIDv7, created_at и итоговый data_version. Повтор возвращает исходный v1 с
исходной категорией/суммой/временем, без чтения mutable Transaction. Clock,
cookie/CSRF и сгенерированный ID не входят в fingerprint исходного запроса.
Schema/body/scope/UUIDv7/версии/время проверяются при восстановлении.

Миграция 00005 добавляет nullable core_transactions.created_at без backfill,
затем CURRENT_TIMESTAMP default для новых INSERT. Репозиторий читает время
новой записи внутри write Tx; оно сохраняется в receipt. Старые строки остаются
без metadata. Domain TransactionData, снимки и protobuf/REST не менялись.
Миграция применена только к отдельной маркированной PostgreSQL Compose project
saldo-core-c4. Приложение не выполняет DDL автоматически.

Проверки — PASS:

- Полный task check: build, fmt, unit/race, vet и lint; 0 issues.
- Полный lint с --build-tags=integration; 0 issues.
- Полный PostgreSQL 18.6 suite: integration, race, shuffle, count=1, timeout=150s.
- Настоящий mTLS с Auth double: 8 concurrent retries дают 1 факт и 1 receipt;
  returned created_at совпадает с SQL, EOD отсутствует, manual source выбран.
- Новый app/connection после category/version и EOD/account изменений возвращает
  первоначальный protobuf-ответ. Проверены key/body/stale conflict, повреждённый
  receipt, CSRF/revoked/foreign account/category и запрет Auth peer.
- Проверены future date, zero, malformed/empty INN, unknown enum, nullable поля,
  UUIDv7, source conflict без записи и повтор после commit rollback.
- Ошибка commit не выдаёт результат и откатывает факт, authority/version/receipt.
  Существующие ledger/payments/snapshot/company тесты также прошли.
- git diff --check — PASS.

Независимые Caveman correctness/security/database review и Ponytail review
завершены без подтверждённых замечаний. Review-агенты тесты не запускали;
указанные runtime-проверки выполнены основным агентом.

Ограничения: Auth sessions/CSRF проверял double, это не browser E2E.
ListTransactions/ListCategories/category PATCH, file import и forecast runtime
этим подэтапом не реализованы. Старые строки без created_at требуют отдельного
решения read-проекции; фиктивный timestamp для них не создаётся.
C5 принят владельцем; commit/push/PR/merge/deployment не выполнялись.

### Разрешение на публикацию C3–C5 — 2026-10-24

После принятия C5 владелец отдельной командой «коммит и пуш» разрешил сохранить
и отправить принятые C3–C5 в существующую ветку
globalarray/feat/core_company_bootstrap. Перед push — обычная синхронизация с
актуальным origin/main и повторные Core проверки; ветки участников и frontend
не изменяются. Это разрешение не включает новый PR, merge или deployment.
