package securitycontext

import _ "embed"

//go:embed sql/find_context.sql

var findContextSQL string

//go:embed sql/find_context_locked.sql

var findContextLockedSQL string

//go:embed sql/insert_context.sql

var insertContextSQL string

//go:embed sql/insert_session.sql

var insertSessionSQL string

//go:embed sql/revoke_context.sql

var revokeContextSQL string

//go:embed sql/touch_session.sql

var touchSessionSQL string
