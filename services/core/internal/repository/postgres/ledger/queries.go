package ledger

import _ "embed"

//go:embed sql/load_account.sql
var queryLoadAccount string

//go:embed sql/load_bindings.sql
var queryLoadBindings string

//go:embed sql/save_binding.sql
var querySaveBinding string

//go:embed sql/save_account.sql
var querySaveAccount string

//go:embed sql/load_balance.sql
var queryLoadBalance string

//go:embed sql/latest_balance.sql
var queryLatestBalance string

//go:embed sql/insert_balance.sql
var queryInsertBalance string

//go:embed sql/load_transaction.sql
var queryLoadTransaction string

//go:embed sql/insert_transaction.sql
var queryInsertTransaction string

//go:embed sql/transaction_created_at.sql
var queryTransactionCreatedAt string

//go:embed sql/category_exists.sql
var queryCategoryExists string
