# Контракт Auth Saldo

Дата: 24 октября 2026 года. Версия: 0.1.0. Статус: **на ревью этапа 3.1**.
Это конкретное предложение для согласования Auth, API и Core, а не принятый
ADR, работающий сервис или разрешение перейти к этапу 3.2.

Основание: [план Auth](../../.agent/plans/auth-service-adaptation.md),
[PR #17](https://github.com/benzocloud/hack-rnd-autumn-2026/pull/17),
[API-план PR #16](https://github.com/benzocloud/hack-rnd-autumn-2026/pull/16),
[REST](../api/openapi.yaml), [ADR-024](../../.agent/adr/024-auth-contract.md).
Канонический приватный контракт: [auth.proto](../../proto/auth/v1/auth.proto).

## Владельцы и доверие

| Компонент | Собственное состояние и обязанности |
| --- | --- |
| Auth | Пользователи, credentials, consent registry/evidence, security contexts, сессии, одноразовые токены, членства, приглашения, команды создания компании и email outbox |
| API, @Prosteyshiyyy | HTTP/JSON, Origin/Referer, cookie, запрет утечки токенов в JSON, trusted client IP, координация Auth/Core без своей БД прав |
| Core, разработчик Core | Company/BankAccount, финансовые данные, идемпотентный bootstrap компании, durable result и защищённый UpdateCompany |
| Frontend | Отдельное согласие, формы и переходы, credentials cookie и CSRF; отсутствие доверенных ролей/ID в пользовательском вводе |

Auth использует отдельную БД/роль PostgreSQL; прямого SQL между сервисами нет.
Приватный `auth.v1.AuthService` доступен только по mTLS. Доверенные сертификаты
содержат точный URI SAN `spiffe://saldo.local/api`, `spiffe://saldo.local/core`
или `spiffe://saldo.local/auth`; CA и ключи отдельны для окружений, вне Git.
CN, заголовки и поля запроса не выбирают service identity. Доверяется только
выданная оператором CA; клиент и сервер проверяют цепочку, SAN и назначение.

API разрешены все RPC, кроме `AuthorizeCompanyBootstrap`. Core разрешены только
`AuthorizeCompany` и `AuthorizeCompanyBootstrap`. Остальные сочетания запрещены
до чтения бизнес-состояния с `PermissionDenied`. Сам Auth вызывает Core с SAN Auth.
Reflection выключен. HTTP Auth предоставляет только liveness/readiness во
внутренней сети; браузер не обходит API. Max receive 1 MiB, max send 2 MiB.
API/Core устанавливают deadline 3 s на обычный RPC, 10 s на Register/Login/Reset;
автоповтор мутаций без описанной ниже идемпотентности запрещён. Timeout не доказывает
откат транзакции. Пароль, cookie, CSRF, token и payload письма не логируются.

## Профиль пользователя

Вход — email/password. Проверка email и reset используют purpose-bound случайный
токен, не шестизначный OTP. Макеты писем с шестью цифрами не меняют этот контракт;
изменение способа входа потребует отдельного согласования REST/RPC и ограничений.

Email: ASCII mailbox без display name, длина после trim не больше 254 символов;
trim внешних пробелов и lowercase всего адреса. Нет удаления `+suffix` или точек.
SMTPUTF8/IDN mailbox пока не поддерживаются. Уникальность по нормализованному email.
Пароль: 12–128 Unicode code points, валидный UTF-8, максимум 512 байт; без trim,
нормализации или скрытого усечения. Display name: 1–120 Unicode code points.
Детали проверки на обеих границах совпадают с REST, ошибки не содержат ввод.

IDs пользователей, consent, приглашений и команд — UUID v4; для браузера opaque.
Company ID выдаёт Auth при сохранении команды, а Core использует тот же ID.
Положительные версии приглашений и компаний передаются десятичными **строками**,
не JSON number; клиент не увеличивает их. Неизвестные enum и нулевые permission
отклоняются, а не означают финансовый доступ.

### Сессия и CSRF

| Параметр | Предлагаемое значение |
| --- | --- |
| Cookie | `__Host-saldo_session`; Secure, HttpOnly, SameSite=Lax, Path=/, без Domain |
| Origin | Один HTTPS origin frontend/API; точный allowlist без wildcard credentials CORS |
| Pre-auth context | 10 минут, без продления |
| Сессия | 24 часа absolute, 30 минут idle; истекает по более ранней границе |
| Cookie expiry | Absolute deadline, никогда не продлевается сверх него |
| Session/pre-auth token | `crypto/rand`, 32 байта, base64url без padding, 43 символа |
| Хранение cookie | SHA-256 проверочный хеш; исходный token не хранится открыто |
| Credential version | Положительный счётчик внутри Auth; сохраняется в сессии, не отдаётся API/Core |

`GetSecurityContext` получает opaque cookie. При пустой/неизвестной/истёкшей cookie
создаёт pre-auth, при живом контексте сохраняет его. Возвращает cookie material
только приватному API и CSRF token для публичного JSON. API устанавливает cookie;
никакой `SecurityContext` целиком не сериализуется браузеру. Все ответы no-store.

Стабильный synchronizer CSRF привязан к серверной записи контекста. В ней хранятся
случайный 32-byte csrf_nonce и key_id. Значение — base64url от HMAC-SHA256 по
`"saldo-csrf-v1\0" || context_uuid_bytes || csrf_nonce`; ключ 32 байта находится вне
БД и Git. Auth восстанавливает значение для GET и сравнивает constant-time.
Это не отдельная bearer-аутентификация: запись контекста должна быть действующей.
Ключи сохраняются при рестарте; старый key_id обслуживается до истечения своих
контекстов либо они явно отзываются. Смена cookie/context меняет CSRF.

Перед каждой HTTP-мутацией API проверяет точный Origin; если его нет, проверяет
origin Referer; отсутствие обоих или чужой origin даёт 403. Затем Auth в самой
транзакции мутации проверяет `WriteContext`, срок, отзыв, credential version и CSRF.
Отдельный предварительный RPC-положительный ответ не заменяет эту проверку.
Client IP извлекает API с учётом allowlist proxy; произвольный X-Forwarded-For
из интернета не принимается. ClientContext не доказывает личность пользователя.

Успешный login отзывает входной контекст и создаёт новую сессию/CSRF. Ошибочный
login не отзывает прежний контекст. Неподтверждённый аккаунт может login/me/logout,
но не обращаться к компаниям. Успешный logout отзывает текущий контекст, API
удаляет cookie; другой вход не затрагивается. Повтор logout с живым pre-auth/CSRF
успешен. Далее frontend получает новый CSRF через GET.

### Register и согласие

`GetPersonalDataConsent` возвращает актуальный полный текст, версию и SHA-256
UTF-8/LF без BOM. Auth владеет реестром; версия однозначно определяет текст.
Register требует отдельное `accepted=true`, document_version/document_sha256.
Нет подтверждения — CONSENT_REQUIRED; корректная, но устаревшая пара —
CONSENT_DOCUMENT_CHANGED. Некорректная форма полей — VALIDATION_ERROR.

Под блокировкой текущей редакции одной транзакцией сохраняются новый пользователь,
credentials, неизменяемый evidence (ID/время сервера, полный текст/версия/хеш,
метод `separate_checkbox`) и intent письма. Outbox виден исполнителю только после
commit. Ошибка не оставляет ни аккаунта, ни evidence, ни задания. При существующем
email после той же проверки согласия возвращается generic acknowledgement;
его credentials/evidence не меняются и новый token/mail не создаётся. Гонка
регистраций разрешается уникальностью нормализованного email в БД, без дублей.
Register не создаёт компанию, членство или сессию. Текущие legal-документы MOCK:
эта поставка не разрешает использование реальных данных без замены реквизитов.

### Login и reset при конкуренции

Login читает hash/version, сравнивает пароль вне длительной блокировки БД,
затем блокирует пользователя и повторно проверяет version, account state и
входной security context перед записью сессии. Unknown email выполняет сравнимое
по стоимости dummy hashing и получает ту же INVALID_CREDENTIALS. Сбой БД/хешера
не маскируется под неверный пароль.

Reset под той же user lock повторно проверяет одноразовый token, меняет hash,
увеличивает credential_version и отзывает все сессии пользователя одной
транзакцией. Входной pre-auth тоже отзывается; автоматического login после reset
нет. Каждая дальнейшая Auth-проверка сопоставляет session credential_version с
текущей user version. API/Core не поддерживают собственную копию счётчика.

Обязательны два теста с управляемыми барьерами: старый пароль уже проверен, затем
reset commit — login не выдаёт сессию; login session commit, затем reset — сессия
не действует. Оба результата сохраняются после рестарта.
Общий порядок при наличии нескольких блокировок: users по UUID → существующие
security contexts/sessions по ID → memberships по (company,user) → invitations
по ID → one-time tokens по ID → onboarding/outbox. Первичный lookup без FOR UPDATE
допустим, но все условия повторно проверяются после нужных блокировок. Reset
блокирует отзываемые сессии до token row; cleanup не берёт user lock после session.

## Подтверждение, восстановление и email

Verify token живёт 24 часа, reset token — 15 минут, оба 32 random bytes/43 chars.
В token records хранятся hash, user_id, purpose, expires_at, consumed/revoked.
Resend отзывает предыдущие verify tokens; forgot отзывает предыдущие reset tokens.
Повтор доставки того же intent не меняет token и не продлевает срок.
Потребление и изменение пользователя атомарны; второй concurrent consume проигрывает.
Истёкший, чужого purpose или использованный token даёт один TOKEN_INVALID_OR_EXPIRED.
При authenticated context другого пользователя verify/reset отклоняется тем же
кодом; pre-auth не имеет actor и допускает предъявление валидного token.

Resend/forgot возвращают generic acknowledgement для известного и неизвестного
адреса; отсутствие аккаунта не раскрывается ни отдельной ошибкой, ни очередью
в публичном ответе. Provider outage после сохранения intent не выдаётся за
«письмо доставлено»; ошибки БД до сохранения дают 503.

SMTP-adapter использует TLS с проверкой сертификата; адрес/credentials окружения
вне Git. Для локальной интеграции выбран test sink/Mailpit; для демо нужен
согласованный SMTP и настоящий тестовый ящик. HTML имеет текстовую альтернативу
и CID PNG, без внешних шрифтов/скриптов. Verification/reset URL строится только
из настроенного trusted frontend origin; query token не пишется в access-log,
страница очищает URL после чтения, Referrer-Policy no-referrer.

Outbox сохраняется в локальной транзакции с token/account. Secret payload
шифруется AES-256-GCM случайным nonce на сообщение, с отдельным ключом/key_id вне
БД; associated data связывает intent ID, user ID и purpose. В outbox не хранится
открытая ссылка. Работники используют lease/attempt state в PostgreSQL, не
удерживают SQL-транзакцию на SMTP I/O. Попытки: сразу, затем через 5, 30, 120,
300 секунд с bounded jitter 20%, не позже expires_at. После max attempts — failed,
без ложного success и бесконечного retry. Рестарт поднимает незавершённые intent;
SMTP acceptance с потерянным ответом может привести к дублю того же письма,
но не к второму действию. Доставленные/истёкшие secret payload удаляются.

### Общие лимиты

Счётчики/окна хранятся в Auth PostgreSQL, атомарны и общие для всех реплик;
отказ хранилища не включает обход. Публичные reverse-proxy лимиты дополняют их.

| Операция | Лимит IP | Дополнительный ключ |
| --- | --- | --- |
| GetSecurityContext | 60/мин | Не создаёт новый контекст при действующем cookie |
| Login | 30/мин | 5/мин на нормализованный email |
| Register | 10/час | 3/час на нормализованный email |
| Resend / forgot, каждый | 10/час | 3/час на email; минимум 60 секунд между новыми письмами |
| Verify / reset, каждый | 20/15 мин | 5/15 мин на token hash |

429 возвращает целочисленный Retry-After до конца окна. Для email bucket
используется keyed HMAC, а не каталог открытых email. Argon2id: 64 MiB, iterations=3,
parallelism=1, salt=16 bytes, key=32 bytes; concurrency по умолчанию 2, валидируется
против памяти процесса. Очередь ожидания ограничена, переполнение — 429, timeout
не теряет ресурсы. Это стартовые настройки для проверки на целевом runtime.

## Матрица доступа

Все company-scoped операции требуют действующей сессии, verified email и live
membership. Нет членства/чужой company ID — 404, а не раскрытие её существования.
Недостаток роли при своём членстве — 403. Отказ Auth/deadline — 503, не allow.

| Permission | Owner | Financier | Область |
| --- | --- | --- | --- |
| READ | да | да | Получение компании |
| UPDATE | да | нет | PATCH name/inn |
| FINANCIAL_READ | да | да | Счёт, операции, категории, импорты, подключения, платежи, прогнозы, сценарии, polling/download/plan-fact |
| FINANCIAL_WRITE | да | да | Изменения и запуск финансовых заданий |
| MEMBERS_READ | да | нет | Список участников |
| MEMBERS_MANAGE | да | нет | Удаление financier |
| INVITATIONS_MANAGE | да | нет | Исходящие приглашения, создание и отзыв |

Удаляется только financier membership; owner удалить нельзя. Аккаунт, компания
и финансы сохраняются. Повтор удаления уже удалённого scoped пользователя
идемпотентен; это не запрос глобального каталога пользователей.
На каждом новом Core RPC, включая чтение результата/скачивание, Core сам вызывает
AuthorizeCompany с cookie capability, company_id и точным permission. Ответ API
или browser user_id/role не заменяет проверку. Положительные ответы не кешируются.
Для UPDATE/FINANCIAL_WRITE/MEMBERS_MANAGE/INVITATIONS_MANAGE caller передаёт `AuthorizeCompanyRequest.write`
с исходными cookie/CSRF/trusted client context; вариант browser для этих
permissions отклоняется как CSRF_INVALID. Так финансовая мутация получает
проверку CSRF в Auth, хотя её SQL исполняется в другой БД. Для чтений применяется
вариант browser. API передаёт WriteContext в Core по private transport после
Origin-проверки; Core не доверяет отдельному `csrf_valid=true` от браузера.
Проверка линейризуется при начале нового запроса, не обещает остановить уже
выполняющийся SQL. Принятые durable jobs могут закончиться под service identity;
отозванный пользователь не запускает новые и не читает результат старых.

## REST и приватные RPC

Пути ниже имеют префикс `/api/v1`. Все mutation RPC получают WriteContext.
Auth-only мутация повторяет проверку прав в транзакции, а не доверяет прежнему
AuthorizeCompany. API валидирует JSON/unknown fields до вызова; Auth повторяет
семантические ограничения на своей границе. Page limit: отсутствует → 50,
явно задан → 1–100 (включая запрет 0); protobuf сохраняет presence.
Opaque cursor — ASCII длиной ≤1024 символов, аутентифицирован, привязан к
actor/company/filters/sort. Некорректный, подделанный или чужой cursor даёт
InvalidArgument/INVALID_CURSOR → HTTP 400, как в REST, без раскрытия его содержимого.
Порядок Auth-списков: created_at DESC, ID DESC (members: joined_at, user ID).

| REST | RPC и координация |
| --- | --- |
| GET /auth/personal-data-consent | GetPersonalDataConsent |
| GET /auth/csrf | GetSecurityContext; cookie только через API |
| POST /auth/register | Register; 202 без user/session/token в JSON |
| POST /auth/login | Login; UserProfile → REST user, SecurityContext → cookie, затем новый GET CSRF |
| POST /auth/verify-email | VerifyEmail |
| POST /auth/resend-verification | ResendVerification |
| POST /auth/forgot-password | ForgotPassword |
| POST /auth/reset-password | ResetPassword; сброс cookie, без автоматического login |
| POST /auth/logout | Logout; очистка cookie |
| GET /me | GetCurrentAccount |
| GET /companies | ListMemberships → Core read для доступных company IDs; роль из Auth, не Core |
| POST /companies | BeginCompanyCreation → Core CreateCompanyBootstrap → CompleteCompanyCreation → Core read; 201 только после COMPLETED |
| GET /companies/{id} | Core read → AuthorizeCompany(READ) |
| PATCH /companies/{id} | Core UpdateCompany → AuthorizeCompany(UPDATE) |
| GET /companies/{id}/members | ListMembers, owner only; Member.joined_at → обязательный REST Membership.joined_at |
| DELETE /companies/{id}/members/{user} | RemoveMember, owner only |
| GET /companies/{id}/invitations | ListOutgoingInvitations, owner only |
| POST /companies/{id}/invitations | Core read имени под owner permission → CreateInvitation с исходным Idempotency-Key |
| DELETE /companies/{id}/invitations/{invite} | RevokeInvitation, owner only |
| GET /me/invitations | ListIncomingInvitations, verified recipient |
| POST /me/invitations/{invite}/accept | AcceptInvitation с исходным Idempotency-Key → Core read доступной компании |
| POST /me/invitations/{invite}/decline | DeclineInvitation |
| Остальные GET /companies/{id}/… | Core → AuthorizeCompany(FINANCIAL_READ) |
| Остальные mutations /companies/{id}/… | Core → AuthorizeCompany(FINANCIAL_WRITE), плюс API Origin/CSRF |

В private RPC допускаются raw cookie/session и CSRF для установки/проверки;
в public JSON не допускаются session/access/refresh/verification/reset tokens.
Публичный GET CSRF возвращает только CSRF, а Login — только REST User.

### Ошибки

gRPC status содержит AuthErrorDetail с фиксированным reason и безопасными
field violations; описание не содержит значений запроса. API переводит reason
в публичный `error.code`, а enum-prefix `AUTH_ERROR_REASON_` не выводится.

| gRPC | Причина | HTTP |
| --- | --- | --- |
| InvalidArgument | INVALID_CURSOR | 400 |
| InvalidArgument | VALIDATION_ERROR, CONSENT_REQUIRED, TOKEN_INVALID_OR_EXPIRED, INVITATION_NOT_AVAILABLE | 422 |
| Unauthenticated | AUTH_REQUIRED, INVALID_CREDENTIALS | 401 |
| PermissionDenied | CSRF_INVALID, EMAIL_UNVERIFIED, FORBIDDEN | 403 |
| NotFound | NOT_FOUND, недоступный tenant/recipient/история | 404 |
| Aborted | CONSENT_DOCUMENT_CHANGED, VERSION_CONFLICT, IDEMPOTENCY_CONFLICT, INVITATION_STATE_CONFLICT, MEMBERSHIP_REVOKED | 409 |
| ResourceExhausted | RATE_LIMITED | 429 с Retry-After |
| Unavailable / DeadlineExceeded | DEPENDENCY_UNAVAILABLE, COMPANY_CREATION_IN_PROGRESS | 503 |
| Internal | INTERNAL_ERROR | 500 |

Ошибки service authentication не подменяются ошибкой пароля пользователя.
Peer-denial до бизнес-обработчика не содержит AuthErrorDetail; API выдаёт 503
как ошибку своей зависимости/настройки, а не пользовательский FORBIDDEN.
Недопустимые permission/enum, ID или page.limit — InvalidArgument/VALIDATION_ERROR.
Ошибки page.cursor используют отдельный INVALID_CURSOR, а не HTTP 422.

## Создание компании и контракт стыка Core

1. Begin проверяет live session/verified email/CSRF. Сохраняет initiator, UUID
   команды/компании, нормализованный spec и fingerprint под уникальным
   `(initiator, operation=create_company, Idempotency-Key)`. Owner не входной параметр.
2. Повтор того же ключа возвращает тот же ID/spec/state. Иное тело — 409.
   Fingerprint v1: SHA-256 UTF-8 JSON фиксированного struct в порядке
   name,inn,currency,timezone (стандартное Go JSON escaping, без trailing LF).
   Отсутствующий/null inn → null; name trim, inn 10/12 ASCII digits,
   currency=RUB, timezone=Europe/Moscow. API передаёт Core сохранённый spec,
   а не повторно нормализует браузерный ввод.
3. Core `CreateCompanyBootstrap` принимает command_id/company_id/spec/hash.
   Core вызывает Core-only AuthorizeCompanyBootstrap; поля должны совпасть с
   Auth-командой. Нет браузерного owner, доверия к произвольному X-User/Role или
   обхода private service authentication. Запись Company/default BankAccount
   и результата команды в Core атомарна; повтор не создаёт новые записи.
4. Complete принадлежит тому же initiator и получает только command_id.
   Auth **сам читает** Core `GetCompanyCreationResult(command_id)` по mTLS,
   сверяет company ID/hash/spec и durable committed result. API boolean не
   подтверждает запись Core. Затем Auth атомарно создаёт уникальное owner
   membership и переводит команду в COMPLETED. 201 допустим только после этого.

Core предоставляет двум потребителям следующий стык (реализация у Core):

| Метод | Caller | Необходимое поведение |
| --- | --- | --- |
| CreateCompanyBootstrap(command_id, company_id, spec, hash) | API, Auth recovery | Core проверяет Auth bootstrap permission, хранит committed receipt и возвращает те же ID при повторе |
| GetCompanyCreationResult(command_id) | Auth | Только durable committed result с company_id/hash/spec; неизвестный результат не объявляется успехом |
| ReadCompany(cookie, company_id) | API | Проверка READ в Auth до Company/BankAccount |
| UpdateCompany(write_context, company_id, expected_version, changes) | API | Проверка UPDATE/CSRF в Auth, optimistic version и частичные изменения одной транзакцией |

Это описание требований к Core, не объявление реализованного Core protobuf.
Для UpdateCompany `name` имеет presence; `inn` имеет три состояния: отсутствует,
строка, очистка. В RPC Core нужна presence/oneof, а не одна строка с пустым default.
`inn:null` очищает; пропущенные поля сохраняются; менять currency/timezone нельзя.
Stale version не пишет ничего и даёт Aborted/VERSION_CONFLICT → HTTP 409.

В Auth есть reconciler тех же долговечных PREPARED-команд: читает Core receipt,
при отсутствии вызывает тот же идемпотентный bootstrap, затем выполняет тот же
finalizer. Он не требует браузерную cookie после принятия команды. Истечение
сессии/reset не отменяет уже принятую команду; live session нужна для новых
пользовательских запросов и чтения результата. Удалённый/заблокированный аккаунт
или несовпавший receipt переводит команду в QUARANTINED, без выдачи членства.
Bootstrap проверяет существующий verified/enabled initiator и доверенную команду.

Таймаут после Core write оставляет PREPARED, не ложный 201 и не автоматическое
удаление. В списке аккаунта компания появляется только с active membership.
Незавершённые команды/receipts и ключи с неизвестным исходом не истекают по TTL;
завершённый key→company mapping сохраняется на срок существования компании.
Обнаружение account state и final membership commit синхронизируются user lock.
Проверки: падение в каждом переходе, потерянный ответ, повтор с прежним/иным
телом, две реплики, недоступный Core/Auth, запрещённый peer и карантин.

## Приглашения

Только owner приглашает существующего verified пользователя по email; роль
всегда financier. Company name — snapshot из разрешённого Core read владельца,
не browser field и не независимая Company-модель Auth. REST company_name в
приглашении показывает этот snapshot, даже если компания позднее переименована.
Активное приглашение уникально по (company, recipient user ID); повтор pending
не меняет ID/expires_at. Неизвестный/неподходящий адресат и существующий member
дают один INVITATION_NOT_AVAILABLE, без каталога или autocomplete.

CreateInvitation и AcceptInvitation требуют исходный REST `Idempotency-Key`:
8–128 ASCII символов `[A-Za-z0-9._:-]+`. Auth хранит durable запись под
`(actor_id, company_id для create, method, canonical path, key)` минимум 24 часа.
Проверка session/CSRF/прав обязательна и при replay. Fingerprint — SHA-256 JSON
фиксированного struct без trailing LF: create `{email}` с нормализованным email,
accept `{version}` с исходной десятичной строкой версии. Cookie/CSRF, IP и snapshot
имени компании в fingerprint не входят; ID приглашения входит в canonical path.
Один ключ с иным fingerprint даёт 409 IDEMPOTENCY_CONFLICT до новой мутации.
Захват ключа, изменение приглашения/членства и запись результата атомарны;
две реплики не выполняют действие дважды. Потерянный ответ и рестарт дают тот же
ID/результат без нового приглашения, даже если прежнее уже terminal. Create replay
сохраняет первоначальный snapshot; изменение Core name не считается новым payload.
Хранение идемпотентности не продлевает срок истории приглашения: после его удаления
остаётся только непродлеваемый ключ/fingerprint/ID без email или snapshot до конца
24-часового окна; операция возвращает 404 и не создаёт запись повторно. После окна
ключ может быть новой операцией, но семантические ограничения membership/pending
всё равно действуют. Для accept даже replay заново проверяет наличие membership:
удалённое членство даёт 409 MEMBERSHIP_REVOKED, а не возвращает доступ или создаёт его.

Срок pending — 7 суток. Accept/decline проверяют recipient user ID, verified
email, session/CSRF, version, expiry, live owner membership инициатора и состояние
компании. Проверка полномочий приглашающего и изменение приглашения/членства
сериализуются с RemoveMember/изменением прав. Пока Core company lifecycle не
поддерживает удаление, существование подтверждает неизменяемый committed receipt;
добавление удаления компании потребует отдельного контракта проверки состояния.
Accept создаёт membership один раз и terminal accepted одной транзакцией.
Семантический повтор accepted/declined тем же пользователем возвращает прежний
результат, даже со старой версией; чужой actor скрыт через 404. Другой terminal
state или stale version ещё pending — 409. Revoke owner-only, без клиентской версии,
блокирует pending и переводит в revoked; accepted требует отдельного RemoveMember.
Повтор accepted после удаления financier не воссоздаёт членство: 409 MEMBERSHIP_REVOKED.

Первая terminal дата неизменяема. Для expired она равна expires_at независимо от
момента cleanup. После неё история доступна максимум 30 календарных дней; retry,
новый invite и чтение не продлевают срок. На deadline списки не содержат запись,
scoped mutation даёт 404. Физическая очистка данных/кешей/копий должна завершиться
к тому же сроку. Backup policy должна это обеспечивать; выдавать соответствие
требованию только из-за SQL-фильтра нельзя. Отдельное membership после удаления
accepted invitation сохраняется. Уточнения применения legal-docs остаются MOCK.

## Инструменты и зависимости перед этапом 3.2

Выбор для ревью, не изменение чужого Core-модуля: Go 1.27.3, PostgreSQL 18,
pgx/v5 5.11.0, goose/v3 3.29.0, x/crypto 0.58.0 (Argon2id), grpc-go 1.84.0,
protobuf-go 1.36.12. Конкретный PostgreSQL image/digest и тестовая БД проверяются
перед 3.2. Модули API/Core согласуют свой toolchain отдельно; wire от него не зависит.
Никакой код референса без выяснения права копирования не переносится: используем
описанные lifecycle/config/repository-приёмы для собственной реализации.

Buf 1.75.0; protoc-gen-go 1.36.12; protoc-gen-go-grpc 1.6.2. Локальная генерация
зафиксирована в [buf.auth.gen.yaml](../../buf.auth.gen.yaml). `Mauth/v1/auth.proto`
отображает тот же wire-контракт в internal/gen каждого потребителя; импорт Auth
internal package из API/Core запрещён. Generated files не редактируются вручную.

Из корня, с Go/Buf в PATH; `$CHECK_DIR` — новый временный каталог вне checkout:

```sh
buf format --diff --exit-code
buf lint
buf build -o "$CHECK_DIR/auth.binpb"
buf generate --template buf.auth.gen.yaml --path proto/auth/v1 --output "$CHECK_DIR"
python -m openapi_spec_validator docs/api/openapi.yaml
git diff --check
```

Generation использует зафиксированные Go generators, не BSR remote plugins.
Generated package сборка проверяется в временных модулях для Auth/API/Core с
указанными grpc/protobuf версиями. На этом этапе handlers, SQL migrations,
секреты, service deployment и runtime-тесты ещё не реализованы.

Перед переходом к 3.2 требуются: явный апрув 3.1, согласие API/Core на RPC и
стык bootstrap/UpdateCompany, принятие ADR-024/политик и тестовая PostgreSQL.
Два агентских ревью и успешная компиляция не заменяют эти решения команды.
