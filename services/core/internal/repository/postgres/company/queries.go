package company

import _ "embed"

//go:embed sql/reserve_bootstrap.sql
var reserveBootstrapSQL string

//go:embed sql/read_bootstrap.sql
var readBootstrapSQL string

//go:embed sql/read_profile.sql
var readProfileSQL string

//go:embed sql/insert_company.sql
var insertCompanySQL string

//go:embed sql/insert_account.sql
var insertAccountSQL string

//go:embed sql/insert_profile.sql
var insertProfileSQL string

//go:embed sql/update_profile.sql
var updateProfileSQL string

//go:embed sql/seed_category.sql
var seedCategorySQL string
