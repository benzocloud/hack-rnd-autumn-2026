# Поручение агенту A: платежи и frozen calendar — один draft PR

Передай агенту весь файл. Репозиторий:
https://github.com/benzocloud/hack-rnd-autumn-2026.
Контрольная точка владельца — 25.10.2026, 11:45 Europe/Moscow.

## Полномочия и сохранение результата

Разрешаю автономно пройти все этапы этого поручения: планирование, реализацию,
проверки, независимое ревью, исправления, commit/push и создание/обновление
одного draft PR. Промежуточный owner review отложен до общего результата:
это явное исключение ADR-020. Не спрашивай «продолжать?» после каждого этапа.
Этапы помечай «На ревью», а не «Принят». Merge, ready-for-review, deployment
и изменение работающего сервера не разрешены. Main может запускать автодеплой.

Первый результат в течение первых 10 минут — план и handoff в своей ветке,
небольшой коммит, push и открытый draft PR. После каждого законченного
содержательного блока сразу обновляй handoff, коммить и пушь. Если блок занимает
больше 20 минут, сохрани промежуточный собираемый срез с честным статусом.
Не жди финального ревью, исчерпания лимита или завершения всей задачи для push.
Если push/PR недоступны, сразу сообщи причину; не уходи в часы локальной работы.
Проверяй соответствие remote SHA локальному; локальный коммит не равен push.

Используй отдельный checkout/worktree от свежего origin/main. Прочитай
AGENTS.md, .agent/instructions.md, .agent/decisions.md, relevant workflows,
git-conventions и go-services. Проверь активные PR/Issue и обсуждения, чтобы
не дублировать чужую работу. GBrain recall — если доступен; иначе укажи причину
и продолжай по исходникам. Не трогай чужой checkout, не делай reset/stash/force.
Имя ветки — по проверенным правилам репозитория и настоящему nickname.
PR title: feat: connect payments and frozen calendar to forecasts.
План: .agent/plans/checkpoint-payments-backend.md.
Handoff: .agent/handoffs/checkpoint-payments-backend.md.
Коммиты/title по-английски, PR description и отчёты по-русски.

## Одна задача

Подключи существующий домен ожидаемых платежей к реальным Core RPC, Go API,
durable snapshot и единственному ML Predict. Новый прогноз должен учитывать
оставшуюся сумму известного платежа ровно один раз и объяснять изменение.
Это backend-часть D2.5; frontend ведёт агент B, E2E — агент C.
Онбординг завершает другой агент: его не реализовывай и Auth не переделывай.
Обучение модели уже поручено отдельно: не запускай обучение и не меняй default.

Начни с проверки фактического кода. На локально проверенной базе 3b3cebc
PaymentsService объявлен в proto/core/v1/payments.proto, но отдельного
handler/grpc/payments ещё нет. Это ориентир, не pin: свежий main может отличаться.
Прочитай .agent/plans/financial-demo-next-steps.md, особенно D2.0/D2.5,
docs/core/financial-demo-decisions.md, financial-ml.md, contract.md,
api-adapter.md, rest-rpc-map.json, docs/api/openapi.yaml и canonical proto.
Уже реализованные части переиспользуй; старый статус плана не доказательство.

Координатор отдельно заканчивает D2.3a: обычный CSV без manifest, schema 2,
mean_90_csv_v1, ручные дополнения после импорта и POST transactions.
Эта работа может ещё отсутствовать в main. Не переписывай imports/CSV/manual
ради своего PR и не копируй грязное дерево координатора. При появлении
опубликованной зависимости зафиксируй её URL/SHA; интегрируй обычным merge
проверенной ветки при необходимости, явно сохраняя зависимость в описании PR.
Если базовый профиль не принимает календарь, не игнорируй платежи молча:
покажи явный отказ/ограничение и точный blocker совместимости. Совместимую
активацию делай только в рамках принятых profile/version правил; не маскируй
новый расчёт старым model context. Рабочий covered-сценарий не доказывает CSV.

## Владение файлами

