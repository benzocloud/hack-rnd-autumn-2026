package postgres

import _ "embed"

//go:embed sql/insert_user.sql
var insertUserSQL string

//go:embed sql/user_by_email.sql
var userByEmailSQL string

//go:embed sql/schema_version.sql
var schemaVersionSQL string
