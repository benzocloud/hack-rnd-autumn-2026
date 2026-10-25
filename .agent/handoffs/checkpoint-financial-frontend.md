# Handoff финансового frontend

Дата: 25.10.2026, старт 10:36 МСК; checkpoint 11:15/11:45 МСК.
Ветка: globalarray/feat/financial_user_journey. Base: 6c9edd9 (origin/main). Local/remote SHA: первый plan commit, см. git log/remote; будет уточнён следующим push. PR: создаётся первым push.
Этап 1: В работе. Прочитаны инструкции/ADR/workflows, README/OpenAPI, исходники Imports/Calendar/Dashboard; обсуждения #47/#48 пусты, #47 merged.

| API | Готовность/следующий шаг |
| --- | --- |
| imports upload/get/issues/commit | клиент существует; проверить stale UX и ack |
| transactions POST/list, account balance | #47 merged; не дублировать coordinator changes |
| forecasts POST/get/result | клиент существует; проверить polling/reload/30 days |
| payments list/detail/create/update/cancel/matches | клиент существует, main backend ещё не подключён; dependency #48 |
| reconciliation controls | сверить опубликованный DTO #48; не выводить source IDs из текста |

Dependencies: https://github.com/benzocloud/hack-rnd-autumn-2026/pull/47 merged f686ca86438948ca4333f0125513a58c5045ce59; https://github.com/benzocloud/hack-rnd-autumn-2026/pull/48 head f4627f1cfcca5fa2ad01ec7e52a618f8e608d4ea.
Проверки: git fetch origin выполнен; новый isolated worktree /tmp/saldo-financial от свежего main, чужие dirty files не копировались. Финансовые браузерные сценарии ещё не проверены; screenshots отсутствуют. CI не проверен.
Инструменты: GBrain MCP не предоставлен; RTK отсутствует. Sandbox bwrap namespace недоступен, команды разрешены через escalation.
Следующий шаг: закончить матрицу source/API, исправить независимые состояния/формы и опубликовать блок.
