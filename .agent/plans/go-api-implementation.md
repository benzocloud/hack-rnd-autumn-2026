# Go API: план первой реализации

Дата: 24.10.2026. Ответственный: @Prosteyshiyyy.
Статус: **Предложен для правок разработчиком; реализация не начата**.
Основание: прямое поручение разработчика в локальном чате подготовить,
задокументировать и опубликовать план. Отдельный Issue не указан.
Режим: после каждого этапа результат и проверки → «На ревью» → остановка
до явного апрува разработчика; только апрув переводит этап в «Принят».
Публикация документа не разрешает реализацию, merge или deployment.

## 1. Цель

Подготовить единственный браузерный Go API и реализовать адаптацию готового
Auth-контракта: HTTP/JSON ↔ private gRPC, cookie, Origin/CSRF и безопасные ошибки.
Результат текущего объёма — проверяемая API-часть с тестовым RPC peer.
Работа пользователя с реальным Auth/Core и финансовыми данными остаётся отдельной
интеграционной приёмкой после появления соответствующих сервисных методов.

## 2. Проверенная база и выполненная подготовка

Общая база — main
[`277af75`](https://github.com/benzocloud/hack-rnd-autumn-2026/commit/277af75b92ed6c7bc56c063e93ab333f57743f20),
PR [#24](https://github.com/benzocloud/hack-rnd-autumn-2026/pull/24).
Рабочая ветка — `prosteyshiyyy/goapi`, исходный локальный HEAD этого плана — `34fe1af`.

| Подготовка | Подтверждённый результат |
| --- | --- |
| Сверка #16/#17 и общей интеграции | Прочитаны Git-снимки, код, контракты и документы; текущие GitHub comments/checks недоступны через имеющуюся авторизацию и не объявляются проверенными |
| Синхронизация ветки | Main включён локальным merge-коммитом `34fe1af`; два собственных коммита `d5661e6` и `ae0579d` сохранены |
| Разрешение конфликта | Сохранены результаты исследования форматов; ссылки обновлены после переименования планов в Saldo |
| Проверка сохранённого импорта | `python3 -B -m unittest discover -s tests/mock_import -p 'test_*.py'`: 25 тестов, OK; HTTP-тест повторён вне sandbox, запрещавшего loopback socket |
| Чистота синхронизации | После merge рабочее дерево чистое; `git diff --check HEAD^ HEAD` без ошибок |
| Общая память | GBrain recall выполнен; сохранённые сведения проверены по текущему Git; старые статусы отдельных веток не определяют состояние main |

Эти результаты относятся к подготовке, **не к реализации этапов API ниже**.
Тесты Core/Auth/frontend в этой подготовке повторно не запускались.
Результаты чужой общей поставки перечислены в [integration-status](../../docs/integration-status.md).

| Компонент | Уже есть | Ограничение |
| --- | --- | --- |
| REST | [OpenAPI 0.2.1](../../docs/api/openapi.yaml), 54 операции | Описание операций не является сервером |
| Auth | [Контракт 0.1.2](../../docs/auth/contract.md), [proto](../../proto/auth/v1/auth.proto), Buf generation, storage и health | Business RPC, consent/outbox/email, memberships ещё не реализованы |
| Core | Четыре атомарные финансовые команды, PostgreSQL и persistent replay | Нет Core proto/transport, access adapter, company bootstrap/profile и snapshot/forecast orchestration |
| Frontend | HTTP-клиент, cookie/CSRF и mock; challenge/code уже синхронизированы | Нужны реальный backend, HTTPS и маршрутизация API |
| ML / импорт | Offline baseline; собственный локальный mock-импорт | Нет интегрированного процесса импорта/расчёта и ML RPC |
| API | [Контекст ответственности](go-api-auth-integration.md) | `services/api/` отсутствует |

Старый документ #16 задаёт границы ответственности. Его четыре этапа —
историческое предложение; настоящий файл является новым планом на согласование,
не меняет статусы Auth/Core и не принимает их следующие этапы.

## 3. Решения и границы

Применяются ADR-002/003, ADR-020, ADR-022/024 и Auth baseline ADR-027:

- Frontend → API: HTTP/JSON; API → Auth/Core: private gRPC.
- Сессии и права принадлежат Auth; финансы — Core. У API нет собственной БД
  пользователей/сессий/прав и прямого SQL-доступа к чужим БД.
- Ручная сборка через конструкторы; узкие интерфейсы у потребителей,
  приватные реализации; transport DTO/mappers отделены от protobuf.
- `__Host-saldo_session`: Secure, HttpOnly, SameSite=Lax, Path=/, без Domain.
  Один HTTPS origin. API проверяет Origin/Referer; Auth проверяет CSRF
  и права атомарно с мутацией. API не выпускает второй собственный CSRF token.
- mTLS и точные URI SAN/peer allowlist — по Auth-контракту; доверенная API identity
  `spiffe://saldo.local/api`. Ключи и CA не коммитятся; тестовая CA только для тестов.
- Денежные значения — строки копеек RUB с проверкой int64; Core IDs — UUID,
  версии — десятичные строки; Auth/source IDs сохраняют собственные форматы.
- Зависимость недоступна → безопасный отказ, без runtime-подмены моками.

Предлагаемые технические решения, подлежащие согласованию вместе с планом:
stdlib `net/http`, `slog`, Go 1.27.3 как в Core; generation использует уже
закреплённые protobuf/grpc plugins. Новые зависимости — только по необходимости.
Общий go.work/перестройка модулей не требуются этому объёму.

Референсы: startup/router/deadlines/shutdown — описание
`references/maxhack/services/gateway`; config/lifecycle — `references/auth-service`;
lint — текущая конфигурация Core и правила
[go-services](../references/go-services.md). Оригинальные reference checkout
в этом окружении недоступны; использовать проверенные описания и текущий код,
не копировать чужую предметную модель или создавать пустые слои.

Вне текущего объёма: реализация Auth/Core/ML, company orchestration,
финансовые HTTP handlers, рабочие импорты и прогнозы, смена security-профиля,
правки интерфейса frontend, production reverse proxy, сертификаты окружения,
Docker/deployment сервисов. Недостающие RPC согласуются с владельцами сервисов.

## 4. Этапы реализации

### Этап 1 — исполняемый HTTP-каркас и проверки API

**Статус: Не начат.** Зависимость: разработчик согласовал этот план и этап 1.
Результат: сервис запускается/останавливается, безопасно принимает HTTP
и имеет воспроизводимые проверки, без заявления о готовой авторизации.

Новые компоненты: `services/api/{go.mod,go.sum,README.md,.gitignore,.golangci.yml}`,
`cmd/api/main.go`, `internal/{app,config,handler/http}`;
`.github/workflows/api-ci.yml`.

- [ ] Создать тонкий main, явную сборку, config/validation, HTTP timeouts,
  ограниченный shutdown и корректное закрытие ресурсов при startup failure.
- [ ] Добавить liveness/readiness, request ID, безопасное логирование и общий
  JSON error writer; исключить тела запросов, пароли, cookie и токены из логов.
- [ ] Обеспечить `Cache-Control: no-store`, контролируемые JSON Content-Type,
  method routing, лимит auth body, отказ на неизвестные поля/лишний JSON.
  Предел auth body фиксируется отдельно и не переносится на будущие CSV uploads.
- [ ] Не объявлять Auth готовым по наличию ClientConn или его HTTP health.
  На этапе 2 подключить read-only RPC probe `GetPersonalDataConsent` к readiness;
  до подключения этого probe readiness полной auth-цепочки не заявляется.
- [ ] Настроить форматирование, модульный lint, build, unit/race/vet и CI API
  по существующим Go-практикам. В triggers учитывать Auth proto и Buf config.

Проверки: unit config/validation, startup failure, shutdown/cancellation,
malformed/oversized JSON, отсутствие секретов в error/log, build/vet/lint/race.
Критерий завершения: воспроизводимый запуск и проверки; штатный останов
освобождает ресурсы; skeleton не выдаёт успешный login или готовую сессию.
После отчёта — **остановка на ревью и явный апрув**.

### Этап 2 — private gRPC Auth-клиент и преобразование ошибок

**Статус: Не начат.** Зависимость: принят этап 1; Auth baseline повторно сверен
с текущими `auth.proto`, OpenAPI и `docs/auth/contract.md`.
Результат: API вызывает существующий wire-контракт и различает доменный отказ,
ошибку service authentication и недоступность зависимости.

Компоненты: `services/api/internal/{client/auth,gen/auth/v1,app,config}`,
`buf.auth.gen.yaml` только при необходимости, тестовые RPC fixtures.

- [ ] Генерировать API-local bindings через имеющийся Buf template;
  generated code не править вручную. Зафиксировать воспроизводимость generation.
- [ ] Реализовать узкий адаптер методов для десяти HTTP endpoints этапов 3/4,
  без преждевременного клиента company/member/financial операций.
- [ ] Подключить mTLS, chain/SAN проверки, lifecycle ClientConn и cancellation.
  Обычный deadline 3 s; Register/Login/Reset — 10 s, с учётом более короткого
  срока родительского запроса. Не повторять мутации автоматически.
- [ ] Передавать BrowserContext/WriteContext/ClientContext по proto;
  не принимать доверенные actor/role/IP из произвольных заголовков браузера.
- [ ] Преобразовать AuthErrorDetail reason/field violations и Retry-After
  по контракту. Peer denial без AuthErrorDetail → 503, не неверный пароль.
  Неизвестные upstream details дают безопасный ответ без raw status message.
- [ ] Подключить bounded read-only readiness probe; успех probe не объявляет
  реализованными остальные бизнес-RPC и не создаёт pre-auth context.

Проверки: Buf lint/generation, сборка; тестовый RPC peer с собственной CA;
валидный/чужой SAN/CA, connection failure, deadline/cancellation, status/details
mapping и отсутствие автоматического повтора записи. API CI проверяет эти gates.
Критерий завершения: вызовы соответствуют proto; Auth outage закрывает доступ,
не превращается в успех/401; fake peer существует только в тестах.
После отчёта — **остановка на ревью и явный апрув**.

### Этап 3 — cookie-контекст, согласие, CSRF и текущий пользователь

**Статус: Не начат.** Зависимость: принят этап 2.
Результат: браузерные read endpoints соответствуют REST и не раскрывают
внутренний security context.

Компоненты: `services/api/internal/handler/http/{auth,me,middleware}`,
адаптер Auth и HTTP contract tests.

- [ ] Реализовать `GET /api/v1/auth/personal-data-consent`,
  `GET /api/v1/auth/csrf`, `GET /api/v1/me`.
- [ ] Cookie material из Auth устанавливать только через Set-Cookie;
  public JSON содержит только согласованные DTO, без session token.
- [ ] Согласие возвращать полностью с исходными version/hash; API не меняет
  текст документа, не подставляет accepted/version/hash за пользователя.
- [ ] Передавать исходный opaque cookie в Auth; проверка сессии остаётся там.
  Ошибка чтения согласия/контекста не подменяется пустым успешным ответом.
- [ ] Подготовить общий Origin/Referer guard для записей: точный allowlist,
  отсутствие/чужой origin отклоняются; trusted proxies по явному allowlist,
  без доверия произвольным X-Forwarded-For и X-User/X-Role.

Проверки: HTTP → test gRPC → HTTP DTO, cookie flags/expiry, no-store;
нет cookie в JSON/log, expiry/invalid session, dependency outage,
ошибки upstream response и Origin/Referer/proxy spoofing.
Критерий завершения: три GET соответствуют OpenAPI; CSRF выдаёт Auth,
API не объявляет пользователя вошедшим по одному наличию cookie.
После отчёта — **остановка на ревью и явный апрув**.

### Этап 4 — auth-мутации и передача API-контракта фронтенду

**Статус: Не начат.** Зависимость: принят этап 3.
Результат: десять auth/account HTTP endpoints проверены на границе API
с тестовым RPC peer; ограничения реальной интеграции явно документированы.

Компоненты: `services/api/internal/handler/http/auth`, mapper/contract tests,
`services/api/README.md`, `docs/api/README.md`, `docs/integration-status.md`.

- [ ] Реализовать `POST /api/v1/auth/{register,login,verify-email,`
  `resend-verification,forgot-password,reset-password,logout}`.
- [ ] Для записей проверять Origin/Referer, наличие CSRF и структуру JSON;
  передавать контекст в Auth для авторитетной транзакционной проверки.
- [ ] Register/resend → generic 202 с challenge_id/expires_at без кода;
  verify принимает строку из шести ASCII цифр, сохраняя ведущие нули.
- [ ] Login устанавливает cookie по Auth-результату; frontend заново получает
  CSRF. Verify/reset не выполняют автоматический login; reset/logout очищают
  cookie по успешному результату с теми же Path/flags.
- [ ] Сохранить `CONSENT_REQUIRED`/422 и `CONSENT_DOCUMENT_CHANGED`/409;
  не повторять регистрацию автоматически с новым текстом согласия.
- [ ] Передать frontend адреса/DTO/error codes, настройки HTTPS одного origin,
  cookie/CSRF sequence и команды локального запуска. Отдельно описать будущие
  reverse proxy и real frontend build без включения deployment в этот этап.
- [ ] Обновить статус каждого реально реализованного endpoint; test peer
  и unit/contract checks не обозначать как реальный Auth E2E.

Проверки: HTTP contract tests десяти endpoints; 400/401/403/422/429/503 по
контракту; consent 409; session rotation, cookie deletion, неверный CSRF;
challenge/code/resend/reset mapping, malformed JSON, отмена/timeout без retry.
Fixture проверяет передачу и mapping, не доказывает бизнес-правила Auth.
Полные module lint/build/unit/race/vet и применимые Buf gates.
Критерий завершения: API-адаптация воспроизводимо проверяется без production
секретов; поддерживаемые методы и ограничения переданы frontend.
После отчёта — **итоговое ревью этапа и явный апрув**.

## 5. Отложенные зависимости и риски

| Зависимость / риск | Что требуется и где проходит граница |
| --- | --- |
| Auth пока имеет только health/storage | Владелец Auth реализует business RPC и email. Успешные тесты API с fixture не подтверждают регистрацию/доставку письма |
| Нет Core proto/access/Company API | Согласовать и реализовать Core read/create/update и проверку доступа до company/financial handlers; API не импортирует Core internal и не ходит в его БД |
| Создание компании между сервисами | Auth durable intent + Core committed receipt + Auth membership; стабильный Company UUIDv7 и Idempotency-Key, recovery после сбоя. Вынесено за этот план |
| Frontend сейчас собирается в mock для демо | Для real flow нужна отдельная сборка без mock, `/api/v1` → API в reverse proxy и HTTPS. Это отдельная согласованная поставка |
| Cookie/mTLS/CSRF | Сертификаты окружения нужны перед реальным соединением; только тестовые keys в fixture. Нельзя отключать TLS/проверку прав ради демонстрации |
| Локальный toolchain | На момент сверки Go 1.27.2, Buf и golangci-lint не обнаружены. Этап 1 фиксирует/проверяет toolchain и CI; загрузка инструментов может потребовать сетевой доступ |
| Ширина REST | 54 операции не равны объёму первой реализации. Десять auth/account endpoints входят; companies/финансы/импорт/forecast остаются будущими задачами |
| Mock-импорт и offline ML | Нужны защищённый импорт, preview/commit, Core snapshot/job и ML protobuf. Не подключать CSV/CLI напрямую к браузерному API |

После готовности Auth — отдельная реальная приёмка:
register → delivery → verify → login → me → logout/reset по HTTPS с PostgreSQL,
перезапуском и отказом зависимости. После готовности Core — отдельный план
workspace/company/member/invitation/finance интеграции и проверки чужого tenant.
Появление этих зависимостей не расширяет текущий объём автоматически.

## 6. Решения для ревью разработчика

1. Подтвердить или изменить объём: сначала десять auth/account endpoints,
   без companies/финансов и внедрения временного runtime mock.
2. Подтвердить или изменить stdlib HTTP, Go 1.27.3 и последовательность этапов.
3. Для реальной интеграции позже согласовать local HTTPS origin, доверенные
   proxy и выдачу test/demo сертификатов с владельцами инфраструктуры/Auth.

## 7. Учёт реализации

| Этап | Статус | Реализация / проверки / ревью |
| --- | --- | --- |
| 1. HTTP-каркас | Не начат | — |
| 2. Auth gRPC client | Не начат | — |
| 3. Cookie, CSRF, consent, me | Не начат | — |
| 4. Auth mutations и передача | Не начат | — |

После этапа добавить дату, затронутые файлы, commit при наличии, команды
и фактические результаты проверок, ограничения и ссылку/основание апрува.
Сохранять историю принятых этапов; расширение объёма согласовывать до кода.
Коммит/push реализации, merge и deployment требуют отдельных разрешений.

### Журнал

- 24.10.2026: выполнены сверка и синхронизация `34fe1af`, сохранены исследования
  и mock-импорт, 25 тестов импорта успешны; API не реализован.
- 24.10.2026: подготовлена первая редакция этого плана для правок разработчиком.
  Все этапы «Не начат»; заявленных результатов проверок будущего API нет.
