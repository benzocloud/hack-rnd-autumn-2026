package companycreation

import _ "embed"

//go:embed sql/insert.sql
var insertSQL string

//go:embed sql/find.sql
var findSQL string

//go:embed sql/find_locked.sql
var findLockedSQL string

//go:embed sql/find_by_key.sql
var findByKeySQL string

//go:embed sql/complete.sql
var completeSQL string

//go:embed sql/quarantine.sql
var quarantineSQL string

//go:embed sql/claim.sql
var claimSQL string

//go:embed sql/retry.sql
var retrySQL string
