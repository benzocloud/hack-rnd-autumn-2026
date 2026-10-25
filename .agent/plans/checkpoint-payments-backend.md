# Payments и frozen calendar — D2.5

Цель: реальный lifecycle платежей API → Core → PostgreSQL и ровно однократный учёт remaining в единственном Predict. Owner явно разрешил автономные этапы, commit/push и один draft PR; промежуточное review отложено. Merge/ready/deploy запрещены.

Исходная база: origin/main 0abae4f. Домен payments, CreatePayment/MatchPayment, SQL и canonical PaymentsService существуют. gRPC payments и REST payments не подключены; read/update/cancel/unmatch service ещё отсутствуют. Snapshot codec и чтение календаря существуют, но runtime совместимость требуют проверки. #47 head 7e5c0d4aff78286c13d9a8245f801e8a5b82e16c — опубликованная CSV/manual зависимость; пока не интегрирована.

Решения: ADR-029/030, точные RUB-копейки, account lock/CAS/replay, отдельные reconciliation groups, immutable snapshots, explicit unsupported profile. Ориентир Go: consumer interfaces/manual wiring cosmohack из .agent/references/go-services.md; локальные originals пока не проверены.

## Этапы
1. Матрица и контрактный handoff — На ревью. Проверены source tree, PR #47 body/discussion/head. Критерий: опубликованный draft и remote SHA.
2. Lifecycle — На ревью. services/core и services/api payments adapters/services/repositories; reuse domain/create/match, добавить reads/CAS update/cancel/unmatch/history. Проверки: unit/race, real marked PG, Auth/scope/CSRF/replay/overmatch.
3. Frozen calendar — На ревью. snapshot capture/codec/worker, independent groups/provenance/availability/mode одной Tx. Старые codecs/results сохраняются. Проверки: restart/immutability, proof carry только без изменения фактов.
4. Единственный Predict — На ревью. Минимальный ML adapter/calendar regression; совместимый version/profile без default/training изменений. Независимая Go arithmetic validation, source IDs/groups/overflow/overdue. Проверки: реальные mTLS 30 точек и negative scenarios; CSV проверяется отдельно.
5. Итоговая интеграция и review — На ревью. Full module tests/race/vet/lint, pinned Python/Ruff, Buf/drift/schema, independent caveman/ponytail на конкретном SHA, fixes, main compatibility/actual CI. Все стадии остаются На ревью до owner approval.

Границы: frontend/deploy/Auth/onboarding/training/чужие handoffs не меняются. Миграции только при необходимости. Риск: несовместимость CSV profile/calendar должна давать явный отказ; не маскировать profile. Изолированный PG18 внутри контейнера, marker обязателен; serial packages при общей test DB.

Проверенная реализация и ограничения находятся в handoff. CURRENT covered сценарий работает; CSV/calendar и synthetic остаются explicit unsupported. Owner acceptance не запрашивался/не утверждался.

Финальный implementation checkpoint: ac90fa3; свежий main6c9edd9 интегрирован. Все локальные gates и независимые reviews завершены; upstream CI на фиксацию pending, не passing. Подробности/repro/limitations и owner pickup — .agent/handoffs/checkpoint-payments-backend.md.

## Продолжение A: ordinary CSV + CURRENT calendar (25.10, поручение владельца)

Это явно разрешённая доработка основного пути, не достаточный blocker-report.
Предыдущие реализации/gates/review сохраняются; owner review отложен, новые
стадии остаются На ревью после выполнения. Merge/ready/deploy запрещены.

Принятый для этого slice context: model_name=mean_90,
model_version=mean_90_csv_calendar_v1,
preprocessing_version=csv_observed_operations_calendar_v1,
configuration_id=mean_90_csv_calendar_v1. Это отдельный профиль, не переименование
CSV-only/covered/trained расчёта. Trained default/model/training не меняются.

Совместимость: прежние mean_90_csv_v1 и mean_90_components_v1 и schema1/2 читаются
с прежней семантикой. Новый schema2 job замораживает assessment+CURRENT+context;
выбор нового profile при существующих payments/groups и допустимом CURRENT
cutoff. До календаря обычный CSV-only сохраняет oldprofile. Никаких proof/fixture
allowlist/SQL facts, history не certified. Eligibility90days/EOD/reconciliation/
unknown days/manual provenance/limits прежняя. Версия результата не переиспользуется
при новой revision; worker не читает live calendar. Proto/REST shape не меняются.

Новый limitations list: csv_history_completeness_unverified,
unobserved_days_unknown, observed_operations_mean_per_calendar_day,
historical_availability_unverified, not_calibrated, current_calendar_only.
Probability unavailable/null, bands отсутствуют; unknown не превращается в zero.

6. CSV+CURRENT happy path — На ревью: shared CSV eligibility и calendar math в ML,
Core explicit selector/frozen decoder/independent arithmetic, API exact context;
реальный CSV import/commit→forecast→payment→новый forecast в own marked PG/Python mTLS.
7. Negatives/backward/review — На ревью: lifecycle/replacement lifetime/overflow/
scope/CAS/replay/unknown context/unavailable ML, serial Go gates/APIcheck/MLRuff,
independent caveman+ponytail нового diff на SHA, fixes и push в draft48.

Бизнес-граница: поддерживается только CURRENT (cutoff сегодня/вчера без post-cutoff
movements). Historical/synthetic не расширяются. Additional только explicit;
replacement только exact IDs/versions/completeness, не категория. Cancel/fullmatch/
за горизонтом сохраняют mapping до revoke. Новое обучение/accuracy claim отсутствуют.


Итог продолжения: CSV+CURRENT path закрыт реальным CSV upload/commit без proof.
Implementation0826fa6; Core765tests/subtests+APIcheck88.70/89.04%+ML59+contracts
PASS локально; independent caveman/ponytail findings closed. Actual CI FAILURE
до запускаjobs из-за GitHub account billing/spending limit, не passing.
Прежний CSV/calendar blocker superseded новым явно разрешённым profile;
historical/synthetic/calibration не расширялись. Все стадии На ревью.
