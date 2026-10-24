# Go HTTP API → Auth gRPC

Запускаемый браузерный адаптер Saldo: **HTTPS → Go API → mTLS gRPC → Auth**.
Реализованы ровно десять продуктовых endpoints плюс два служебных health routes.
Контракты не изменены: [OpenAPI 0.2.1](../../docs/api/openapi.yaml),
[Auth protobuf](../../proto/auth/v1/auth.proto), [security/mapping](../../docs/auth/contract.md).

`cmd/testauth` — отдельный синтетический RPC-стенд. Он действительно принимает
protobuf по gRPC, но не является настоящим Auth: нет БД, production password
hashing, писем, consent evidence/outbox, production rate limits и полного lifecycle.
В production-приложение fixture не подключается. Handlers используют интерфейс
клиента и не зависят от fixture или его фиксированных ответов.

## Требования и единая проверка

Linux/macOS, Go **1.27.3**, C compiler для `-race`, Python 3.9+, `protoc`,
golangci-lint **2.15.0**. Нужны разрешённые loopback sockets для сетевых тестов.
Загрузки инструментов нужны только при настройке, тесты не вызывают внешние сервисы.

Из `services/api`:

```sh
python3 -m venv /tmp/saldo-api-venv
. /tmp/saldo-api-venv/bin/activate
python -m pip install -r requirements-dev.txt
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.2
export PATH="$(go env GOPATH)/bin:$PATH"
bash scripts/check.sh
```

`protoc` устанавливается пакетом `protobuf-compiler` в Debian/Ubuntu или
`protobuf` в Homebrew. Generated bindings включены в Git для обычной сборки.
Локально использован protoc 36.1; проверка generation игнорирует только строку
версии protoc в комментарии, сравнивая весь остальной результат и версии Go plugins.
Не редактировать generated code руками. Команды:

```sh
bash scripts/generate.sh          # API-local generation; Auth/Core modules не меняет
bash scripts/generate.sh --check  # обнаруживает устаревшие bindings
go build ./...
go test ./...
go test -race ./...
go vet ./...
golangci-lint fmt --diff
golangci-lint run ./...
python scripts/smoke.py --output artifacts/smoke.json
python scripts/contract.py artifacts/smoke.json
```

`check.sh` выполняет generation check, build, unit/integration/security tests,
race, coverage gate, vet, полный Go/Python lint, два отдельных процесса smoke
и проверку 20 реальных HTTP-ответов по неизменённому OpenAPI.
CI: [api-ci.yml](../../.github/workflows/api-ci.yml); только проверки, без публикации
образов/deployment. CI реагирует и на изменения Auth proto, OpenAPI/security docs.

Coverage исключает **только `internal/gen`**, включая в знаменатель команды,
адаптеры и собственный тестовый Auth/PKI. `scripts/coverage.py` объединяет повторные
записи одного блока из cross-package profile и требует ≥85% statements и ≥85%
исполняемых строк coverage-блоков. Строки — объединение диапазонов исполняемых
блоков без пустых строк/отдельных скобок/комментариев; это не процент строк всего
текстового файла. Нормализованный профиль — `coverage-own.out`.
Результаты конкретного прогона: [verification.md](verification.md).

## Ручной запуск двух процессов

В `services/api`, первый терминал:

```sh
go build -o bin/api ./cmd/api
go build -o bin/testauth ./cmd/testauth
bin/testauth -address 127.0.0.1:8082 -pki /tmp/saldo-api-demo-pki
```

Стенд генерирует свежую CA, клиентский сертификат API, серверный сертификат Auth
и HTTPS identity (ключи 0600, срок 24 часа), затем слушает gRPC только на loopback.
Не использовать эти ключи/фиксированные credentials в реальном окружении.

Второй терминал:

```sh
export API_ADDRESS=127.0.0.1:8443
export API_ORIGIN=https://127.0.0.1:8443
export API_AUTH_ADDRESS=127.0.0.1:8082
export API_AUTH_CA=/tmp/saldo-api-demo-pki/ca.pem
export API_AUTH_CERT=/tmp/saldo-api-demo-pki/api.pem
export API_AUTH_KEY=/tmp/saldo-api-demo-pki/api-key.pem
export API_HTTP_CERT=/tmp/saldo-api-demo-pki/https.pem
export API_HTTP_KEY=/tmp/saldo-api-demo-pki/https-key.pem
bin/api
```

Ctrl+C/SIGTERM дренирует HTTP и закрывает соединения. При превышении shutdown
deadline запросы отменяются, соединения принудительно закрываются, процесс сообщает
ошибку. Полный автоматический запуск/останов без ручной подготовки:

```sh
python scripts/smoke.py --output artifacts/smoke.json
```

Примеры настоящих HTTP-запросов:

```sh
curl --cacert /tmp/saldo-api-demo-pki/ca.pem https://127.0.0.1:8443/health/ready
# 200 {"status":"ok"}
curl --cacert /tmp/saldo-api-demo-pki/ca.pem -c /tmp/saldo-api-demo-cookies \
  https://127.0.0.1:8443/api/v1/auth/csrf
# 200 {"csrf_token":"<token>","expires_at":"<UTC timestamp>"}; Secure/HttpOnly cookie
curl --cacert /tmp/saldo-api-demo-pki/ca.pem https://127.0.0.1:8443/api/v1/me
# 401 {"error":{"code":"AUTH_REQUIRED",...,"field_errors":[],"request_id":"..."}}
```

Для входа взять CSRF из предыдущего ответа, сохранив cookie jar:

