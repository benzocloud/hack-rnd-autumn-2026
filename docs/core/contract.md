# API → Go Core: первый контракт

Дата: 2026-10-24. Статус: подготовлен для ревью; **серверные handlers не
реализованы**. Основание: REST `docs/api/openapi.yaml` 0.2.1, ADR-002/003/022/024,
Auth ADR-027 и `proto/auth/v1/auth.proto`. Ответственность за Core proto принял
владелец в текущем обсуждении. Это не замена всему REST и не работающий API.

Канонические файлы: `proto/core/v1/common.proto`, `company.proto`, `ledger.proto`,
`payments.proto`, `errors.proto`. Отдельные сервисы здесь — gRPC-группы внутри
одного Core-процесса, не новые микросервисы. Референсы gateway/engine-go помогают
с границей handler → service и ручной сборкой; предметные DTO определяются нашим
REST. Service не импортирует generated protobuf, SQL остаётся в repository.

## Потребители, доступ и деньги

- Только приватный mTLS с точными URI SAN `spiffe://saldo.local/api` и
  `spiffe://saldo.local/auth`; Core при вызове Auth использует
  `spiffe://saldo.local/core`, как в ADR-027. API может вызывать обычные company/financial RPC и
  bootstrap; Auth — bootstrap/recovery и `GetCompanyCreationResult`. Прочие peer
  и методы отвергаются. Service identity берётся из сертификата/allowlist.
- API передаёт исходный opaque BrowserContext/WriteContext из Auth. Ни actor ID,
  ни роль, ни внешний ИНН не подтверждают доступ. До чтения/изменения компании Core
  вызывает Auth AuthorizeCompany; ActorID в сервисную команду берёт из ответа.
  Недоступный Auth блокирует операцию. У writes обязательны WriteContext/CSRF;
  API дополнительно проверяет Origin/Referer. Логи не содержат cookie/CSRF.
- Auth-проверка предшествует чтению финансов, в том числе возврату replay receipt.
  Она подтверждает разрешение на момент проверки: общей атомарной транзакции
  Auth/Core нет. Отзыв прав не отменяет уже разрешённую финансовую транзакцию;
  runtime-тесты должны проверить порядок и отсутствие обхода этой границы.
- Ненулевые canonical UUID, новые Core ID — UUIDv7. `account_id` должен
  принадлежать `company_id`; MVP допускает один основной счёт, а не совокупность
  всех денег бизнеса. REST не принимает account_id: API получает его из
  GetDefaultAccount и не доверяет произвольному браузерному значению.
- Money — signed int64 копейки, только RUB; для потоков > 0, для остатка допустим
  отрицательный результат. Версии положительные int64. API переводит числа в
  десятичные строки JSON без float и без JavaScript Number; переполнение — 422.
- Бизнес-даты YYYY-MM-DD в Europe/Moscow, timestamps — UTC. Отсутствующие message/
  optional поля означают отсутствие, а не нулевой остаток/пустую строку.
  Неизвестные enum и UNSPECIFIED во входных командах отвергаются.

## REST → RPC

| REST operationId | RPC | Auth permission |
| --- | --- | --- |
| createCompany | Auth Begin → Core CreateCompanyBootstrap → Auth Complete | Auth durable bootstrap command |
| getCompany | CompanyService.ReadCompany | READ |
| updateCompany | CompanyService.UpdateCompany | UPDATE, WriteContext |
| getDefaultAccount | LedgerService.GetDefaultAccount | FINANCIAL_READ |
| setDatedBalance | LedgerService.RecordManualBalance | FINANCIAL_WRITE |
| listCategories | LedgerService.ListCategories | FINANCIAL_READ |
| listTransactions | LedgerService.ListTransactions | FINANCIAL_READ |
| createManualTransaction | LedgerService.CreateManualTransaction | FINANCIAL_WRITE |
| listPayments | PaymentsService.ListPayments | FINANCIAL_READ |
| getPayment | PaymentsService.GetPayment | FINANCIAL_READ |
| createPayment | PaymentsService.CreateManualPayment | FINANCIAL_WRITE |
| listPaymentMatches | PaymentsService.ListPaymentMatches | FINANCIAL_READ |
| matchPayment | PaymentsService.MatchPayment | FINANCIAL_WRITE |

Auth recovery отдельно вызывает GetCompanyCreationResult. Company list собирает
API из разрешённых Auth ListMemberships и Core ReadCompany; Core не выдаёт список
всех компаний. Membership/role возвращает Auth, не Core persistence.

