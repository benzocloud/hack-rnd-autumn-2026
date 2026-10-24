package forecasting

import _ "embed"

//go:embed sql/read_revision.sql
var queryReadRevision string

//go:embed sql/read_facts.sql
var queryReadFacts string

//go:embed sql/read_calendar.sql
var queryReadCalendar string

//go:embed sql/insert_snapshot.sql
var queryInsertSnapshot string

//go:embed sql/read_snapshot.sql
var queryReadSnapshot string
