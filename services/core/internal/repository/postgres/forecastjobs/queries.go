package forecastjobs

import _ "embed"

//go:embed sql/lock_revision.sql
var queryLockRevision string

//go:embed sql/read_coverage.sql
var queryReadCoverage string

//go:embed sql/insert_job.sql
var queryInsertJob string

//go:embed sql/read_job.sql
var queryReadJob string

//go:embed sql/list_jobs.sql
var queryListJobs string

//go:embed sql/claim_job.sql
var queryClaimJob string

//go:embed sql/finish_job.sql
var queryFinishJob string
