# D2.4 — runtime financial analysis

Прямое поручение владельца: автономная реализация/публикация, промежуточный owner review отложен (ADR-020). Все законченные стадии — На ревью; merge/ready/deploy запрещены.

Parent: draft #48, SHA `7771af9e945de0e2b51d14af19fbb30de94ce4c2`; base branch `globalarray/feat/payments_frozen_calendar`. Main проверен `71f2a86`. Сначала принять #48, затем этот зависимый PR. Diff содержит только D2.4.

## Фактическая матрица

| Компонент | Состояние |
|---|---|
| categories.categorize / recurring.fit_monthly | pure helpers существуют; offline infer_patterns/daily_as_of не используем |
| ML AnalysisContext/hypotheses и Core ForecastAnalysis | canonical/generated contracts уже есть; runtime/read wiring отсутствует |
| CSV-only / CSV CURRENT | explicit v1 profiles работают; derived fields запрещены validators |
| Ledger manual category/clear | существующие commands/CAS; анализ не меняет факты |
| Snapshot/job/result | immutable schema1/2; используем additive optional saved analysis без новой SQL таблицы |

## Scope / compatibility

Ровно 90 дней исходного Predict, максимум20000 operations, до cutoff; manual metadata приоритетны. Rules `text_categories_monthly_v1`; monthly fit thresholds не меняем. Новые contexts: `mean_90_csv_analysis_v2` / `csv_observed_operations_analysis_v2` и `mean_90_csv_calendar_analysis_v2` / `csv_observed_operations_calendar_analysis_v2`; configuration_id равен model_version, model_name=mean_90, без seed/trajectories. Core выбирает их явно для новых eligible CSV jobs, frozen context остаётся в schema2. Старые jobs/bytes/v1 guards сохраняются. Covered/trained default не меняем, analysis там пока not-analyzed. Денежный расчёт переиспользует v1 без derived labels в arithmetic. Никакой новой калибровки/accuracy.

Wire diff до wiring: новых protobuf полей/номеров не планируется — используем ML response23/24 и Core ForecastResult.analysis8. REST additive nullable analysis повторяет существующий Core contract; точная DTO фиксируется в handoff перед consumer wiring. Отдельное сохранение derived result от Ledger/Payments; old JSON без analysis читается. Stale forecast остаётся доступным только как frozen view и не применяется к новым фактам.

## Этапы

1. План/compatibility/draft — На ревью. Прочитаны ADR/workflows/D2.4, #48–50 bodies/files/full discussions (comments/reviews/inline пусты), main и parent. Пересечений с B/C нет.
2. ML pure runtime analysis — Не начат. Single Predict, explicit new contexts, deterministic evidence, strict limits/manual precedence, regression/RPC/Ruff.
3. Core validation/persistence и API read — Не начат. Factual equality + derived allowlist, scoped evidence/version/rules, unchanged money validators; saved result projection, additive REST DTO/schema/tests. Go patterns: feature-owned DTO/manual app wiring/sqlx Tx из go-services; reference checkouts отсутствуют.
4. Реальный ordinary CSV + CURRENT lifecycle — Не начат. Own marked disposable PG18/Core/Python/mTLS, Auth double явно; manual override/clear, immutable results/restart/revisions. Backward и negative cases, serial race/vet/full lint/API check/Buf/schema.
5. Независимые caveman/ponytail на SHA, исправления и публикация — Не начат. Финальный diff/remote/main/actual CI, handoff и итоговое owner review.

GBrain MCP отсутствует в tool catalog; RTK отсутствует. Billing CI blocker перепроверяется, локальные gates обязательны. Training/frontend/Auth/deploy/чужие plans/handoffs вне scope. Риск: транспорт2MiB и время monthly fitting; превышение — явный отказ без усечения. Гипотеза не платёж/mapping/probability. Quality measurements отсутствуют.
