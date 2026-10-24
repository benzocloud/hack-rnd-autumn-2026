# Проверка Go API HTTP → gRPC

## Текущий финансовый срез — 24.10.2026

Ветка `globalarray/feat/api_financial_flow`, база `01ecfc0`; изменения локальные.
План и границы: [go-api-hour-flow](../../.agent/plans/go-api-hour-flow.md).
Последний полный scripts/check.sh завершился с exit 0 на macOS/arm64:
Go 1.27.3, pinned Go plugins/lint и Python tooling из requirements-dev.txt.
Auth/Core bindings совпадают с canonical protobuf; build/unit/race/vet/lint/Ruff
и 3 data-contract tests прошли. Coverage: statements 89.75%, block lines 90.41%.
Проверены 36 HTTP-ответов по неизменённому OpenAPI: 20 Auth через отдельные
HTTPS/mTLS процессы, 16 финансовых через HTTP handlers с typed fixtures.
Дополнительно TestFinancialHTTPThroughCoreMTLS проверяет финансовую HTTP→Core
границу на настоящей mTLS-сети: scope/WriteContext/key, accepted/poll/success,
повтор GET и недоступный Core. Это synthetic Core server, не production E2E.
govulncheck: No vulnerabilities found; go mod verify: all modules verified.

Локальные последовательные Caveman/Ponytail review завершены. Исправлен приём
финансовых путей вне /api/v1/; regression tests запрещают вызов upstream.
Настоящие Auth/Core jobs/ML, durable persistence и browser E2E этой проверкой
не подтверждены. Commit/push/merge/deployment не выполнялись.

Дополнение к текущему срезу: после исправления двух Caveman P2 (insufficient
succeeded и несогласованные метрики) полный scripts/check.sh повторно завершился
с exit 0. Непротиворечивые ready/limited и честный failed принимаются;
противоречивые first_gap/minimum/closing/max_shortfall и MinInt64 отвергаются
без исправления финансовых данных Core. Coverage: statements 89.79%,
executable block lines 90.45%; lint 0 issues, race/mTLS/OpenAPI — PASS.

## Исторический отчёт Auth API из исходной поставки

Дата: 24.10.2026. Ветка: `prosteyshiyyy/goapi`.
Проверены изменения поверх `038416f`; после синхронизации с main `31e7913`
в merge `b00a828` API generation/build/tests повторно прошли. Все D1–D8 проверены
локально. Разработчик отдельно поручил опубликовать реализацию и планы на GitHub.
Человеческое ревью этапов ожидается; merge/deployment не выполнялись.

## Фактические команды и результаты

Единый `bash scripts/check.sh` завершился с **exit 0**. Тот же скрипт указан
в `.github/workflows/api-ci.yml`. Локальное окружение: Linux/amd64,
Go 1.27.3, protoc 36.1, protoc-gen-go 1.36.12, protoc-gen-go-grpc 1.6.2,
golangci-lint 2.15.0, Python 3.14.8, PyYAML 6.0.3, jsonschema 4.26.0, Ruff 0.17.1.
Smoke и integration использовали ephemeral PKI, synthetic data и только loopback.

| Проверка | Наблюдаемый результат |
| --- | --- |
| `bash scripts/generate.sh --check` | API bindings совпадают с canonical Auth proto; публичные контракты не менялись |
| `go build ./...` | Exit 0, включая оба cmd |
| `go test ./...` | Exit 0; все unit/integration/security tests проходят |
| `go test -race -coverpkg=./... -covermode=atomic -coverprofile=coverage.out ./...` | Exit 0; race detector не нашёл гонок |
| `python scripts/coverage.py coverage.out` | Statements **574/646 = 88.85%**; executable block lines **605/674 = 89.76%**, оба ≥85% |
| `go vet ./...` | Exit 0 |
| `golangci-lint fmt --diff` | Exit 0, нет formatting/import diff |
| `golangci-lint run ./...` | Exit 0, **0 issues**, полный модуль с тестами |
| Ruff check / format scripts | Exit 0 |
| `python scripts/smoke.py --output artifacts/smoke.json` | Exit 0; 10 endpoints × успех/ошибка, отдельные процессы HTTPS API + mTLS gRPC Auth |
| `python scripts/contract.py artifacts/smoke.json` | OpenAPI 0.2.1: **20 реальных HTTP-ответов** прошли JSON Schema/format validation |
| `go mod verify` | All modules verified |
| `actionlint -shellcheck= -pyflakes= .github/workflows/api-ci.yml` из корня | Exit 0; shellcheck/pyflakes не запускались |

Coverage исключает только generated bindings. Команды и fixture входят в
знаменатель; cmd wrappers не имеют unit coverage, их запуск проверен отдельным
smoke. Метрика строк и объединение повторных coverage records описаны в README
и реализованы в `scripts/coverage.py`; обычный Go statement coverage указан отдельно.

