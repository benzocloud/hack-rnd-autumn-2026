# REST → RPC: руководство Go API

24.10.2026. Контрактная поставка: RPC определены, runtime-готовность каждого
сервиса проверяется отдельно. Ни один handler нельзя заменять фиктивным успехом.

| HTTP | operationId | Последовательность RPC | Успешный HTTP |
| --- | --- | --- | --- |
| `GET /auth/csrf` | `getCsrfToken` | `auth.v1.AuthService.GetSecurityContext` | 200 |
| `GET /auth/personal-data-consent` | `getPersonalDataConsentDocument` | `auth.v1.AuthService.GetPersonalDataConsent` | 200 |
| `POST /auth/register` | `register` | `auth.v1.AuthService.Register` | 202 |
| `POST /auth/login` | `login` | `auth.v1.AuthService.Login` | 200 |
| `POST /auth/verify-email` | `verifyEmail` | `auth.v1.AuthService.VerifyEmail` | 200 |
| `POST /auth/resend-verification` | `resendVerification` | `auth.v1.AuthService.ResendVerification` | 202 |
| `POST /auth/forgot-password` | `forgotPassword` | `auth.v1.AuthService.ForgotPassword` | 202 |
| `POST /auth/reset-password` | `resetPassword` | `auth.v1.AuthService.ResetPassword` | 200 |
| `POST /auth/logout` | `logout` | `auth.v1.AuthService.Logout` | 204 |
| `GET /me` | `getMe` | `auth.v1.AuthService.GetCurrentAccount` | 200 |
| `GET /companies` | `listCompanies` | `auth.v1.AuthService.ListMemberships` → `core.v1.CompanyService.ReadCompany` | 200 |
| `POST /companies` | `createCompany` | `auth.v1.AuthService.BeginCompanyCreation` → `core.v1.CompanyService.CreateCompanyBootstrap` → `auth.v1.AuthService.CompleteCompanyCreation` → `core.v1.CompanyService.ReadCompany` | 201 |
| `GET /companies/{company_id}` | `getCompany` | `core.v1.CompanyService.ReadCompany` | 200 |
| `PATCH /companies/{company_id}` | `updateCompany` | `core.v1.CompanyService.UpdateCompany` | 200 |
| `GET /companies/{company_id}/members` | `listMembers` | `auth.v1.AuthService.ListMembers` | 200 |
| `DELETE /companies/{company_id}/members/{user_id}` | `removeMember` | `auth.v1.AuthService.RemoveMember` | 204 |
| `GET /companies/{company_id}/invitations` | `listCompanyInvitations` | `auth.v1.AuthService.ListOutgoingInvitations` | 200 |
| `POST /companies/{company_id}/invitations` | `createInvitation` | `core.v1.CompanyService.ReadCompany` → `auth.v1.AuthService.CreateInvitation` | 201 |
| `DELETE /companies/{company_id}/invitations/{invitation_id}` | `revokeInvitation` | `auth.v1.AuthService.RevokeInvitation` | 204 |
| `GET /me/invitations` | `listMyInvitations` | `auth.v1.AuthService.ListIncomingInvitations` | 200 |
| `POST /me/invitations/{invitation_id}/accept` | `acceptInvitation` | `auth.v1.AuthService.AcceptInvitation` → `core.v1.CompanyService.ReadCompany` | 200 |
| `POST /me/invitations/{invitation_id}/decline` | `declineInvitation` | `auth.v1.AuthService.DeclineInvitation` | 200 |
| `GET /companies/{company_id}/account` | `getDefaultAccount` | `core.v1.LedgerService.GetDefaultAccount` | 200 |
| `PUT /companies/{company_id}/account/balance` | `setDatedBalance` | `core.v1.LedgerService.RecordManualBalance` | 200 |
| `GET /companies/{company_id}/categories` | `listCategories` | `core.v1.LedgerService.ListCategories` | 200 |
| `GET /companies/{company_id}/transactions` | `listTransactions` | `core.v1.LedgerService.ListTransactions` | 200 |
| `POST /companies/{company_id}/transactions` | `createManualTransaction` | `core.v1.LedgerService.CreateManualTransaction` | 201 |
| `PATCH /companies/{company_id}/transactions/{transaction_id}/category` | `updateTransactionCategory` | `core.v1.LedgerService.UpdateTransactionCategory` | 200 |
| `GET /companies/{company_id}/imports` | `listImports` | `core.v1.ImportsService.ListImports` | 200 |
| `POST /companies/{company_id}/imports` | `uploadImport` | `core.v1.ImportsService.UploadImport` | 202 |
| `GET /companies/{company_id}/imports/{import_id}` | `getImport` | `core.v1.ImportsService.GetImport` | 200 |
| `GET /companies/{company_id}/imports/{import_id}/issues` | `listImportIssues` | `core.v1.ImportsService.ListImportIssues` | 200 |
| `POST /companies/{company_id}/imports/{import_id}/commit` | `commitImport` | `core.v1.ImportsService.CommitImport` | 200 |
| `GET /companies/{company_id}/external-providers` | `listExternalProviders` | `core.v1.IntegrationsService.ListExternalProviders` | 200 |
| `GET /companies/{company_id}/connections` | `listConnections` | `core.v1.IntegrationsService.ListConnections` | 200 |
| `POST /companies/{company_id}/connections` | `createConnection` | `core.v1.IntegrationsService.CreateConnection` | 201 |
| `DELETE /companies/{company_id}/connections/{connection_id}` | `disableConnection` | `core.v1.IntegrationsService.DisableConnection` | 204 |
| `POST /companies/{company_id}/connections/{connection_id}/sync` | `syncConnection` | `core.v1.IntegrationsService.SyncConnection` | 202 |
| `GET /companies/{company_id}/payments` | `listPayments` | `core.v1.PaymentsService.ListPayments` | 200 |
| `POST /companies/{company_id}/payments` | `createPayment` | `core.v1.PaymentsService.CreateManualPayment` | 201 |
| `GET /companies/{company_id}/payments/{payment_id}` | `getPayment` | `core.v1.PaymentsService.GetPayment` | 200 |
| `PATCH /companies/{company_id}/payments/{payment_id}` | `updatePayment` | `core.v1.PaymentsService.UpdatePayment` | 200 |
| `DELETE /companies/{company_id}/payments/{payment_id}` | `cancelPayment` | `core.v1.PaymentsService.CancelPayment` | 204 |
| `GET /companies/{company_id}/payments/{payment_id}/matches` | `listPaymentMatches` | `core.v1.PaymentsService.ListPaymentMatches` | 200 |
| `POST /companies/{company_id}/payments/{payment_id}/matches` | `matchPayment` | `core.v1.PaymentsService.MatchPayment` | 201 |
| `DELETE /companies/{company_id}/payments/{payment_id}/matches/{match_id}` | `removePaymentMatch` | `core.v1.PaymentsService.RemovePaymentMatch` | 204 |
| `GET /companies/{company_id}/forecasts` | `listForecasts` | `core.v1.ForecastingService.ListForecasts` | 200 |
| `POST /companies/{company_id}/forecasts` | `createForecast` | `core.v1.ForecastingService.CreateForecast` | 202 |
| `GET /companies/{company_id}/forecasts/{forecast_id}` | `getForecast` | `core.v1.ForecastingService.GetForecast` | 200 |
| `GET /companies/{company_id}/forecasts/{forecast_id}/comparison` | `compareForecasts` | `core.v1.ForecastingService.CompareForecasts` | 200 |
| `GET /companies/{company_id}/forecasts/{forecast_id}/plan-fact` | `getPlanFact` | `core.v1.ForecastingService.GetPlanFact` | 200 |
| `GET /companies/{company_id}/scenarios` | `listScenarios` | `core.v1.ScenariosService.ListScenarios` | 200 |
| `POST /companies/{company_id}/scenarios` | `createScenario` | `core.v1.ScenariosService.CreateScenario` | 202 |
| `GET /companies/{company_id}/scenarios/{scenario_id}` | `getScenario` | `core.v1.ScenariosService.GetScenario` | 200 |

