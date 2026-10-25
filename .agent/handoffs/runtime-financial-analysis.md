# D2.4 runtime analysis — handoff A

Статус всех стадий: **На ревью**, не Принят. Прямое поручение владельца разрешило автономную реализацию/commit/push и отложило промежуточный owner review (ADR-020). Merge/ready/deploy не выполнялись.

PR: https://github.com/benzocloud/hack-rnd-autumn-2026/pull/51 (draft).
Branch: `globalarray/feat/runtime_financial_analysis`, checkout `/workspace/repo-analysis`.
Base: `globalarray/feat/payments_frozen_calendar`, parent draft #48.
Parent functional SHA: `7771af9e945de0e2b51d14af19fbb30de94ce4c2`;
актуальный parent с внешними CI-only обновлениями: `795963731bfe837462682d51acebc02d5c5d0972`.
Проверенный main: `71f2a86ffbcf218d7fff517830b7aa41f0a97c95`, новых main commits нет.
Reviewed implementation SHA: `f56b9b29defce44949514aba34f9e6a5b353c7e7`.
Current source после обычного parent merge: `a690983b0c60fc22cca00283cf789df3273e8c11`.
Перед этим docs checkpoint опубликованный local/remote f56b9b2 совпадали; следующий push публикует a690983 + только этот docs checkpoint, итоговый SHA проверяется git ls-remote и указан в PR.
Порядок принятия: сначала #48, затем #51; base чужих PR не менялся.

## Что действительно работает

Обычный CSV без manifest/proof → настоящий upload/preview/commit → forecast с явным analysis v2 → сохранённые automatic category/source/rule и typed monthly hypotheses через публичный API. Manual override приоритетен; clear возвращает rules на следующем forecast. Analysis не обновляет Ledger/Payments или account/resource revisions. Старые результаты и snapshot/input/result bytes неизменны, stale отмечен явно. Reload/Core reader/API restart не вызывает ML заново.

Два реальных serial race сценария:
- Sparse CSV2факта/88unknown days → automatic tax → manual override/clear → CSV+CURRENT → полный payment lifecycle, additional/replacement, partial match/unmatch/rematch/cancel/fullmatch/outside horizon/revoke, все30точек и remaining, restart.
- CSV5фактов/85unknown days с честными EOD →3rent confirmations → monthly hypothesis с exact refs → manual одной rent: no pattern → clear: pattern → available-empty CURRENT: тот же residual. Все30точек100in/80out, ни новых payments, ни facts/revision от анализа; immutable bytes/staleness/restart.

Реальные API/Core/PostgreSQL/Python и mTLS. **Auth — явный test double**, настоящий Auth/browser остаётся B/C. SQL inserts facts/proof/fixture allowlist для этих сценариев не использовались. Early invalid CSV fixture с blank balance_eod был отклонён parser; исправлены фактические EOD, guards не менялись. Попытка reimport изменённого EOD не обходилась: monthly scenario использует отдельный scope/первый импорт.

## Версии и финансовая совместимость

| Режим новых CSV jobs | model_name | model_version / configuration_id | preprocessing_version |
|---|---|---|---|
| CSV без календаря | mean_90 | mean_90_csv_analysis_v2 | csv_observed_operations_analysis_v2 |
| Frozen CURRENT | mean_90 | mean_90_csv_calendar_analysis_v2 | csv_observed_operations_calendar_analysis_v2 |

Analysis version: `text_categories_monthly_v1`; random_seed/trajectory_count отсутствуют. Core выбирает полный context явно и замораживает его в schema2 job. Один Predict, без fallback/второго RPC. Денежная часть переиспользует v1 на исходных metadata, derived labels не участвуют в residual/calendars. CSV90-day span, cutoff EOD, exact reconciliation, unknown days/manual provenance/limits сохранены. CURRENT/replacement/additional остаются из #48; overdue не сдвигается, гипотеза не создаёт Payment/group/known occurrence.

Старые CSV-only/CSV+CURRENT v1 jobs, schema1/2, covered/trained contexts сохраняют decoder/bytes. **Covered/trained runtime analysis пока not-analyzed**; default/training/evaluation/artifacts не менялись. Historical/synthetic не активировались. Вероятность null/not_calibrated, bands не придумываются; measured category/recurrence accuracy отсутствует.

