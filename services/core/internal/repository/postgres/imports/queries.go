package imports

import _ "embed"

//go:embed sql/seed_category.sql
var querySeedCategory string

//go:embed sql/lock_quota.sql
var queryLockQuota string

//go:embed sql/find_key.sql
var queryFindKey string

//go:embed sql/usage.sql
var queryUsage string

//go:embed sql/insert.sql
var queryInsert string

//go:embed sql/read.sql
var queryRead string

//go:embed sql/save.sql
var querySave string

//go:embed sql/append_chunk.sql
var queryAppendChunk string

//go:embed sql/chunk.sql
var queryChunk string

//go:embed sql/account.sql
var queryAccount string

//go:embed sql/claim.sql
var queryClaim string

//go:embed sql/reset_rows.sql
var queryResetRows string

//go:embed sql/reset_issues.sql
var queryResetIssues string

//go:embed sql/add_row.sql
var queryAddRow string

//go:embed sql/add_issue.sql
var queryAddIssue string

//go:embed sql/issues.sql
var queryIssues string

//go:embed sql/list.sql
var queryList string

//go:embed sql/balances.sql
var queryBalances string

//go:embed sql/cleanup.sql
var queryCleanup string

//go:embed sql/delete_receiving.sql
var queryDeleteReceiving string

//go:embed sql/delete_chunks.sql
var queryDeleteChunks string

//go:embed sql/summary.sql
var querySummary string

//go:embed sql/diagnostics.sql
var queryDiagnostics string

//go:embed sql/commit_conflicts.sql
var queryCommitConflicts string

//go:embed sql/count_new.sql
var queryCountNew string

//go:embed sql/bind_source.sql
var queryBindSource string

//go:embed sql/set_revision.sql
var querySetRevision string

//go:embed sql/staged_rows.sql
var queryStagedRows string

//go:embed sql/insert_facts_batch.sql
var queryInsertFactsBatch string

//go:embed sql/insert_eod.sql
var queryInsertEod string

//go:embed sql/verify_coverage.sql
var queryVerifyCoverage string

//go:embed sql/manual_history_exists.sql
var queryManualHistoryExists string

//go:embed sql/insert_coverage.sql
var queryInsertCoverage string

//go:embed sql/coverage_exists.sql
var queryCoverageExists string

//go:embed sql/source_company.sql
var querySourceCompany string