## Общий маппинг

- Существующий `docs/auth/contract.md` определяет cookie, CSRF, consent, challenge,
  privacy и rate limits. API проверяет Origin/Referer, передаёт opaque context;
  не получает роль из браузера. В Auth и Core отдельные service-local bindings.
- `AccountScope.company_id` берётся из пути, account_id — из разрешённого
  GetDefaultAccount; upload account_id дополнительно совпадает с ним. Resource
  IDs из пути/формы проверяет API и затем upstream. Source company ID/ИНН не tenant.
- Money/int64/версии парсятся из десятичных строк с bounds check без float;
  положительный version <=MaxInt64. UUID с leading zeros в INN не преобразовывать
  в числа. Timestamp в UTC RFC3339, бизнес-дата остаётся YYYY-MM-DD.
- Optional absence превращается в REST null для nullable **read**-полей.
  На write omitted означает preserve; null — clear только если REST это разрешает.
  Для PATCH name='' является переданным пустым значением и отвергается.
- Перечисления маппятся по фиксированным REST literals (например OUTFLOW → outflow);
  UNSPECIFIED/unknown в обязательном ответе — ошибка upstream, не пустая строка.
  Исключение Category.direction=UNSPECIFIED соответствует REST null.
- Repeated JSON-поля всегда [] даже для nil Go slice; nullable объект — null,
  не `{}`. Wrapper `{data:...}`/ошибка соответствуют OpenAPI; данные gRPC не
  возвращаются через автоматический protojson без адаптации.