Ровно cutoff-89..cutoff, <=20000operations; <=1000hypotheses и <=20000refs, message/result2MiB, превышение — отказ без усечения. Pure categorize/fit_monthly без offline loader/daily_as_of/dataset assumptions. Пороги прежние:3regular-outflow confirmations/4otherwise,75%periods,4-day tolerance,25%median monthly-max floor, non-regular MAD/median<=0.3, exclusions. Go независимо проверяет пороги точными int64/big.Rat расчётами и сохраняет финансовые overflow/components guards.

## UI / C: точные endpoints и DTO

Новых endpoints/request fields нет. Company scope, CSRF и resource CAS прежние.

- GET `/api/v1/companies/{company_id}/forecasts/{id}`: existing optional `result.analysis`, null для legacy/not-analyzed. `analysis_version`, `window_from`, `window_through`, `classified_operations`, `recurring_hypotheses`.
- Classified row: `{transaction_id, version, category_code, category_source, category_rule_id, category_id}`. `version` decimal string; category_id UUID|null. source=`manual|rules|unknown`; rules означает automatic, неизвестная категория явная. Ручная категория сохраняет UUID/source и пустой rule. Core mapping использует только явные code→frozen tenant catalogue codes + direction, без угадывания display name; unmapped semantic labels не получают UUID.
- Hypothesis: `{pattern_id,rule_id:"monthly_fit",rule_version:"text_categories_monthly_v1",direction,historical_operations:[{transaction_id,version}],day_of_month OR month_end:true,weekend_shift,expected_amount_minor,confirmations,expected_periods,first_evidence_date,last_evidence_date,category_code,status:"hypothesis"}`. shift=none/previous/next, amount decimal string. IDs — opaque hash snapshot/rule/version/sorted UUIDs, PII/amount не в ID. Party display разрешается из авторизованных factual refs.
- PATCH `/api/v1/companies/{company_id}/transactions/{id}/category`: `{"version":"N","category_id":"UUID"}`; clear: `{"version":"N","category_id":null}`. На следующем forecast новая operation.version, account revision и analysis. Same stale CAS — прежний conflict.

Consumer не должен накладывать frozen labels на более новую operation.version/revision. `forecast.is_stale`, `current_data_version`, `input.data_version` применяются ко всему analysis; строки вне window или legacy absence означают not-analyzed, не unknown/successfully-empty. Hypothesis — объяснимая эвристика, не платёж/complete replacement/вероятность.

Wire diff: **0 новых protobuf полей/номеров**, активированы существующие ML response23/24, Core ForecastResult.analysis8 и OpenAPI schemas. Номера/смысл существующих fields не менялись. Existing REST requests и ответы B сохраняются; optional analysis теперь заполняется только explicit v2.

## Persistence / rollback

Derived data отдельно от Ledger/Payments: immutable `core_forecast_jobs.result.Analysis`; optional `Categories` в schema2 input заморожен под account Tx вместе с assessment/model/snapshot/calendar/groups. Старые codecs schema1/2 backward readable, optional JSON omitted в legacy, старые bytes не переписываются. SQL migrations **нет**.

Один общий ValidateAnalysis используется на ML boundary и durable read: factual equality, manual precedence, exact scoped IDs/versions, allowlisted category/source/rule/direction, tenant mapping, unique/nonoverlap evidence, party/hash/monthly pattern/confirmations/amount-floor/MAD/overflow. Worker/read не перечитывают live calendar/catalogue/ML. Rollback: прекратить новые v2 producers, сохранить v2 reader/worker support для уже записанных jobs; старый binary нельзя смешивать с новыми context без поддержки.

## Gates — фактические результаты

