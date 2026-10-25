# Payments и frozen calendar — D2.5

Цель: реальный lifecycle платежей API → Core → PostgreSQL и ровно однократный учёт remaining в единственном Predict. Owner явно разрешил автономные этапы, commit/push и один draft PR; промежуточное review отложено. Merge/ready/deploy запрещены.

База: origin/main 0abae4f. Домен payments, CreatePayment/MatchPayment, SQL и canonical PaymentsService существуют. gRPC payments и REST payments не подключены; read/update/cancel/unmatch service ещё отсутствуют. Snapshot codec и чтение календаря существуют, но runtime совместимость требуют проверки. #47 head 7e5c0d4aff78286c13d9a8245f801e8a5b82e16c — опубликованная CSV/manual зависимость; пока не интегрирована.

Решения: ADR-029/030, точные RUB-копейки, account lock/CAS/replay, отдельные reconciliation groups, immutable snapshots, explicit unsupported profile. Ориентир Go: consumer interfaces/manual wiring cosmohack из .agent/references/go-services.md; локальные originals пока не проверены.

## Этапы
1. Матрица и контрактный handoff — На ревью. Проверены source tree, PR #47 body/discussion/head. Критерий: опубликованный draft и remote SHA.
2. Lifecycle — Не начат. services/core и services/api payments adapters/services/repositories; reuse domain/create/match, добавить reads/CAS update/cancel/unmatch/history. Проверки: unit/race, real marked PG, Auth/scope/CSRF/replay/overmatch.
3. Frozen calendar — Не начат. snapshot capture/codec/worker, independent groups/provenance/availability/mode одной Tx. Старые codecs/results сохраняются. Проверки: restart/immutability, proof carry только без изменения фактов.
4. Единственный Predict — Не начат. Минимальный ML adapter/calendar regression; совместимый version/profile без default/training изменений. Независимая Go arithmetic validation, source IDs/groups/overflow/overdue. Проверки: реальные mTLS 30 точек и negative scenarios; CSV проверяется отдельно.
5. Итоговая интеграция и review — Не начат. Full module tests/race/vet/lint, pinned Python/Ruff, Buf/drift/schema, independent caveman/ponytail на конкретном SHA, fixes, main compatibility/actual CI. Все стадии остаются На ревью до owner approval.

Границы: frontend/deploy/Auth/onboarding/training/чужие handoffs не меняются. Миграции только при необходимости. Риск: несовместимость CSV profile/calendar должна давать явный отказ; не маскировать profile. Изолированный PG18 внутри контейнера, marker обязателен; serial packages при общей test DB.
