package membership

import _ "embed"

//go:embed sql/find_membership.sql

var findMembershipSQL string

//go:embed sql/list_memberships.sql

var listMembershipsSQL string
