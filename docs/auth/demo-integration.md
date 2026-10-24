# D1/Auth — минимальный интеграционный срез

Статус: На ревью владельцем. Основание: прямое поручение
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
API передаёт его в `GET /auth/csrf` как `email_verification_mode` рядом
с `csrf_token` и `expires_at`. **Cookie material не попадает в JSON.** Имя закреплено
в Auth protobuf, OpenAPI CsrfToken, API DTO и frontend type; передача через
настоящий API проверена по HTTPS/mTLS. Подтверждение согласования другим человеком не заявляется.
Браузер не выбирает режим: такого поля нет в запросах и use-case inputs.

На форме регистрации/подтверждения показано «Демо: подтверждение замокано» по
серверному ответу с пояснением, что это не доказательство владения почтой.
При загрузке, ошибке или отсутствии/неизвестном значении режима регистрация
и подтверждение блокируются с retry. Никакого включения
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

Фактически выполнено для этого среза:

- Полный Auth lint — **0 issues**; format/import checks, vet с integration tag,
  build и unit/integration с shuffle прошли. PostgreSQL 18.6 — отдельная
  `auth_test`, случайные схемы; исходная БД не удалялась.
- Пройден реальный protobuf → service → PostgreSQL flow, включая consent
  rotation/evidence, строгий HMAC, demo-proof isolation, budget между resend,
  session rotation/logout, CSRF и owner/financier/foreign company scope.
- Настоящие процессы **HTTPS API → mTLS Auth → PostgreSQL 18.6** прошли
  регистрацию, demo acknowledgement без автологина, неверный/правильный пароль,
  ротацию сессии, Origin/CSRF/logout, restart в strict, запрет включения demo
  query-параметром и отклонение неверного шестизначного кода в strict.
  Отдельный API mode test подтверждает отсутствие cookie material в JSON.
- API lint — **0 issues**; HTTP handler tests, vet и build прошли. Полный API
  unit-run на Windows остановился только на существующем Unix permission test
  `TestEphemeralPKIIdentityChainAndPermissions` (проверка `0600`); это ограничение
  среды не выдаётся за успешный полный API unit-run.
- Frontend: **17/17 tests**, Next typegen, TypeScript и полный ESLint прошли.
  После merge сохранены актуальные ссылки согласия из main и наш UI mode guard.
- Buf format/lint/build, OpenAPI validation и воспроизводимость Auth bindings
  с закреплёнными генераторами проверены. `govulncheck`: 0 reachable findings;
  есть одна запись только на уровне required module без вызова уязвимого кода.
- Два независимых caveman-review и ponytail-review выполнены. Исправлены
  замечания о status согласия, malformed metadata, cursor/order, CSRF formula,
  Argon2 `p=1` и гонке первоначального CSRF в UI. Финальная перепроверка Auth
  и API mode delta открытых замечаний не оставила. Это не GitHub approval.

Локальный race недоступен без CGO/C compiler; для нового SHA результат Linux CI
нужно читать отдельно после публикации, старые CI за новый код не выдаются.
Нет утверждения о production provisioning, реальной email delivery или полном
browser → Core → ML финансовом E2E. Core onboarding/receipt остаётся зависимостью.

Перед публикацией влито `origin/main` `02ec65c1c136594755c11def50100921551bd8bc`
обычным merge. Единственный конфликт — `docs/integration-status.md`; сохранены
актуальные API/Core/ML сведения и фактический D1/Auth status. Published history
не переписывалась. Перед push/PR fetch и ancestor check повторяются.
После среза требуется ревью владельцем; дальнейшие этапы, merge и deployment
этим отчётом не принимаются и не разрешаются.
