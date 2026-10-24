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