- next_cursor='' → null. Cursor не создаётся frontend/API и не переносится на
  другой actor/company/filter/sort. Paging limits/defaults см. core contract.
- Для write передать исходный Idempotency-Key там, где он определён в REST.
  PATCH/DELETE не приобретают новое требование ключа. Не генерировать новый ключ
  при retry неизвестного исхода, не перебирать resource versions автоматически.
- DELETE cancel/remove-match/disable дают HTTP 204 без тела; внутреннюю returned
  projection API не сериализует. Повтор уже выполненного удаления следует
  описанной upstream семантике, не превращается в удаление фактов.

## Загрузка, импорт и интеграции

Multipart upload остаётся REST с file/adapter/source_company_id/account_id/currency.
API ограничивает файл 100 MiB, временно сохраняет в приватный bounded spool,
вычисляет byte_size/SHA256, затем отправляет metadata первым frame и chunks <=1 MiB.
Не передаёт локальный путь/URL. Core до записи job проверяет Auth/scope, поток,
реальную длину/digest; несовпадение/пустой stream ничего не enqueue. Изменившийся
файл под прежним ключом — IDEMPOTENCY_CONFLICT. Ограничение 300000 строк проверяет
парсер, не gRPC framing. EOF при transport error не считается успешным upload.
Отмена/timeout может оставить неизвестный исход: повтор исходного файла/ключа.

Job/file durable до 202; preview ничего не применяет. Статусы queued → processing
→ preview_ready → applied либо failed. При failed error обязателен, applied имеет
applied_data_version, preview_ready содержит preview. Прочие nullable поля остаются
null; запрещены комбинации вроде failed с applied_data_version. Issues paging
читает все diagnostics; issues_truncated не означает отсутствие остальных ошибок.
Commit требует версии preview/import, base_data_version и acknowledgement;
проверки source bindings, rows и state атомарны. Повтор applied возвращает исходный
результат, не переприменяет записи. Устаревший preview не reparses автоматически.

