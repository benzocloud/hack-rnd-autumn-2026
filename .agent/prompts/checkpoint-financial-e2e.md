# Поручение агенту C: реальная финансовая приёмка — один draft PR

Передай агенту весь файл. Репозиторий:
https://github.com/benzocloud/hack-rnd-autumn-2026.
Контрольная точка — 25.10.2026, 11:45 Europe/Moscow.

## Авторизация и сохранение работы

Разрешаю автономно выполнить план, тестовый harness, проверки, независимое ревью,
исправления своей части, commit/push и создать/обновлять один draft PR.
Промежуточное ревью владельца отложено до общего результата — явное исключение
ADR-020. Не останавливайся с вопросом «продолжать?» между этапами. Этапы остаются
«На ревью». Merge, ready-for-review, public deployment и изменение работающего
сервера не разрешены. Разрешён только собственный изолированный локальный test stack.

Первые 10 минут: план/handoff, небольшой commit/push, draft PR. После каждого
завершённого блока сразу обновляй handoff, commit/push; при долгой работе публикуй
собираемый checkpoint не реже чем после ~20 минут содержательной реализации.
Не надейся написать финал перед лимитом. При недоступном push/PR сразу сообщи
причину и сохрани точные шаги подхвата; проверь remote SHA.
PR title: test: verify the real financial user journey.
План: .agent/plans/checkpoint-financial-e2e.md.
Handoff: .agent/handoffs/checkpoint-financial-e2e.md.
Коммиты/title английские, human-facing отчёты и описание PR русские.

Прочитай AGENTS.md, .agent/instructions.md, .agent/decisions.md, workflows,
git-conventions, .agent/plans/financial-demo-next-steps.md (D3.2/D4), текущие
Auth/API/Core/ML контракты и существующие интеграционные тесты. GBrain recall
если доступен; при отсутствии назови причину и продолжай. Проверь PR/Issue и
полные обсуждения. Fetch origin, отдельный checkout от свежего main; не трогай
чужие dirty trees, не reset/stash/force. Ветка по правилам с настоящим nickname.

## Одна задача и владение

Сделай воспроизводимую финансовую приёмку с настоящими Frontend → Go API → Auth /
Core → PostgreSQL → Python ML, с отчётом на конкретных SHA. Результат — runnable
сценарий, доказанные проходы и точные blockers для владельцев компонентов.
Это тестовый PR, не ещё одна реализация продукта и не переделка инфраструктуры.

