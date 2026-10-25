# Финансовый frontend — checkpoint

Цель: импорт → операции/остаток → сохранённый прогноз 30 дней → календарь → изменение → новый прогноз → reload через Go API.
Авторизация: автономная реализация/проверки/commit/push/один draft PR; owner review отложен до общего результата. Merge/ready/deploy запрещены.
База: origin/main 6c9edd9. #47 merged f686ca86438948ca4333f0125513a58c5045ce59; #48 open f4627f1cfcca5fa2ad01ec7e52a618f8e608d4ea.
Границы: финансовые frontend/src и unit tests; без Auth/membership/backend/contracts/deploy/financial E2E агента C. Существующие компоненты и DTO.

| Этап | Результат/пути | Зависимости/проверки | Статус |
| --- | --- | --- | --- |
| 1 | Сверка API, план/handoff, draft | main/OpenAPI/PR discussions; remote SHA | На ревью |
| 2 | Imports/Transactions: preview, ошибки, точные деньги, retry | существующий API #47; unit tests | На ревью |
| 3 | ForecastDetail/Dashboard: 30 дней, polling, immutable/stale, null | forecast DTO; unit/typecheck | На ревью |
| 4 | Calendar: lifecycle, remaining/history, явная reconciliation семантика | #48 handlers; contract tests | На ревью |
| 5 | Проверки, независимые caveman/ponytail, исправления | frontend test/typecheck/lint/build; real backend browser при доступности | В работе |

Критерий: доступные реальные действия без финансового mock; неподдержанные режимы явно ограничены. Unknown/probability не превращаются в ноль. После каждого блока handoff и push; завершённые этапы только «На ревью».
Риски: #48 ещё не merged, runtime анализа не подтверждён, backend окружение пока не проверено. GBrain MCP не предоставлен; RTK отсутствует. Нужные ограничения фиксируются без имитации успеха.
