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
