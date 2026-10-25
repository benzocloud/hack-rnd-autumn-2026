package invitation

import _ "embed"

//go:embed sql/insert.sql
var insertSQL string

//go:embed sql/find.sql
var findSQL string

//go:embed sql/find_locked.sql
var findLockedSQL string

//go:embed sql/find_create_idempotency.sql
var findCreateIdempotencySQL string

//go:embed sql/list_outgoing.sql
var listOutgoingSQL string

//go:embed sql/list_incoming.sql
var listIncomingSQL string

//go:embed sql/save.sql
var saveSQL string