Твои: необходимые services/core/** и services/api/** для payments/calendar,
соответствующие тесты, payment/forecast-разделы canonical contracts и docs;
миграции только если действительно нужны. Из services/ml — минимальная
интеграция существующего financial calendar и относящиеся regression tests.
Не переписывай ML model/training modules. При необходимости additive contract
change сначала зафиксируй точный wire diff в своём PR и handoff для агента B;
не меняй смысл/номера существующих полей и не добавляй второй ML RPC.
Не меняй frontend/**, deploy/**, Auth/onboarding, общий план и чужие handoff.
Общие app/router/client файлы редактируй минимально, без форматирования всего файла.

## Этапы, выполнять непрерывно

1. Зафиксируй матрицу existing/not wired/not implemented по REST/RPC и первый
   P0 slice. Опубликуй draft. В handoff сразу выпиши endpoints/DTO для frontend.
2. Подключи list/get/create/update/cancel/match/unmatch и чтение match history
   к существующим services/repositories. Сохрани Auth scope/CSRF, resource CAS,
   replay до stale guards, атомарность и точные RUB-копейки. Match не исполняет
   платёж, не создаёт новую транзакцию и не позволяет overmatch/чужой scope.
3. Заморозь в snapshot календарь, remaining/status/version, reconciliation
   groups, availability/knowledge mode и provenance в одной согласованной Tx.
   Не перечитывай live calendar при worker/read старого прогноза. Сохрани старые
   codecs и результаты. Перенос proof допустим только при неизменных фактах.
4. Подключи календарь к одному Predict, включи в сохранённый результат
   components/evidence/effective occurrences. Available empty и unavailable
   различаются. Не выдавай synthetic historical scenario за CURRENT или
   историческое знание; используй принятый typed mode и обязательные limitations.
5. Проверь арифметику independently в Go: residual после replacement плюс known
   remaining по датам, суммы components, source IDs, непересечение групп,
   overflow. Additional obligation требует явного основания; совпадение категории
   не доказывает complete replacement. Complete group живёт отдельно от active
   occurrences: cancel/full match/перенос за горизонт не возвращают residual
   без явного revoke mapping. Не сдвигай overdue молча на первый день.
6. Закончи регрессионные проверки, независимое caveman-review и ponytail-review
   на конкретном SHA, исправь подтверждённые находки и запушь итог в тот же PR.
   Если skill недоступен, назови ограничение; не утверждай, что ревью выполнено.

## Проверки и приёмка

Не запускай весь suite и ревью после мелких правок. Собери законченный блок,
потом один подходящий набор gates; узкий rerun — по новому изменению/падению.
RTK используй для поддерживаемых команд, raw output — для точного разбора.
Для Go: tests/race, vet и full module lint из каждого изменённого Go module;
для Python: existing RPC/mTLS/financial tests и pinned Ruff; для contracts:
Buf lint/build/breaking/generation drift и API schema tests.
PG integration запускай в собственном marked disposable project/DB/port.
При общей тестовой DB используй go test -p 1: cross-package workers могут
забирать чужие задания. Не подключайся к публичной/чужой БД, не ослабляй guards.

Докажи реальными API→Core→PostgreSQL и Go→Python mTLS вызовами:
create → forecast → amount edit → date edit → partial match → unmatch →
partial match → cancel → новый forecast → reload/restart. Проверяй фактические
30 точек/remaining и staleness старого результата. Старый snapshot/result
неизменен. Partial cancel не удаляет факт; match history читается после cancel;
unmatch после cancel либо законный переход, либо конкретный договорённый отказ.
Негативные случаи: duplicate key/body conflict, stale version, foreign scope,
overmatch, ML unavailable, unavailable calendar, недоказанный replacement,
cancel/full match/за горизонтом, unknown model/mode и no silent fallback.

P0 — работающий платежный lifecycle и реальное изменение нового прогноза.
Не трать время на redesign, массовый рефакторинг и новую модель. К 11:15 МСК
зафиксируй максимально цельный проверенный срез; дальше исправляй блокеры и
готовь интеграцию к 11:45. Если сроки уже прошли — сразу публикуй текущий
checkpoint и точные оставшиеся шаги, не объявляй незавершённое готовым.

В каждом handoff: branch/base/current/remote SHA, URL PR, что работает,
последний законченный этап, команды/результаты/skips, known failures,
контрактные зависимости, схема/rollback, точная следующая команда.
Перед финалом проверь новый main без force, собери итоговый diff и покажи
actual CI status. Pending CI не называй passing. PR оставь draft; выдай URL,
готовность по сценариям, ограничения и список того, что подхватить владельцу.
