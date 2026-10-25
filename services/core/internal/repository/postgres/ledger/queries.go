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

//go:embed sql/imported_equivalent.sql
var queryImportedEquivalent string

//go:embed sql/transaction_created_at.sql
var queryTransactionCreatedAt string

//go:embed sql/category_exists.sql
var queryCategoryExists string

//go:embed sql/list_categories.sql
var queryListCategories string

//go:embed sql/category.sql
var queryCategory string

//go:embed sql/list_transaction_views.sql
var queryListTransactionViews string

//go:embed sql/read_transaction_view.sql
var queryReadTransactionView string

//go:embed sql/save_transaction_category.sql
var querySaveTransactionCategory string

//go:embed sql/carry_history_coverage.sql
var queryCarryHistoryCoverage string
