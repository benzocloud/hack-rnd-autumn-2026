package membership

import _ "embed"

//go:embed sql/insert_owner.sql
var insertOwnerSQL string

//go:embed sql/insert_financier.sql
var insertFinancierSQL string

//go:embed sql/delete_financier.sql
var deleteFinancierSQL string

//go:embed sql/find_membership.sql
var findMembershipSQL string

//go:embed sql/list_memberships.sql
var listMembershipsSQL string

//go:embed sql/list_company_members.sql
var listCompanyMembersSQL string

//go:embed sql/lock_company_memberships.sql
var lockCompanyMembershipsSQL string
