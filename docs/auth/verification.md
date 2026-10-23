# Проверка поставки Auth этапа 3.1

Дата: 24 октября 2026 года. Состояние: контрактная поставка на ревью PR #17;
принятие политик и переход к этапу 3.2 требуют явного апрува разработчика.

## Проверяемые артефакты

- Канонический `auth.v1.AuthService`: 23 приватных RPC, отдельные Request/Response.
- `buf.yaml`: STANDARD lint, FILE breaking policy для последующих изменений.
- `buf.auth.gen.yaml`: Go generators закреплены на 1.36.12/1.6.2, отдельные
  import mappings в internal/gen Auth, API и Core, без межмодульного internal import.
- Proposed ADR-024 и единый контракт: границы, TTL, permissions, consent,
  credential_version, email outbox и recovery создания компании.
- OpenAPI 0.1.2: существующие 54 операции/100 схем; уточнения Auth не реализуют handlers.

## Выполненные проверки

1. Buf 1.75.0 format/lint/build. Успешно скомпилирован FileDescriptorSet.
2. Generation с Go 1.27.3 в отдельный временный каталог, без создания
   прикладных services в checkout. Получены bindings всех трёх потребителей.
3. Сборка generated packages через `go test ./...` в трёх временных модулях
   с grpc-go 1.84.0/protobuf 1.36.12. Результат `[no test files]` означает
   успешную компиляцию bindings, не проверку бизнес-логики Auth.
4. Структурная проверка OpenAPI 3.1 через openapi-spec-validator 0.9.0.
5. 433 примера проверены по JSON Schema с jsonschema 4.26.0/PyYAML 6.0.3;
   15 недопустимых RegisterRequest отклонены, включая отсутствие согласия,
   неверный хеш/accepted и browser-supplied consent ID/время.
6. SHA-256 примера согласия сопоставлен с исходным UTF-8 legal-документом.
7. Сверены 20 auth/account/member/invitation REST-операций с таблицей RPC,
   наличие WriteContext у Auth-мутаций, read/write oneof AuthorizeCompany,
   отсутствие client-selected user/role/evidence в credentials requests.
8. Проверены локальные ссылки новых контрактных документов и `git diff --check`.
9. После ревью отдельно сверены границы REST/RPC: обязательный joined_at,
   Idempotency-Key создания компании/создания и принятия приглашения, единое
   имя cookie и cursor ≤1024 с HTTP 400. После изменений protobuf повторены
   format/lint/build/generation и сборка всех трёх generated packages.

Инструменты Buf/Go загружены из официальных release artifacts, SHA-256 сверены
с опубликованными метаданными. Установка выполнена во временном каталоге,
системный Go и глобальные настройки пользователя не изменялись.

На базовом main до этой поставки нет protobuf. Поэтому сравнение breaking с
предыдущим Auth descriptor неприменимо; policy задана для будущих редакций,
успешный исторический breaking-check не заявляется.

## Независимые агентские ревью

Выполнены два параллельных read-only ревью: caveman-review и ponytail-review.
Первый проход выявил отсутствующий Member.joined_at, потерю Idempotency-Key
приглашений, старое имя cookie и несогласованные ограничения/ошибку cursor.
Замечания исправлены в protobuf, контракте и плане. На повторном проходе оба
ревьювера проверили обновлённый staged diff относительно main `3ddc6d9` и не
нашли новых доказанных дефектов этапа 3.1. Это advisory-результат; GitHub approval
не выставлялся. Runtime и будущие этапы этим ревью не проверены.

## Что ещё не проверено

Auth handlers, PostgreSQL migrations/repositories, runtime CSRF/mTLS, конкурентные
login/reset, email provider и Core/API E2E ещё не реализованы. Нужные сценарии и
критерии приёмки закреплены в контракте и этапах 3.2–3.6. Наличие protobuf, два
агентских ревью и компиляция не доказывают работающую авторизацию/доставку писем.

Командное принятие RPC/стыка Core и ADR-024 остаётся открытым. Ни runtime/deploy,
ни Accepted-статус политики этим отчётом не объявляются.
