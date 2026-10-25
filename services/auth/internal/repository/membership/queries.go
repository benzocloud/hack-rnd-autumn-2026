package membership

import _ "embed"

//go:embed sql/insert_owner.sql
var insertOwnerSQL string

//go:embed sql/find_membership.sql

var findMembershipSQL string

//go:embed sql/list_memberships.sql

var listMembershipsSQL string
