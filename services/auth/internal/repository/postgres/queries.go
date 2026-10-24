package postgres

import _ "embed"

//go:embed sql/schema_version.sql
var schemaVersionSQL string
