# Runtime financial analysis — handoff A

Статус На ревью. Branch `globalarray/feat/runtime_financial_analysis`; base `globalarray/feat/payments_frozen_calendar`; parent #48=`7771af9e945de0e2b51d14af19fbb30de94ce4c2`, unchanged. Main71f2a86. Новый draft URL и local/remote SHA фиксируются следующим checkpoint после publication. Последний завершённый этап1: plan/матрица/compatibility; runtime ещё не реализован.

Profiles: mean_90_csv_analysis_v2 / csv_observed_operations_analysis_v2 и mean_90_csv_calendar_analysis_v2 / csv_observed_operations_calendar_analysis_v2, configuration=model_version; analysis=text_categories_monthly_v1. Legacy CSV/covered immutable results читаются; новые analysis contexts не fallback. CSV eligibility и CURRENT arithmetic из #48 сохраняются. Snapshot/job schema1/2, optional analysis в saved result; SQL migration не планируется. Rollback producer требует сохранения поддержки новых jobs в worker/reader.

Планируемый UI read: существующий GET /api/v1/companies/{company_id}/forecasts/{id}, result.analysis nullable; analysis_version/window_from/window_through, classified_operations[{transaction_id,version,category_code,category_source,category_rule_id,category_id}], recurring_hypotheses с exact IDs/versions/monthly_anchor/weekend_shift/amount/confirmations/status. Endpoint/request прежние. Manual override/clear — существующий PATCH transactions category command. category_source user/automatic/unknown; отсутствие analysis=not-analyzed. Это frozen scoped view, stale не применяем к новым facts. Точный wire diff:0 новых proto полей, используем уже объявленные contracts; REST дополнение будет уточнено перед wiring.

Checks: git status parent clean, fetch/main/active PR48/49/50 descriptions/files/full discussions прочитаны; обсуждения пусты. Runtime gates пока не запускались. GBrain MCP и RTK отсутствуют. Настоящие services ещё не запускались; Auth double будет явно отмечен. Known blockers: прежний GitHub billing issue, перепроверить. Никаких credentials/logs в Git.

Следующая команда: `sed -n '110,190p' services/core/internal/service/forecasting/job_worker.go`.