Твои пути: scripts/financial-e2e/**, frontend/tests/e2e/financial/**,
docs/verification/financial-checkpoint-2026-10-25.md, собственные plan/handoff.
Применяй существующий runner/browser stack; при необходимом изменении общих
frontend manifests/config сначала опиши точный diff в handoff: ими владеет агент B.
Не меняй services/**, canonical proto/OpenAPI, frontend/src/**, Auth/onboarding,
deploy/**, общие CI workflows и чужие планы. Минимальный fixture/runner config
внутри своих путей разрешён. Не дублируй deploy/stack/smoke.py: он уже занят.
Найденный product bug передай воспроизведением в handoff/PR, не исправляй его
параллельно с владельцем. Авторизация публикации относится к твоему PR и его
комментариям; не рассылай сообщения в другие чаты/сервисы без поручения.

Онбординг заканчивает отдельный агент: не реализуй его. Агент A — backend
платежей/frozen calendar, агент B — финансовый UI, координатор — CSV без manifest
и ручные дополнения после импорта; обучение модели уже у другого агента.
Зависимые PR могут быть ещё не опубликованы/не слиты. Не выдавай stale main за
общий итог и не копируй грязные файлы. Читай опубликованные SHA и отмечай,
какой состав проверялся. Подготовь harness независимо, затем отдельный integration
checkout со штатными merge опубликованных зависимостей; эти чужие коммиты не
включай в свой тестовый PR. Общий merge/deploy оставь владельцу.

## Этапы без промежуточной остановки

1. Опубликуй draft с матрицей сценариев, точным источником тестовых файлов и
   окружением. Найди существующий способ изолированного запуска реальных сервисов.
   Если локальный stack недоступен, запиши blocker и сделай runnable harness;
   не объявляй непроведённые проверки успешными и не замени сервисы doubles.
2. Создай запуск с отдельным project name, свободными ports и disposable DB/
   volumes. Проверяй marker/identity перед teardown. Не используй production DB,
   чужие containers/networks/PKI, Fadwix/GBrain/Grafana и привилегированные sockets.
   Используй реальные сервисы и их штатные migrations/config, не ослабляй TLS/Auth.
   Генерируй test credentials локально; не коммить secrets/cookies/private keys.
3. Через публичный UI/API: register → verify → login → create company → upload
   CSV → preview/issues/ack → commit → ledger → forecast → polling → 30 точек →
   saved result reload. Уникальная новая company/account, без SQL-вставок financial
   facts, membership, coverage/proof и без operator fixture вместо пользовательского
   импорта. До готовности onboarding можно проверить остальные сценарии с штатно
   созданной тестовой компанией, но полный first-user flow остаётся незакрытым.
4. CSV/manual regression: импорт той же выписки/повтор ключа не удваивает факты;
   ручная операция после импорта сохраняется один раз при replay и имеет manual
   provenance; namespace счёта остаётся внешним; точный imported duplicate отвергнут;
   совпадение следующего CSV с manual требует явного ack. Проверяй реальные
   responses/counts через публичный API, сохранность после reload/restart и stale
   старого forecast. Не выводи точный баланс простым суммированием неполного CSV.
5. Платежи: create → forecast → amount edit → date edit → partial match → unmatch
   → partial match → cancel → новый forecast → reload/restart. Проверяй remaining,
   наличие факта исполнения ровно один раз, сохранность старого forecast и новую
   revision. Match history после cancel доступна; запрещённый переход даёт явный
   отказ. Изменение будущего обязательства должно менять новые прогнозные точки
   по договорённому режиму, не только надпись stale. Covered fixture и обычный CSV
   проверяются отдельно: успех одного не доказывает поддержку другого.
6. Негативные сценарии после работающего P0: invalid/partial upload, duplicate
   key с другим body, stale preview/CAS, короткая история, EOD conflict, missing
   cutoff anchor, session expiry/revocation, CSRF, foreign company/resource/cursor,
   ML unavailable. Ошибка не выглядит «нулевой риск». Unknown days не zero-fill;
   probability unavailable не рисуется как 0%; отсутствующий interval не выдуман.
7. Перезапусти собственные сервисы, проверь persistence и восстановление jobs
   в пределах существующего контракта. Не тестируй crash на общем/public stack.
   Сделай короткий browser walkthrough с timestamp и конкретными source SHA,
   screenshots без секретов, затем независимые caveman-review/ponytail-review
   своего harness/report, исправления и финальный push.

## Что считать доказательством

В отчёте отдельные колонки: implemented / automated contract test / real HTTP /
real browser / restart / blocked. Double Auth/predictor и финансовый mock не
подтверждают настоящий путь. Не вызывай private RPC напрямую из браузера.
Server demo email profile допустим только явно включённый/allowlisted, с пометкой;
strict должен отвергать неверный код. VerifyEmail не даёт скрытый автологин.
Не тестируй безопасность выключением CSRF или ручной выдачей роли в БД.

Harness: bounded timeouts/polling, Abort/cleanup, уникальные аккаунты/имена,
детерминированные даты и проверяемые fixture hashes; полезные ошибки и ненулевой
exit status при провале обязательного шага. Не делай skipped mandatory scenario
зелёным результатом. Точные expected outcomes привязывай к canonical контракту,
а не к тому, что случайно вернул сервис. Не тащи большой framework ради пары шагов.

Тесты и ревью после содержательного блока, не после каждой мелкой правки.
RTK по поддерживаемым командам, raw output для точного разбора. Не гоняй бесконечно
full suite каждого сервиса: его владелец проверяет модуль, ты проверяешь сквозной
результат. Известный общий PG integration suite требует serial packages (-p 1)
или отдельных DB: runtime workers могут выбирать задания соседних пакетов.

## Срок, публикация и подхват

Сначала основной успешный сценарий, затем payment/manual regression, потом
негативные проверки. К 11:15 МСК опубликуй исполнимый срез и главные blockers;
до 11:45 проверяй доступный интегрированный кандидат. Если времени мало/срок
прошёл — сразу checkpoint, не hours-long setup и не выдуманная готовность.
Для демонстрации выпиши последовательность действий, файл, expected outcomes,
честные ограничения и резервный записанный сценарий; запись не заменяет тест.

Handoff после каждого push: PR URL, base/local/remote SHA, SHA всех проверенных
сервисов/зависимостей, exact commands, env variable names без значений secrets,
fixture/artifact hashes, результаты/skips, blockers с минимальным воспроизведением,
что уже сохранено на GitHub и следующий шаг подхвата. Отчёт не утверждает
качество модели только по успешному RPC; not_calibrated остаётся открытым пунктом.
Перед финалом проверь diff и actual CI status, оставь PR draft. Не сливай,
не развёртывай и не объявляй исправленным публичный сайт по локальному прогону.