CreateCompanyBootstrap сверяет command/company/spec/hash с результатом
AuthorizeCompanyBootstrap. Auth CompanyCreateSpec переиспользуется без собственной
версии нормализации. Receipt фиксирует **исходный** spec/hash/default account и
committed_at атомарно с Company/BankAccount; изменение имени компании позднее не
переписывает receipt. Повтор возвращает тот же результат, несовпадение — конфликт.
GetCompanyCreationResult доступен только Auth, NotFound означает отсутствие
committed receipt; API не может подтвердить успех boolean. Auth создаёт membership
сам после чтения receipt. Строго соблюдать fingerprint и recovery из
[Auth contract](../auth/contract.md#создание-компании-и-контракт-стыка-core).

UpdateCompany: `name` omitted сохраняет имя; INN oneof отсутствует — сохранить,
`inn` — заменить, `clear_inn` — очистить. Currency/timezone/owner неизменяемы.
Пустой patch отвергается. REST не требует Idempotency-Key для PATCH; stale version
отвергается, нельзя безусловно повторять PATCH с новой версией.

Ручные финансовые creates/match используют исходный Idempotency-Key
(8–128 символов `[A-Za-z0-9._:-]+`). Scope: actor + company/account + operation +
resource + semantic payload. Replay с тем же payload возвращает исходный результат
до stale-version guard; иное тело — IDEMPOTENCY_CONFLICT. ID/время генерирует Core,
не API при повторе. Не следует увеличивать data_version на replay.
Текущие сервисные receipts содержат ID/версии: для полного REST-ответа понадобится
сохранить исходную projection/receipt или восстановить её без подмены исходных
версий текущими. Прочитать изменившийся payment и выдать его за исходный ответ
создания недостаточно. Это задача реализации handlers/receipts.

Manual transaction связывает историю со source manual_v1, не пересчитывает
остаток. Manual payment не связывает источник фактов и не создаёт transaction.
Manual EOD — датированное наблюдение с note (1–500), не вымышленный поток; будущая
дата и конфликтующее наблюдение того же дня отвергаются. При replay старого EOD
результат не должен подменяться текущим состоянием счёта. Read GetDefaultAccount
наоборот возвращает последнее наблюдение и честную дату; при отсутствии — null.

Match использует REST payment_version и transaction_version: одновременно
проверяет оба ресурса под lock, направление/счёт/остатки и сумму распределения.
Всё выполняется в одной Tx; один факт не распределяется сверх его суммы. Повтор
не должен создавать дополнительное сопоставление. Это не перевод денег.

## Чтения и ограничения

Пагинация: default 50, present limit 1–100, cursor <= 1024 ASCII; cursor привязан
к actor/company/filter/sort и после изменения scope не используется. Сортировки:
transactions `operation_date DESC,id DESC`; payments `scheduled_date ASC,id ASC`;
categories `name ASC,id ASC`; matches `created_at ASC,id ASC`. `next_cursor=""`
отображается в JSON null. Даты from/to включительные; отсутствие — без границы,
присутствующие пустые даты и from > to недопустимы. Default payment status —
planned/partially_matched; settled/cancelled читаются явным фильтром. Nullable
category_id маппится в absent; переданная пустая строка — ошибка.

Ограничения строк и nullable берутся из соответствующих схем REST: имя компании
1–160; INN 10/12 ASCII цифр с сохранением ведущих нулей; counterparty_name <=500,
purpose/description <=2000. RPC-валидатор применяет ограничения до записи, а
публичный API дополнительно проверяет форму JSON. Proto сам не валидирует эти
условия. Read projection Source сохраняет file/manual/mock_external, import_id и
connection_id; manual write не принимает ни один из этих признаков извне.

## Ошибки и таймауты

`CoreErrorDetail` прикладывается к non-OK gRPC status. gRPC message безопасный;
SQL/credentials/финансовые значения остаются за границей публичного ответа.

| reason | gRPC | HTTP |
| --- | --- | --- |
| VALIDATION_ERROR | InvalidArgument | 422 |
| INVALID_CURSOR | InvalidArgument | 400 |
| AUTH_REQUIRED | Unauthenticated | 401 |
| FORBIDDEN, CSRF_INVALID, EMAIL_UNVERIFIED, MEMBERSHIP_REVOKED | PermissionDenied | 403 |
| NOT_FOUND | NotFound | 404 |
| VERSION_CONFLICT | Aborted | 409 |
| IDEMPOTENCY_CONFLICT | AlreadyExists | 409 |
| FACTUAL_SOURCE_CONFLICT, SOURCE_BINDING_CONFLICT, BALANCE_OBSERVATION_CONFLICT, PAYMENT_STATE_CONFLICT | FailedPrecondition | 409 |
| RATE_LIMITED | ResourceExhausted | 429 |
| DEPENDENCY_UNAVAILABLE | Unavailable | 503 |
| INTERNAL_ERROR | Internal | 500 |

Field violations содержат путь/безопасное описание, не исходное значение.
Отмена клиента — Canceled, дедлайн — DeadlineExceeded (API 504, если соединение
ещё открыто); изменение могло успеть commit, поэтому retry с тем же ключом и
телом, без обещания rollback из-за сетевого timeout. Для PATCH сначала перечитать
состояние. Адаптер явно задаёт бюджет: обычные RPC <=10 секунд, ML <=30 секунд;
более короткий parent deadline побеждает. Вызов ML выполняется вне SQL Tx.
Повтор writes новым ключом после неизвестного исхода запрещён. До runtime-сервера
эти бюджеты — спецификация, не проверенная конфигурация deployment.

## Непокрытая реализация и следующие поставки

Первый proto не включает import/commit, category update, payment update/cancel/
unmatch, forecast jobs/results/chart, scenarios, connectors. Это остаётся в REST
и P0/P1-плане; не считать отсутствие RPC удалением бизнес-требования.
Не добавляем generic JSON passthrough, fake success, брокер или временный обход Auth.

Перед подключением API нужны отдельные проверки/реализация:

1. Company metadata/default account/bootstrap receipts: нынешняя core_companies
   содержит только ID; добавить миграцию/domain/services/repositories и recovery.
2. Get/list projections, provenance и created_at: нынешний TransactionData не
   хранит created_at/import_id/connection_id; нельзя подставлять time.Now на read.
3. Входы Payments уже адаптированы локально: manual CreateCommand допускает
   ExpectedDataVersion=0, MatchCommand при ExpectedDataVersion=0 требует
   положительную ExpectedTransactionVersion и проверяет её вместе с версией
   ожидания. Wire handler должен передавать исходные обе версии REST, не
   изобретать browser field и не считать прочитанную API версию версией клиента.
   Положительный account CAS сохранён для старых внутренних команд; импорт
   ожиданий по-прежнему требует его. Старые JSON/fingerprint receipts сохранены
   через omitempty нового поля; для новых match версия факта входит в digest.
4. Полный durable replay результата вместо гидрации из изменившихся строк.
5. mTLS/Auth adapters, status mapping, запросы с чужим company/account, revoked
   member/CSRF/failure и lifecycle. Сборка схем этого не доказывает.
6. Дневная подтверждённая coverage для ML, jobs/persistence и календарь без двойного
   учёта. PR #27 — history-only baseline; не способен закрыть эти требования один.

## Генерация и проверка

Buf **1.75.0**, local plugins `protoc-gen-go v1.36.12` и
`protoc-gen-go-grpc v1.6.2` (версии закреплены в `buf.core.gen.yaml`):

```sh
buf lint
buf format --diff --exit-code
buf build --as-file-descriptor-set -o /tmp/saldo-contract.binpb
buf breaking --against '.git#branch=main'
buf generate --template buf.core.gen.yaml
```

Core получает собственные auth/core/ml bindings в internal/gen. Другие сервисы
генерируют локальные пакеты; импортировать `services/core/internal/gen` из API
или Auth нельзя. `scripts/check-core-contracts.sh` компилирует копии bindings
в отдельных временных API/Auth modules, проверяя import mappings, не меняя
разработку других участников. Это не runtime-тест финансового сервера.

## Ревью текущей поставки

После проверок выполнены caveman-review и ponytail-review. Исправлены
расхождения первоначального черновика: удалён Idempotency-Key из UpdateCompany,
MatchPayment передаёт обе REST resource versions, CreateManualPayment не требует
несуществующего публичного data_version. Убраны лишние is_overdue/direction filter
из платежных DTO; сохранены действующие REST status/date filters. Обнаруженные
различия service inputs и wire-контракта перечислены выше как следующие задачи,
а не выданы за реализованные проверки. Generated bindings не правились вручную.


Payments inputs проверены отдельно: весь Core PostgreSQL integration/race прошёл;
дополнительный concurrent test подтвердил отсутствие over-allocation при
ExpectedDataVersion=0 и обеих resource versions. Совместимость receipts_v1,
повтор без записи и digest conflict при изменении версии факта — PASS.
Это проверка сервисов/storage; сетевые handlers остаются следующим этапом.
