# D1/Auth — минимальный интеграционный срез

Статус: В работе; после проверок — ревью владельцем. Основание: прямое поручение
владельца и ADR-028, срок сдачи 25.10.2026 12:00 МСК. Это один срез D1,
не завершение всей цепочки и не принятие старых этапов 3.3–3.6.

## Результат и границы

Приватный Auth предоставляет API `GetPersonalDataConsent`, `GetSecurityContext`,
`Register`, `Login`, `VerifyEmail`, `ResendVerification`, `Logout`,
`GetCurrentAccount`, `AuthorizeCompany`, `ListMemberships`. Регистрация не создаёт
сессию/компанию; VerifyEmail отвечает acknowledgement, после него нужен Login.
Членства не создаются из пользовательского company_id или роли.

Хранилище — sqlx/PostgreSQL, пул принадлежит app. Новая миграция 2 добавляет
серверное evidence согласия и членства, хранит происхождение email-подтверждения
и общий бюджет ошибок пользователя. Сохраняются настоящие Argon2id, opaque
серверные сессии, credential version, stable synchronizer CSRF и company scope.
HTTP-ответы/Origin/Referer/Secure HttpOnly cookie остаются у API.

## Профиль и общий параметр для API/frontend

`AUTH_DEMO_EMAIL_VERIFICATION=false` по умолчанию. Включение разрешено только
для `AUTH_ENV=local`, с `AUTH_GRPC_ADDRESS` и точным `AUTH_DEMO_TEST_EMAILS`:
непустой список нормализованных синтетических адресов в домене `.test` без
wildcard. Например, `alice@example.test,bob@example.test`. Нужна отдельная
изолированная БД с тестовыми аккаунтами; реальные персональные данные не использовать.

В demo любой код `[0-9]{6}` принимается только для активного challenge
allowlisted пользователя. Purpose, user binding, expiry, revoked/blocked,
аутентифицированный actor и CSRF всё равно проверяются. Чужой actor не тратит
попытки другого пользователя. Default strict сравнивает HMAC constant-time.
Пять неверных попыток пользователя за 15 минут сохраняются между resend.

В БД сохраняется `email_verification_method=demo`. При последующем strict-режиме
такой результат не признаётся настоящим владением почтой: финансовый доступ
закрыт до строгого подтверждения. VerifyEmail не выдаёт новую cookie/сессию.

`GetSecurityContextResponse.email_verification_mode` — ответное поле `strict|demo`.
API должен передавать его в `GET /auth/csrf` как `email_verification_mode` рядом
с `csrf_token` и `expires_at`. **Cookie material не попадает в JSON.** Имя закреплено
в Auth protobuf, OpenAPI CsrfToken и frontend type; API-adapter должен подключить
его по этому контракту. Подтверждение согласования другим человеком не заявляется.
Браузер не выбирает режим: такого поля нет в запросах и use-case inputs.

На форме регистрации/подтверждения показано «Демо: подтверждение замокано» по
серверному ответу с пояснением, что это не доказательство владения почтой.
При ошибке загрузки режима подтверждение блокируется с retry. Никакого включения
глобального frontend developerMode=mock для финансовой интеграции не требуется.

## Запуск приватного RPC

Задать `AUTH_GRPC_ADDRESS`, `AUTH_GRPC_CERT_FILE`, `AUTH_GRPC_KEY_FILE`,
`AUTH_GRPC_CA_FILE`, `AUTH_CODE_HMAC_KEY`, `AUTH_CSRF_HMAC_KEY`; см.
[пример окружения](../../services/auth/.env.example). Ключи — независимые 32 случайных
байта в raw base64url, без padding; хранить вне Git. CA и service private keys
выдаются оператором вне репозитория. Без AUTH_GRPC_ADDRESS остаётся только
прежний health listener; health 200 сам по себе не означает готовый Auth RPC.

RPC использует TLS 1.3 и обязательный проверенный client certificate. API URI SAN
`spiffe://saldo.local/api` может вызывать браузерные операции; Core
`spiffe://saldo.local/core` — только AuthorizeCompany/AuthorizeCompanyBootstrap.
Произвольные поля/заголовки не выбирают service identity. Reflection не включён.
Не реализованные RPC отвечают Unimplemented; это не успешные заглушки.

## Минимум, необходимый API, и Core onboarding

| Потребитель | Минимум этого среза |
| --- | --- |
| Формы | GetPersonalDataConsent, GetSecurityContext, Register, Login, VerifyEmail, ResendVerification |
| Сессия | GetCurrentAccount, Logout; API устанавливает/ротирует cookie и заново получает CSRF |
| Финансовые маршруты | AuthorizeCompany на каждом защищённом чтении/изменении; мутации передают WriteContext; ListMemberships для выбора компании |
| Core | Сам вызывает AuthorizeCompany с реальным company scope; положительный результат API не заменяет проверку Core |

Для пользовательского создания компании ещё требуются BeginCompanyCreation,
AuthorizeCompanyBootstrap, CompleteCompanyCreation и настоящий Core adapter:
создание Company/BankAccount с выданным Auth UUIDv7, durable receipt по command_id,
чтение receipt и защищённые company read/update. Без подтверждённого Core receipt
Auth не создаёт owner membership. Для изолированного интеграционного теста
используется только операторский fixture membership к совпадающему Core company ID;
это не реализованный пользовательский onboarding и не browser-доступ к Auth DB.

Email delivery/outbox, forgot/reset, приглашения и расширенный onboarding остаются
в целевом плане. Строгая проверка кода реализована, доставка и полный strict email
E2E этим срезом не заявляются. Полные распределённые IP/method rate limits, rotation
ключей, deployment и API → Core → ML → browser E2E проверяются отдельно.

## Проверки и точка ревью

План проверки: demo/strict, ASCII и leading zeros, foreign/expired/revoked/blocked
challenge, replay, без автологина, реальные пароль/CSRF/session revocation,
company/role scope, consent version/hash/evidence/rollback, persistence и конкуренция.
PostgreSQL — отдельная auth_test со случайной schema на каждый тест.
Дополнительно: реальный mTLS positive API/negative Core и unknown peer, structured
ошибки без секретов, Buf/OpenAPI, полный Go lint/vet/build/tests и frontend checks.

Фактические результаты и ограничения будут записаны после выполнения команд;
план/чек-лист не является утверждением успешного прохождения тестов.
После среза — два независимых caveman-review/ponytail-review и ревью владельцем.
Дальнейшее расширение/merge/deployment этим отчётом не разрешаются.