## Матрица HTTP → gRPC

Для каждой строки integration test считает фактически полученные protobuf
запросы test Auth: увеличение ровно на один на успешный и ошибочный HTTP-вызов.
Ошибка зависимости в integration проходит по реальному RPC, не подменяется внутри handler.
В smoke часть ошибок — браузерная валидация, часть — реальные доменные/RPC отказы.

| HTTP | RPC | Smoke успех | Smoke ошибка | Integration успех + RPC ошибка |
| --- | --- | --- | --- | --- |
| GET `/auth/personal-data-consent` | GetPersonalDataConsent | 200 | 503 после остановки Auth | PASS |
| GET `/auth/csrf` | GetSecurityContext | 200 | 503 после остановки Auth | PASS |
| GET `/me` | GetCurrentAccount | 200 | 401 AUTH_REQUIRED | PASS |
| POST `/auth/register` | Register | 202 | 409 CONSENT_DOCUMENT_CHANGED | PASS |
| POST `/auth/login` | Login | 200 | 401 INVALID_CREDENTIALS | PASS |
| POST `/auth/verify-email` | VerifyEmail | 200 | 422 CODE_INVALID_OR_EXPIRED | PASS |
| POST `/auth/resend-verification` | ResendVerification | 202 | 422 VALIDATION_ERROR | PASS |
| POST `/auth/forgot-password` | ForgotPassword | 202 | 422 VALIDATION_ERROR | PASS |
| POST `/auth/reset-password` | ResetPassword | 200 | 422 TOKEN_INVALID_OR_EXPIRED | PASS |
| POST `/auth/logout` | Logout | 204, empty | 403 CSRF_INVALID | PASS |

Все пути имеют `/api/v1`. Другие продуктовые маршруты не реализованы.

## Security/lifecycle assertions

- mTLS: trusted chain/hostname и точные URI SAN; отдельные отрицательные тесты
  неизвестной CA, неизвестной client CA, неверной client/server URI, неверного
  EKU и отсутствующего клиентского сертификата. Проверка TLS не отключалась.
- Cookie flags/path/domain, session token вне public JSON, no-store/request ID;
  ротация CSRF после login, отказ со старым CSRF, выход отзывает сессию.
- Origin/Referer: foreign/null/missing/multiple origins и userinfo/suffix attack;
  same-origin Referer fallback работает. XFF/X-User/X-Role не дают полномочий.
- Неверные/лишние/case-mismatched/duplicate JSON fields, типы, пустые поля,
  oversized body, UTF-8, leading-zero verification code и отсутствие согласия.
- AuthErrorDetail/status mapping, Retry-After, redaction raw upstream message
  и field descriptions. API logs не содержат request body/cookie/password/token.
- RPC deadline, более короткий parent deadline, HTTP cancellation с наблюдаемой
  отменой server-side gRPC context, outage →503, отсутствие автоматического retry.
- Startup failures: config/CA/HTTP TLS/occupied port; обычный и TLS startup,
  live readiness probe, graceful drain, forced shutdown timeout и закрытый listener.
- Process smoke проверяет реальный запуск двух собранных binaries, HTTP+RPC TLS,
  все endpoints, отсутствие секретов в API logs и **exit 0 обоих процессов**
  после SIGTERM.

## Исправления, найденные проверками

1. Добавлена exact-key validation: стандартный encoding/json иначе принимал
   ключи без учёта регистра вопреки additionalProperties:false.
2. Неверный тип JSON отделён от syntax error: 422 VALIDATION_ERROR против 400.
3. HTTP WriteTimeout вычисляется из RPC deadlines и бюджета чтения запроса.
4. Тестовый CA содержит SKID, leaf получает AKID для строгого OpenSSL verification;
   smoke не ослабляет проверку сертификатов.
5. Замечания полного module lint исправлены без blanket exclusions.

Первый запуск сетевых тестов был запрещён sandbox (`socket: operation not permitted`);
после разрешения loopback они выполнены. Промежуточные неуспешные проверки
исправлены; итоговый единый check завершился успешно.

## Границы доказательства и передача

Настоящий Auth не проверялся: fixture не реализует production storage, password
hashing, доставку email, durable consent/outbox и полный security lifecycle.
Для его подключения нужны работающие те же RPC, совместимые status/details,
сертификаты/адрес и отдельный реальный E2E с PostgreSQL/email.

GitHub workflow создан и прошёл actionlint; результат remote GitHub Actions
этим отчётом не подтверждается. Все обязательные команды этого workflow выполнены
локально через тот же check.sh. Core, companies/finance/import/ML, frontend
переработка и production deployment в поставку не входят.

Воспроизведение: выполнить setup из README, затем **`bash scripts/check.sh`**.