```sh
curl --cacert /tmp/saldo-api-demo-pki/ca.pem -b /tmp/saldo-api-demo-cookies \
  -c /tmp/saldo-api-demo-cookies -H 'Origin: https://127.0.0.1:8443' \
  -H 'Content-Type: application/json' -H 'X-CSRF-Token: <token>' \
  --data '{"email":"demo@example.test","password":"example-password-only"}' \
  https://127.0.0.1:8443/api/v1/auth/login
# 200 {"user":{"id":"...","email":"demo@example.test",...},"expires_at":"..."}
```

После login получить новый CSRF; старый контекст отозван.
Код тестового подтверждения — `001234`; reset token — `example-one-time-token`.
Стенд не отправляет письмо. Register/resend возвращают challenge handle без кода.

## Endpoints

Пути относительно `/api/v1`. Все ответы `Cache-Control: no-store`.

| Method | Путь | Успех |
| --- | --- | --- |
| GET | `/auth/personal-data-consent` | 200 document |
| GET | `/auth/csrf` | 200 CSRF + Set-Cookie |
| GET | `/me` | 200 User |
| POST | `/auth/register` | 202 challenge |
| POST | `/auth/login` | 200 Session + Set-Cookie |
| POST | `/auth/verify-email` | 200 acknowledgement |
| POST | `/auth/resend-verification` | 202 challenge |
| POST | `/auth/forgot-password` | 202 acknowledgement |
| POST | `/auth/reset-password` | 200 acknowledgement + cookie removal |
| POST | `/auth/logout` | 204 empty + cookie removal |

Валидация отвергает неизвестные/повторные/case-mismatched JSON keys, неверные типы,
лишние JSON values, invalid UTF-8 и auth bodies >16 KiB. Syntax/unknown fields →400,
неверный тип/семантические ограничения →422. Пароль передаётся без trim/нормализации;
verify code остаётся строкой с ведущими нулями. Logout не принимает body.

## Модули и безопасность

```text
cmd/api                 config + signals + composition
internal/app            HTTP lifecycle, readiness, connection ownership
internal/config         env defaults and validation
internal/client/auth    mTLS gRPC + deadlines, no automatic retry
internal/securetls      certificate chain/hostname + exact peer URI checks
internal/handler/http   routes, DTO/validation, mapping, cookie/Origin/error boundary
internal/gen/auth/v1    generated canonical protobuf bindings
cmd/testauth            standalone synthetic gRPC fixture process
internal/testauth       fixture state, failure injection/recording, ephemeral PKI
integration             real HTTPS -> mTLS RPC success/error/security tests
scripts                 reproducible generation/check/smoke/OpenAPI/coverage
```

API не проверяет password hash и не хранит сессии. Origin/Referer проверяется до
каждого POST, включая login/register/reset; отсутствие обоих или чужой Origin →403.
API требует CSRF header и передаёт исходный token в Auth, где проверка выполняется
с мутацией. Session token идёт только через cookie/private RPC, не в public JSON.
Cookies: `__Host-saldo_session; Secure; HttpOnly; SameSite=Lax; Path=/`, без Domain.
Logout/reset удаляют cookie; verify/reset не создают login автоматически.

Client IP берётся **только из прямого сетевого peer**. X-Forwarded-For, X-User-ID,
X-Role и входной request ID не считаются доверенными; request ID создаёт API.
За reverse proxy IP будет адресом proxy: доверенный forwarder/allowlist нужно
отдельно согласовать перед реальным deployment, не включать доверие ко всем XFF.
Логи содержат только известный route/status/request ID и события lifecycle.
URL/query/body/cookie/пароли/verification/reset tokens не логируются.

Ошибки переводятся из известной пары gRPC status + AuthErrorDetail.reason;
raw status messages и field descriptions не копируются. Peer denial без Auth detail,
Unavailable/DeadlineExceeded и неработающий RPC →503, без runtime mock fallback.
Поддельная CA, отсутствие клиентского сертификата и неверный URI SAN отвергаются.

## Конфигурация и передача настоящему Auth

| Переменная | Назначение / default |
| --- | --- |
| API_ADDRESS | `127.0.0.1:8080`; HTTP bind |
| API_ORIGIN | Обязательный точный HTTPS origin без path/query/userinfo |
| API_AUTH_ADDRESS | Обязательный `host:port`, без URL scheme |
| API_AUTH_CA / API_AUTH_CERT / API_AUTH_KEY | Обязательные CA/client identity для mTLS |
| API_HTTP_CERT / API_HTTP_KEY | Оба или ни одного; без них plaintext только на literal loopback за TLS proxy |
| API_RPC_TIMEOUT | 3s; предел обычных RPC |
| API_SLOW_TIMEOUT | 10s; Register/Login/ResetPassword |
| API_SHUTDOWN_TIMEOUT | 5s |

Настраиваемые timeout: 1ms–1m; родительский deadline никогда не продлевается.
HTTP write budget учитывает RPC timeout и время чтения body. Readiness делает
ограниченный read-only `GetPersonalDataConsent`, а не смотрит только на наличие
ClientConn/health Auth. Liveness не зависит от Auth; readiness не доказывает
готовность всех бизнес-операций. Auth connection восстанавливает gRPC transport,
но мутации без согласованной идемпотентности API автоматически не повторяет.

Для настоящего Auth заменить адрес и выдать сертификаты с правильными SAN
(`spiffe://saldo.local/api` клиент, `spiffe://saldo.local/auth` сервер), реализовать
имеющиеся RPC/ошибки по контракту и проверить реальный flow с PostgreSQL/email.
Handlers менять не требуется. В API отсутствуют Core/companies/финансы/ML,
миграции чужих БД, frontend переделки и production deployment.