Provider mode=mock явно передаётся в UI; credentials реальных банков не вводятся.
Sync создаёт такой же preview-job с явными record_kinds, не commit. Disable
запрещает новые sync, не удаляет факты и не отменяет уже принятые preview jobs.
При supplementary planned_payments нельзя молча отбрасывать factual rows.

## Прогноз, график и ошибки

CreateForecast передаёт только публичные cutoff/horizon и исходный ключ; Core
выбирает/фиксирует модель. 202 — durable acceptance, не готовый расчёт.
status_url строит API из company и возвращённого forecast UUID:
`/api/v1/companies/{company_id}/forecasts/{id}`. Для ImportAccepted аналогично
`.../imports/{id}`; ScenarioAccepted.status_url ведёт на **forecast_id**, не scenario_id.
poll_after_ms >=1000. Replay возвращает исходный acceptance, GET — текущее состояние.

Forecast.oneof:

- pending: status queued/running, completed_at/result/error → null;
- succeeded: status succeeded, completed_at и result обязательны, error → null;
- failed: status failed, completed_at и error обязательны, result → null.

Пустая oneof/отсутствующие обязательные children — некорректный upstream ответ.
is_stale относится к текущему data_version и не переписывает сохранённый input/result.
Historical input/start/cutoff остаются бизнес-датами, а не «деньги сейчас».
Вероятность value=0 не теряется из-за zero-value Go: выбранная oneof.value
сохраняется; unavailable_reason преобразуется в явный JSON null для value.
Не выдумывать 0%, когда оценки нет. Для интервала нужны обе границы.

График не требует отдельной выдуманной REST-ручки: GetForecast.result.daily_points
даёт будущее, ListTransactions/датированные наблюдения и GetPlanFact — прошлое/
план-факт. Неполные actuals остаются null с coverage; нельзя рисовать их нулём.
GetPlanFact ограничен успешным несценарным прогнозом, датами его горизонта и
наблюдённым прошлым; отсутствие пересечения — 409 ACTUALS_NOT_AVAILABLE.
CompareForecasts требует совместимый snapshot/model/seed/horizon, иначе 409;
разница риска null, если хотя бы одна оценка unavailable.

Async OperationError уже содержит safe code/message/field_errors/request_id;
прогноз может успешно читаться по HTTP 200 со state=failed: не превращать ошибку
самого вычисления в ошибку transport GET. Transport CoreErrorDetail маппится
по таблице core contract; unknown enum/reason — 500, не успешный data:null.
Непредусмотренный payload status не логировать, cookie/финансовые значения не утекут.

## Что если

REST CreateScenario.oneOf выбирает независимый сценарий либо ответный. API
заполняет ровно одну protobuf-ветку independent/response. `type` каждого изменения
отображается в соответствующую ветку ScenarioChange; adjustment.mode — в
increase_basis_points/new_amount. Имя сценария отдельно; 1–16 уникальных changes.
Не копировать discriminator строкой в протокол: выбранная ветка уже определяет тип.

Ответный сценарий передаёт только parent scenario и response_action с
серверным effective_occurrence_id. Core наследует весь stress, snapshot/model/seed
и преобразования; браузер не пересылает свою копию базы/changes. В REST прочитанной
Scenario response_action восстанавливает type=reschedule_payment_part и
`target: {effective_occurrence_id: ...}`. Никаких настоящих платежей.

Delay переносит только выбранную часть позднее исходной даты; остаток остаётся.
ReduceSales действует на явные eligible receipts в периоде (1..10000 bp).
IncreaseExpenses — eligible recurring occurrences после effective_date; процент
1..100000 bp либо новая **полная** сумма выше исходной. Округление minor units
half-away-from-zero один раз на occurrence; не допускается повторная трансформация
того же economic occurrence пересекающимися changes. Outside-horizon IDs не
теряются, а явно перечисляются. Если capability false — 422 SCENARIO_NOT_SUPPORTED,
не fake scenario success. Вычисленный результат читается обычным GetForecast.
