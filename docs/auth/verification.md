# Проверка поставки Auth этапа 3.1

Дата: 24 октября 2026 года. Исторический отчёт контрактной поставки `29477c3`.
После исправлений и ревью владелец прямо поручил выполнить 3.2, приняв 3.1 как
основание каркаса. Заявления ниже относятся к 3.1; актуальная реализация и проверки
каркаса/SQL — в [отчёте 3.2](stage-3.2.md).

## Проверяемые артефакты

- Канонический `auth.v1.AuthService`: 23 приватных RPC, отдельные Request/Response.
- `buf.yaml`: STANDARD lint, FILE breaking policy для последующих изменений.
- `buf.auth.gen.yaml`: Go generators закреплены на 1.36.12/1.6.2, отдельные
  import mappings в internal/gen Auth, API и Core, без межмодульного internal import.
- Proposed ADR-026 и единый контракт: границы, TTL, permissions, consent,
  credential_version, email outbox и recovery создания компании.
- OpenAPI 0.1.3: 54 операции/101 схема; уточнения Auth не реализуют handlers.

## Выполненные проверки

1. Buf 1.75.0 format/lint/build. Успешно скомпилирован FileDescriptorSet.
2. Generation с Go 1.27.3 в отдельный временный каталог, без создания
   прикладных services в checkout. Получены bindings всех трёх потребителей.
3. Сборка generated packages через `go test ./...` в трёх временных модулях
   с grpc-go 1.84.0/protobuf 1.36.12. Результат `[no test files]` означает
   успешную компиляцию bindings, не проверку бизнес-логики Auth.
4. Структурная проверка OpenAPI 3.1 через openapi-spec-validator 0.9.0.
5. 438 примеров проверены по JSON Schema с jsonschema 4.26.0/PyYAML 6.0.3;
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
10. По уточнению владельца PR #17 проверены 3 допустимых шестизначных кода
    (в том числе ведущие нули) и 18 недопустимых VerifyEmailRequest: числовой код,
    неверная длина/алфавит, отсутствие полей, лишние token/user/purpose/attempts,
    нулевой challenge ID. Сверены одинаковые generic Register/resend 202 с
    challenge_id/expires_at, отсутствие code/eligibility в ответах и сохранение
    отдельного reset token. Проверено резервирование удалённого proto token field.

Инструменты Buf/Go загружены из официальных release artifacts, SHA-256 сверены
с опубликованными метаданными. Установка выполнена во временном каталоге,
системный Go и глобальные настройки пользователя не изменялись.

На базовом main до этой поставки нет protobuf. Поэтому сравнение breaking с
предыдущим Auth descriptor неприменимо; policy задана для будущих редакций,
успешный исторический breaking-check не заявляется.

Коррекция по [замечанию PR #17](https://github.com/benzocloud/hack-rnd-autumn-2026/pull/17#issuecomment-6377367058)
меняет draft wire/API относительно `9b771e8`: VerifyEmail теперь challenge/code,
Register/resend возвращают challenge metadata; старое поле token зарезервировано.
Это согласованная миграция ещё не реализованных потребителей, а не утверждение
об обратной совместимости старого verify-link. Auth ADR перенесён с 024 на 026
без присвоения локальным Core ADR статуса Accepted. Выбор Resend проверен по
официальным HTTP/idempotency/error/test-address docs; реальные API sends не выполнялись.

## Независимые агентские ревью

Выполнены два параллельных read-only ревью: caveman-review и ponytail-review.
Первый проход выявил отсутствующий Member.joined_at, потерю Idempotency-Key
приглашений, старое имя cookie и несогласованные ограничения/ошибку cursor.
Замечания исправлены в protobuf, контракте и плане. На повторном проходе оба
ревьювера проверили обновлённый staged diff относительно main `3ddc6d9` и не
нашли новых доказанных дефектов этапа 3.1. Это advisory-результат; GitHub approval
не выставлялся. Runtime и будущие этапы этим ревью не проверены.

После корректировок владельца повторно запущены оба независимых ревьювера.
Найдены и исправлены два уточнения: account-specific Verify limit не должен
раскрывать реальность challenge через 429, а reset разрешён из собственной
сессии. В финальном контракте/OpenAPI только IP bucket до lookup даёт 429;
user/challenge limits возвращают общий 422. На повторном проходе снимка
`fff2d964…62be1ac` оба ревьювера не нашли новых доказанных дефектов.
В runtime-приёмку 3.4 включить одинаковые ответы real/inert handles, в том числе
три resend и 4+4+3 Verify attempts, а также reset из собственной/чужой сессии.
Сейчас это статическая проверка требований, не исполненный конкурентный тест.

## Что ещё не проверено

Auth handlers, PostgreSQL migrations/repositories, runtime CSRF/mTLS, конкурентные
login/reset, email provider и Core/API E2E ещё не реализованы. Нужные сценарии и
критерии приёмки закреплены в контракте и этапах 3.2–3.6. Наличие protobuf, два
агентских ревью и компиляция не доказывают работающую авторизацию/доставку писем.

Командное принятие RPC/стыка Core и ADR-026 остаётся открытым. Ни runtime/deploy,
ни Accepted-статус политики этим отчётом не объявляются.
