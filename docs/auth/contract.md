# Контракт Auth Saldo

Уточнение для изолированного демо, 24.10.2026: владелец разрешил временный
mock проверки email-кода по [ADR-028](../../.agent/decisions.md) и
[плану сдачи](../../.agent/plans/demo-integration.md). Явно включённый server-side
demo-profile принимает любой код из шести ASCII-цифр для действующего challenge
тестового пользователя; обычный/default режим проверяет секрет строго.
Пароль, сессии, CSRF, срок/привязка challenge и права компании сохраняются.
Подтверждение не доказывает владение почтой; UI обозначает mock, финансы реальны.
Доставка писем отложена из критического пути демо. Это разрешённое исключение
из строгого поведения ниже, а не утверждение готовности реализации/конфигурации;
REST/protobuf shapes и целевой полноценный Auth не меняются.

Дата: 24 октября 2026 года. Версия: 0.1.2. Статус: **принят владельцем для этапа 3.2**.
После исправлений/ревью владелец прямо поручил перейти к 3.2. Принятие этого
контракта не означает готовность Auth RPC или интеграции API/Core; каркас и SQL
проверяются отдельно в [отчёте 3.2](stage-3.2.md).

Основание: [план Auth](../../.agent/plans/auth-service-adaptation.md),
[PR #17](https://github.com/benzocloud/hack-rnd-autumn-2026/pull/17),
[API-план PR #16](https://github.com/benzocloud/hack-rnd-autumn-2026/pull/16),
[REST](../api/openapi.yaml), [ADR-027](../../.agent/adr/027-auth-contract.md).
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
откат транзакции. Пароль, cookie, CSRF, verification code, token и payload письма не логируются.

## Профиль пользователя

Вход — email/password. По [уточнению владельца в PR #17](https://github.com/benzocloud/hack-rnd-autumn-2026/pull/17#issuecomment-6377367058)
подтверждение email использует challenge и шестизначный код на 10 минут;
password reset сохраняет отдельную одноразовую ссылку. Код подтверждения не
является способом входа. Возврат к verify-link требует отдельного решения владельца.

Email: ASCII mailbox без display name, длина после trim не больше 254 символов;
trim внешних пробелов и lowercase всего адреса. Нет удаления `+suffix` или точек.
SMTPUTF8/IDN mailbox пока не поддерживаются. Уникальность по нормализованному email.
Пароль: 12–128 Unicode code points, валидный UTF-8, максимум 512 байт; без trim,
нормализации или скрытого усечения. Display name: 1–120 Unicode code points.
Детали проверки на обеих границах совпадают с REST, ошибки не содержат ввод.

IDs пользователей, consent, challenge, приглашений и команд — UUIDv4; для браузера opaque.
Новый Company ID — ненулевой UUIDv7: Auth генерирует его один раз при сохранении
durable команды. Core принимает тот же ID без изменения на каждом повторе/recovery
и не генерирует другую компанию. Прочие Auth-owned ID остаются UUIDv4.
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
его credentials/evidence не меняются и новый challenge/mail не создаётся.
Ответ Register всегда содержит публичный challenge_id и expires_at по правилам ниже.
Гонка регистраций разрешается уникальностью нормализованного email в БД, без дублей.
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
по ID → verification challenges/reset tokens по ID → onboarding/outbox. Первичный lookup без FOR UPDATE
допустим, но все условия повторно проверяются после нужных блокировок. Reset
блокирует отзываемые сессии до token row; cleanup не берёт user lock после session.

## Подтверждение, восстановление и email

### Email challenge и шестизначный код

Register для нового аккаунта атомарно сохраняет user/consent, challenge и encrypted
outbox. Challenge содержит UUIDv4 ID, user_id, purpose=email_verification,
created_at/expires_at, failed_attempts, consumed_at/revoked_at. TTL ровно 10 минут
с момента commit/создания; доставка, чтение и повтор проверки его не продлевают.
Код — строка из шести ASCII цифр `[0-9]{6}`, включая `000000`/ведущие нули:
равномерный `crypto/rand.Int` в [0, 1000000), формат `%06d`, без modulo bias.
В challenge хранится HMAC-SHA256 по purpose, challenge ID, user ID и коду;
секретный pepper/key_id вне БД/Git. Обычный SHA-256 шестизначного кода не подходит
из-за перебора при утечке БД. В покое исходный код хранится только в зашифрованном
payload outbox; в запросах он не логируется и в ответах не возвращается.

Register и ResendVerification возвращают обязательный `EmailVerificationChallenge`;
REST — `{message, challenge_id, expires_at}`. Идентификатор не является секретом
или доказательством личности; код никогда не возвращается в RPC/REST ответе.
Для существующего email при Register и неизвестного/уже подтверждённого адреса
при resend возвращаются свежий случайный inert UUIDv4 и такой же срок now+10m,
без создания challenge/mail и изменения аккаунта. Форма, текст, TTL и лимиты
ответа одинаковы; API не сообщает eligible/sent/remaining_attempts. Нельзя по
отсутствию handle определить наличие аккаунта. После перезагрузки frontend
запрашивает resend по email, а не выбирает аккаунт по user_id.

Resend для eligible аккаунта одной транзакцией отзывает прежний challenge,
создаёт новый ID/код/TTL и outbox. Письма не переиспользуют revoked challenge.
Работник перед отправкой проверяет актуальность; уже начатый HTTP send может
доставить старое письмо, но его код более не действует. Resend не сбрасывает
общий лимит попыток пользователя. Сохранённый Register retry не меняет согласие.

VerifyEmail принимает только challenge_id и code вместе с WriteContext.
Под user → session → challenge lock Auth проверяет purpose, expiry (now < expires_at),
отзыв, actor (если сессия аутентифицирована) и попытки, затем constant-time HMAC.
Каждое неверное шестизначное значение атомарно увеличивает failed_attempts;
после пяти ошибок challenge навсегда blocked. Проверка кода, счётчик, consumed_at
и email_verified изменяются одной транзакцией. Одновременные правильные запросы
дают один переход; повтор **того же кода** по consumed challenge до expires_at
возвращает 200 без повторного действия, если challenge не revoked/blocked и
общие лимиты не исчерпаны.
Неверный, неизвестный, expired/revoked/blocked challenge, неверный код и чужой
authenticated actor дают одну 422 CODE_INVALID_OR_EXPIRED; чужой actor не меняет
попытки чужого challenge. Ошибки формы полей — 422 VALIDATION_ERROR. Подтверждение
не создаёт сессию/компанию и не повышает права кроме email_verified текущего user.

### Password reset

Reset token остаётся отдельными 32 random bytes/43 base64url chars, TTL 15 минут,
purpose=password_reset, hash/user_id/expiry/consumed/revoked в БД. Forgot отзывает
прежние reset tokens; потребление и смена пароля/credential_version/session revoke
атомарны. Повтор/expired/чужой purpose или authenticated actor другого пользователя
даёт TOKEN_INVALID_OR_EXPIRED. Собственная сессия не запрещает reset; pre-auth
допускает валидный reset token. Код verify
и challenge ID никогда не принимаются вместо reset token.

### Resend HTTP API и outbox

Целевой email adapter — [Resend HTTP API](https://resend.com/docs/api-reference/emails/send-email),
`POST https://api.resend.com/emails` с Bearer из `RESEND_API_KEY`, проверкой TLS,
таймаутом 10 s и запретом redirect. `EMAIL_FROM` — настроенный отправитель на
подтверждённом домене; ключ отправки и AES/pepper keys находятся вне Git/логов.
Production/demo используют `EMAIL_PROVIDER=resend`; SMTP не является выбранным
transport. Тесты используют fake adapter/локальный HTTP stub без настоящего ключа,
внешнего send и произвольного production base URL. Provider acceptance с email ID
означает принятие Resend, а не доставку в ящик; 202 клиенту подтверждает сохранённый
intent, не provider delivery. Недоступность БД до commit даёт 503.

Request содержит from/to/subject/html/text, встроенный PNG как base64 attachment
с CID, без внешних шрифтов/скриптов. Verify-письмо показывает код; reset-письмо —
URL только из trusted frontend origin. Query reset token не пишется в access-log,
страница очищает URL после чтения, Referrer-Policy no-referrer.

Outbox сохраняется в локальной транзакции с challenge или reset/account. Secret
payload, включая замороженный provider request, шифруется AES-256-GCM со случайным
nonce, key/key_id вне БД; associated data связывает intent ID, user ID и purpose.
Рендеринг/смена template после retry не меняет сохранённое тело. Работник использует
lease/attempt state в PostgreSQL, не держит SQL-транзакцию на HTTP I/O и повторяет
тот же request с `Idempotency-Key: auth-email/<intent_uuid>`. Ключ не меняется
после рестарта/timeout и не содержит email/код. Resend хранит ключи
[24 часа](https://resend.com/docs/dashboard/emails/idempotency-keys); наше окно
retry всегда короче TTL challenge/reset, без отправки после expires_at/отзыва.

Попытки: сразу, затем через 5, 30, 120, 300 секунд с jitter 20%, максимум пять.
Network/timeout, HTTP 429/5xx и 409 concurrent_idempotent_requests допускают retry
с тем же ключом; Retry-After соблюдается, но не продлевает deadline. Остальные
4xx, включая 409 invalid_idempotent_request, — failed без смены ключа/тела.
Классификация соответствует [ошибкам Resend](https://resend.com/docs/api-reference/errors).
После max attempts/expiry — failed, без бесконечного retry. Успешный ID сохраняется;
потерянный HTTP ответ восстанавливается тем же ключом. Secret payload удаляется
после acceptance/expiry/revoke. 202 для resend/forgot остаётся generic независимо
от eligibility/provider state; неизвестный адрес не создаёт очередь.

До 3.4 нужны проверки fake/stub: timeout после provider acceptance, 429 с
Retry-After, 5xx, permanent 4xx, конкурентный worker/restart, revoked challenge.
Отдельный live smoke — только с согласованным ключом/отправителем, синтетическими
[Resend test addresses](https://resend.com/docs/dashboard/emails/send-test-emails)
и затем настоящим тестовым ящиком для ручной проверки; сейчас они не выполнены.

### Общие лимиты

Счётчики/окна хранятся в Auth PostgreSQL, атомарны и общие для всех реплик;
отказ хранилища не включает обход. Публичные reverse-proxy лимиты дополняют их.

| Операция | Лимит IP | Дополнительный ключ |
| --- | --- | --- |
| GetSecurityContext | 60/мин | Не создаёт новый контекст при действующем cookie |
| Login | 30/мин | 5/мин на нормализованный email |
| Register | 10/час | 3/час на нормализованный email |
| Resend / forgot, каждый | 10/час | 3/час на email; минимум 60 секунд между новыми письмами |
| Verify | 20/15 мин | 5 ошибок на challenge за весь TTL; 10 проверок/15 мин на user, включая resend |
| Reset | 20/15 мин | 5/15 мин на token hash |

Для Verify только общий IP bucket, проверяемый до lookup challenge/user,
возвращает 429 с Retry-After. Исчерпание challenge/user limit даёт одинаковый
422 CODE_INVALID_OR_EXPIRED без Retry-After/remaining_attempts; inert/неизвестные
handles получают ту же ошибку и не раскрывают наличие пользователя. Это явное
исключение из общего правила rate-limit, чтобы account-specific 429 не стал oracle.
Для остальных bucket 429 возвращает целочисленный Retry-After до конца окна. Для email bucket
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
| POST /auth/register | Register; challenge → 202 {message, challenge_id, expires_at}, без кода/user/session/token |
| POST /auth/login | Login; UserProfile → REST user, SecurityContext → cookie, затем новый GET CSRF |
| POST /auth/verify-email | VerifyEmail(challenge_id, code); 200 acknowledgement, повтор того же кода до expiry безопасен |
| POST /auth/resend-verification | ResendVerification(email); новый challenge → такой же generic 202 как Register |
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
в public JSON ответов не допускаются session/access/refresh/reset tokens и verification code.
Публичный challenge_id и expires_at допускаются и не подтверждают eligibility.
Публичный GET CSRF возвращает только CSRF, а Login — только REST User.

### Ошибки

gRPC status содержит AuthErrorDetail с фиксированным reason и безопасными
field violations; описание не содержит значений запроса. API переводит reason
в публичный `error.code`, а enum-prefix `AUTH_ERROR_REASON_` не выводится.

| gRPC | Причина | HTTP |
| --- | --- | --- |
| InvalidArgument | INVALID_CURSOR | 400 |
| InvalidArgument | VALIDATION_ERROR, CONSENT_REQUIRED, CODE_INVALID_OR_EXPIRED, TOKEN_INVALID_OR_EXPIRED, INVITATION_NOT_AVAILABLE | 422 |
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

1. Begin проверяет live session/verified email/CSRF. Сохраняет initiator, UUIDv4
   команды, единожды созданный ненулевой UUIDv7 компании, нормализованный spec и fingerprint под уникальным
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

Выбор для Auth, подтверждённый переходом к 3.2, без изменения Core: Go 1.27.3,
PostgreSQL 18, sqlx 1.4.0 с pgx/v5 5.11.0 только через `pgx/stdlib`
(`database/sql` driver), goose/v3 3.29.0, x/crypto 0.58.0 (Argon2id),
grpc-go 1.84.0, protobuf-go 1.36.12. После ревью 3.2 согласованные Go-правила
Core применяются к Auth: создание/config/Ping `*sqlx.DB` в `internal/app/postgres`,
`Close` у composing app; приватные repository получают готовый `*sqlx.DB`/`*sqlx.Tx`,
без native pgxpool. Репозитории разделяются по ответственности: пользовательские
операции — в `internal/repository/user`, записи БД с db-тегами — в `user/record`,
SQL — в `user/sql`, embed — в `user/queries.go`; преобразование записи в storage
projection выполняет родительский пакет. `internal/repository/postgres` содержит
общие миграции, schema check и транзакции. Интерфейсы — узкие и около потребителя.
Полные правила — `.agent/instructions.md`. Это устройство хранилища не меняет
версию wire-контракта и не означает переход к следующему этапу.

Историческая проверка 3.2 на `b102904590af9d11217aeff90ac4822104e5ed62`
использовала native pgx и PostgreSQL 18.6 из официальных EDB Windows binaries.
SHA-256 и сценарии сохранены в отчёте; они не доказывают проверку исправленного
sqlx-снимка. Его format/import/lint, unit/integration/build и race-check фиксируются
отдельно в [отчёте 3.2](stage-3.2.md). Для нового Auth CI закреплён image
PostgreSQL 18.6 в workflow; deployment image этим не выбирается.
API/Core согласуют свой toolchain отдельно.
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
указанными grpc/protobuf версиями. Каркас и SQL migrations добавлены в 3.2;
Auth handlers, consent/outbox, email delivery и runtime сценарии бизнес-операций
ещё относятся к 3.3–3.6. Секреты и deployment не входят в эту поставку.

Переход к 3.2 явно разрешён владельцем. Перед 3.3 требуется принятие результата
3.2; согласование с API/Core bootstrap/UpdateCompany остаётся перед интеграцией.
Два агентских ревью и успешные тесты не заменяют апрув следующего этапа.

## Реализация среза D1/Auth

Серверный профиль, текущие минимальные RPC, read-only поле
`GetSecurityContextResponse.email_verification_mode` / REST `CsrfToken.email_verification_mode`,
явная UI-маркировка и оставшиеся onboarding зависимости описаны в
[отчёте D1/Auth](demo-integration.md). Режим выбирает только конфигурация Auth,
не REST/protobuf request. Demo-подтверждение хранится отдельно от strict proof;
переход в strict не превращает mock в реальное владение почтой.
Статус среза и проверки фиксируются отдельно от исторического основания 3.2.