- Core full serial `go test -json -p 1 -tags=integration,mlintegration -race -count=1 ./...` **PASS813tests/subtests**:812pass,1opt-in large-import measurement skip;30test packages,9без tests. После него добавленный точный forged-threshold fixture отдельно `-race` PASS. Tagged `go vet` PASS; full tagged module lint0 и fmt clean. Первый full run выявил один predictor double без analysis в manual-import test; double исправлен, финальный полный rerun зелёный. Guards не ослаблялись.
- API штатный `scripts/check.sh` **PASS**: build/generation/unit/race/coverage/vet/fmt/full lint0, Ruff/data-contracts, separate HTTPS/mTLS smoke10endpoints. Coverage **88.94%statements /89.27%lines**, gate85 сохранён. После добавления analysis sample узкий real-HTTP conformance **42responses PASS**.
- ML `PYTHONPATH=services/ml python -B services/ml/check_rpc_tests.py`: **63tests,0skips PASS** (RPC/mTLS/security/financial/CSV/covered/trained). Ruff0.17.1 check/format **20files clean**.
- Buf1.75 lint/format/build/breaking против parent/generated drift/consumer compilation **PASS**,58REST mappings. API schema **13tests PASS**; actual42HTTPresponses conformance PASS с pinned jsonschema4.26.0. Initial schema gate использовал4.25.1, повторён с точным API pin в собственном ignored target. PyYAML6.0.3/grpcio-tools1.78.0/Ruff0.17.1. После внешнего parent protobuf-tooling update `generate.py --check` PASS; D2.4 wire diff0.
- Negative/backward: determinism, manual override/clear, stale fact/scope/read, unknown/post-cutoff/duplicate IDs/versions, malformed derived labels, foreign evidence/party, insufficient/no pattern, excluded category, anchor/shift/confirmations/periods, amount floor/MAD/large exact money, unknown versions/modes, ML failures/no fallback и oldCSV/covered/current financial guards. No training/quality promotion.

Logs/tools/state только ignored `.agent/local`: core-analysis-final.json/.err, core-analysis-final-vet.log, core-analysis-final-lint.log, api-analysis-check.log, analysis-http-contract*.log, ml-analysis-full.log, analysis-contracts.log, analysis-schema-pinned.log, analysis-ruff-final.log. Repro env `.agent/local/test-env.sh`; own PG18 process data `.agent/local/pg18`, port55489,DB saldo_core_test, marker saldo-core-integration, штатные migrations1–15. **PG остановлен успешно25.10 09:09UTC**; общий стек/Docker/server не менялись.

## Независимое review и actual CI

Caveman/ponytail e57eabe:2findings; followup2eff14c: закрыты + найден amount-threshold gap. Все3 исправлены и regression tested. Final review **f56b9b29defce44949514aba34f9e6a5b353c7e7**: новых correctness findings нет, ponytail net0. Независимый reviewer подтвердил binding к **a690983b0c60fc22cca00283cf789df3273e8c11**: D2.4 patch побайтно совпадает после parent CI-only merge (cmp PASS, SHA25603ef5a0e1f5673bdc0ef9a9815253f14b0774bee93cf7d698bad0d975bf43d52). Reviewer read-only, без tests/lint, review не acceptance.

Actual GitHub Actions на проверенный f56b9b2 — **pending/queued**, не passed: Core contracts/lint, API verify/onboarding, ML rpc/tests/safety. Проверено API jobs:runner_id0,steps0,conclusion null. Прежний billing failure не выдаётся за текущий статус: owner отдельно опубликовал self-hosted CI #52; billing/settings/CI policies здесь не менялись. После final push нужно смотреть latest run SHA, pending не pass. Source run: https://github.com/benzocloud/hack-rnd-autumn-2026/actions/runs/40179333469.

GBrain MCP отсутствует в tool catalog, recall/remember не изображались; RTK отсутствует. Go routing skills + caveman/ponytail применены; ignored reference checkouts отсутствуют, использованы tracked go-services/ADRs/source. Active PR48–52 bodies/files и все доступные comments/reviews/inline прочитаны; discussions пусты. Изменения #52 пришли только обычными merges в remote/parent; D2.4 diff к parent не содержит frontend/Auth/deploy/CI/training/чужих plans/handoffs. `/workspace/repo-payments` остаётся clean на7771af9, его ветка/PR body не изменялись нами.

## Точный подхват

Следующая команда: **`gh pr checks 51`**.
Владельцу: дождаться/проверить CI и итогово review draft51 после48. B/C: использовать опубликованный новый profile/optional analysis DTO; настоящий Auth/browser и frontend presentation остаются отдельной интеграцией, здесь не заявлены. Принятия/merge/ready/deploy нет. Owner deadline11:45МСК прошёл во время работы; runnable checkpoint публиковался рано, реализация/gates по прямому поручению продолжены до связного результата.
