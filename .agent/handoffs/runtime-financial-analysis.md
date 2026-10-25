# Runtime financial analysis — handoff A

Статус На ревью. Branch `globalarray/feat/runtime_financial_analysis`; base `globalarray/feat/payments_frozen_calendar`; parent #48=`7771af9e945de0e2b51d14af19fbb30de94ce4c2`, unchanged. Main71f2a86. Новый draft URL и local/remote SHA фиксируются следующим checkpoint после publication. Последний завершённый этап1: plan/матрица/compatibility; runtime ещё не реализован.

Profiles: mean_90_csv_analysis_v2 / csv_observed_operations_analysis_v2 и mean_90_csv_calendar_analysis_v2 / csv_observed_operations_calendar_analysis_v2, configuration=model_version; analysis=text_categories_monthly_v1. Legacy CSV/covered immutable results читаются; новые analysis contexts не fallback. CSV eligibility и CURRENT arithmetic из #48 сохраняются. Snapshot/job schema1/2, optional analysis в saved result; SQL migration не планируется. Rollback producer требует сохранения поддержки новых jobs в worker/reader.

Планируемый UI read: существующий GET /api/v1/companies/{company_id}/forecasts/{id}, result.analysis nullable; analysis_version/window_from/window_through, classified_operations[{transaction_id,version,category_code,category_source,category_rule_id,category_id}], recurring_hypotheses с exact IDs/versions/monthly_anchor/weekend_shift/amount/confirmations/status. Endpoint/request прежние. Manual override/clear — существующий PATCH transactions category command. category_source user/automatic/unknown; отсутствие analysis=not-analyzed. Это frozen scoped view, stale не применяем к новым facts. Точный wire diff:0 новых proto полей, используем уже объявленные contracts; REST дополнение будет уточнено перед wiring.

Checks: git status parent clean, fetch/main/active PR48/49/50 descriptions/files/full discussions прочитаны; обсуждения пусты. Runtime gates пока не запускались. GBrain MCP и RTK отсутствуют. Настоящие services ещё не запускались; Auth double будет явно отмечен. Known blockers: прежний GitHub billing issue, перепроверить. Никаких credentials/logs в Git.

Следующая команда: `sed -n '110,190p' services/core/internal/service/forecasting/job_worker.go`.

## Первый runnable slice, 25.10 08:48 UTC — На ревью

Новый draft: https://github.com/benzocloud/hack-rnd-autumn-2026/pull/51.
ML single Predict заполняет существующие typed fields; Core явно выбирает v2 CSV contexts, замораживает tenant catalogue в job input, независимо проверяет facts/evidence/monthly schedule/amounts и прежнюю финансовую арифметику. Derived result сохраняется отдельно в core_forecast_jobs.result, schema1 optional Analysis; schema2 input optional Categories. SQL migrations нет. Core→API использует существующий ForecastResult.analysis8; wire diff0, REST mapping существующего optional analysis активирован.

DTO точный: category_source=`manual|rules|unknown` (rules — automatic); legacy analysis null=not-analyzed. JSON keys analysis_version/window_from/window_through/classified_operations/recurring_hypotheses; refs transaction_id/version, money и version decimal strings; anchor ровно day_of_month либо month_end=true; weekend_shift none/previous/next; status=hypothesis. GET /api/v1/companies/{company_id}/forecasts/{id}; PATCH /transactions/{id}/category {version,category_id:UUID|null}. Новые request fields/endpoints отсутствуют. Stale analysis остаётся только частью frozen forecast (is_stale/current_data_version/input.data_version), не накладывается на live transactions.

Проверено: ML3 новых tests с subcases PASS (context/manual clear/threshold3/monthly/unknown/90-day guards/денежное равенство v1); Core affected unit suites PASS, API handler existing tests PASS. Настоящий TestOrdinaryCSVCalendarHTTPPostgresPythonLifecycle -race PASS4.861s: обычный CSV upload/commit, automatic tax→manual override→clear, новые forecasts CSV/CURRENT, все30точек, additional/replacement/fullmatch/cancel/revoke и неизменность старых input/result/snapshot/restart. Auth — явно double; API/Core/PG/Python/mTLS настоящие. Analysis сам revision не повышает; manual commands повышают ровно по1. PG18 собственный .agent/local/pg18,127.0.0.1:55489, marked saldo-core-integration, штатные migrations1–15. Logs/tooling ignored.

Full gates/negative expansion/review ещё не завершены. Срок11:45МСК наступил во время реализации; работа по прямому разрешению продолжается, ready не заявляется. Внешний coordinator добавил CI merge в remote #48(fd1cb15) и #51(daf423e); сохраняем обычным merge remote без force, parent checkout не меняем. Published parent functional SHA7771af9 по-прежнему dependency; CI-only update отдельно от D2.4.

Следующая команда: `. .agent/local/test-env.sh; go -C services/core test -json -p 1 -tags=integration,mlintegration -race -count=1 ./...` после завершения negatives.
