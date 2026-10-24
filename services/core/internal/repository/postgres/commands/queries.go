package commands

import _ "embed"

//go:embed sql/reserve.sql
var queryReserve string

//go:embed sql/load_result.sql
var queryLoadResult string

//go:embed sql/save_result.sql
var querySaveResult string
