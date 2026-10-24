# Saldo: состояние общей поставки

Дата сверки: 24.10.2026. Этот документ описывает реализованные части и их
границы. Разработчик Go API самостоятельно составляет план исполнения.

Новый приоритет владельца: сдача **25 октября, 12:00 МСК**,
[план D1–D4](../.agent/plans/demo-integration.md) — объединить готовые части
в настоящий финансовый сценарий. ADR-028 разрешает отдельный явно включённый
mock подтверждения email в изолированном демо; его server-side реализация
ещё не проверена. Существующий общий frontend-mock не является этой интеграцией.

## Исходные PR

| Компонент | PR и исходный head | Что включено |
| --- | --- | --- |
| Go Core | [#23](https://github.com/benzocloud/hack-rnd-autumn-2026/pull/23), `9eb9e0d` | Домен Ledger/Payments, sqlx/PostgreSQL, четыре атомарные команды и persistent replay; решения Core ведущие для общей поставки |
| Auth | [#17](https://github.com/benzocloud/hack-rnd-autumn-2026/pull/17), `1bdbc21` | ADR-027, Auth protobuf/REST-контракт, каркас 3.2, схема, storage и health/readiness |
| Go API | [#16](https://github.com/benzocloud/hack-rnd-autumn-2026/pull/16), `3746e6e` | Существующий контекст Auth-интеграции и границ ответственности; собственного API-кода пока нет |
| ML | [#15](https://github.com/benzocloud/hack-rnd-autumn-2026/pull/15), `decdbf3` | Локальные Python baseline, диагностика и исследования; без HTTP/gRPC сервера |
| Frontend | [#21](https://github.com/benzocloud/hack-rnd-autumn-2026/pull/21), `25e714e` | Next.js, страницы, HTTP-клиент, browser-local mock, тесты и Docker/CI; remote `frontend` и PR head сверены, оба указывают на этот SHA |

Продолжение ML: реализованы canonical [protobuf](../proto/ml/v1/forecast.proto)
и Python gRPC-сервис `mean_90` по готовому snapshot. Есть проверка версий,
денежной арифметики, mTLS/идентичности Core и локального сетевого вызова.
Это дополняет историческую строку PR #15. Core ML-клиент проверен реальным
вызовом Python по mTLS; автоматическая подготовка 90-дневного входа и сохранение
готового прогноза ещё не реализованы. Команды и ограничения: [ML RPC](../services/ml/GRPC_CONTRACT.md).

История исходных веток сохраняется merge-коммитами. Их авторские ветки не
переписываются. Статусы принятия этапов сохраняются в соответствующих планах;
общая поставка не объявляет следующие продуктовые этапы реализованными.

## Контракты и согласованные стыки

- Канонический браузерный контракт — [OpenAPI 0.2.1](api/openapi.yaml): Core
  UUID, RUB/копейки и один подключённый счёт компании сохранены. Auth добавляет
  challenge/code, generic 202 register/resend и актуальное согласие.
- Frontend отправляет `challenge_id` и шестизначный ASCII `code`, заменяет handle
  после resend и не ожидает создания сессии после verify. При потере handle
  доступен запрос нового кода по email. Неподтверждённому пользователю доступны
  подтверждение и личный кабинет с выходом, финансовые страницы закрыты.
- Регистрация получает и показывает полный активный текст согласия. После
  `CONSENT_DOCUMENT_CHANGED` текст загружается заново, checkbox снимается;
  требуется новое самостоятельное подтверждение, без автоматического retry.
- Core resource IDs в mock-примерах/создаваемых ресурсах соответствуют UUID
  формату; внешние/provider/model-local ключи сохраняют свои строковые форматы.
- Браузер обращается только к Go API. API не получает прямой доступ к БД Auth/Core.
  Межсервисные границы — [архитектура](../.agent/plans/saldo-architecture.md),
  [Auth contract](auth/contract.md), [auth.proto](../proto/auth/v1/auth.proto).

## Что ещё требуется для реальной интеграции

| Компонент | Отсутствующий результат |
| --- | --- |
| Go API | Исполняемый сервис, HTTP handlers/middleware, cookie/CSRF/Origin, Auth/Core adapters и проверка полного browser flow |
| Auth | Бизнес-RPC регистрации/входа/подтверждения/reset, consent/outbox/email delivery, компании/членства/приглашения; этапы 3.3–3.6 не начаты |
| Core | Transport, Auth access adapter, company bootstrap/read/update и durable onboarding receipt для Auth; forecast orchestration. FinancialSnapshot (2.4) принят и находится в main; подготовлены API → Core proto и проверенный Go ML-клиент, handlers ещё не реализованы |
| ML | Подключение Python gRPC через Go-клиент к orchestration/90-дневной coverage, согласование будущего календаря без двойного учёта, обоснованная вероятность и provisioned mTLS |
| End-to-end | Реальная цепочка Frontend → API → Auth/Core → ML, provisioned TLS/credentials и согласованная проверка результата |

Текущая Core Company содержит ID; наличие таблицы ещё не даёт company profile
с name/ИНН/timezone/version. Auth health/readiness подтверждают каркас и БД,
но не работоспособность Register/Login. ML evaluation использует будущую цель
только офлайн; его нельзя подставлять вместо inference. Вероятность у baseline
не рассчитана и не должна превращаться в фиктивное число.

## Проверки и deployment

Локально проверены Core `task check`, PostgreSQL integration/race и fresh Goose
up; Auth unit/integration/race и lint; 68 ML tests, ограниченный smoke C001;
126 infrastructure tests; Python Ruff 0.17.1; frontend lint/typecheck и тесты.
Структурная проверка OpenAPI: 54 операции, 103 схемы, 866 примеров, 820 локальных
`$ref`. Эти проверки не заменяют отсутствующий backend E2E и не доказывают
численные выводы всех ML-исследований.

CI отдельно запускает ML unit tests и PostgreSQL Core integration tests.
Frontend при push в `main` публикует mock-образ в GHCR; SSH deployment разрешён
только явным `workflow_dispatch` на `main`. Общий merge не обновляет сервер.
Core GHCR runtime publish остаётся пропущен до появления `cmd/core`.
Инфраструктура Fadwix в этой поставке не меняется.


### Объединение Core и ML — 24.10.2026

По поручению владельца PR #27 (`994ed6e`) включён обычным merge в ветку PR #29.
ML Predict дополнен финансовой реализацией и DTO; отдельного ML RPC нет.
Полный API → Core контракт сохраняет все 54 REST operationId.
Конфликт документации разрешён с сохранением принятого FinancialSnapshot 2.4.
Сеть/handlers/Auth/bootstrap и финансовый E2E этим объединением не объявляются
готовыми. Runtime provisioning/deployment не выполнялись.

Свежая финансовая модель ML из PR #27 `49a7862` включается в ту же поставку.
Два исходных endpoint сведены к одному Predict с явной версией модели;
calendar replacement/components и empirical bands проверяются отдельно.
Вероятность не откалибрована, Core worker/HTTP orchestration ещё не подключены.
