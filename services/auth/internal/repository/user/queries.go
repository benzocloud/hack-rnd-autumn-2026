package user

import _ "embed"

//go:embed sql/insert_user.sql
var insertUserSQL string

//go:embed sql/user_by_email.sql
var userByEmailSQL string
