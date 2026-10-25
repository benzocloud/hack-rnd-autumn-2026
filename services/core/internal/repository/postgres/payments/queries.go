package payments

import _ "embed"

//go:embed sql/load_payment.sql
var queryLoadPayment string

//go:embed sql/insert_payment.sql
var queryInsertPayment string

//go:embed sql/save_payment.sql
var querySavePayment string

//go:embed sql/allocated.sql
var queryAllocated string

//go:embed sql/insert_match.sql
var queryInsertMatch string

//go:embed sql/carry_history_coverage.sql
var queryCarryHistoryCoverage string

//go:embed sql/update_payment.sql
var queryUpdatePayment string

//go:embed sql/find_match.sql
var queryFindMatch string

//go:embed sql/remove_match.sql
var queryRemoveMatch string

//go:embed sql/list_payments.sql
var queryListPayments string

//go:embed sql/list_matches.sql
var queryListMatches string

//go:embed sql/find_reconciliation_group.sql
var queryFindReconciliationGroup string

//go:embed sql/list_reconciliation_groups.sql
var queryListReconciliationGroups string

//go:embed sql/insert_reconciliation_group.sql
var queryInsertReconciliationGroup string

//go:embed sql/save_reconciliation_group.sql
var querySaveReconciliationGroup string
