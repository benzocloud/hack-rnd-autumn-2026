package forecasting

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

func TestExpenseScenarioPreservesBaseAndPropagatesBalance(t *testing.T) {
	base := SavedForecast{SchemaVersion: 1}
	start := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	for n := range 30 {
		base.Days = append(base.Days, SavedForecastDay{Date: ledger.Date(start.AddDate(0, 0, n).Format(time.DateOnly)), Inflow: 100, Outflow: 50, Balance: 1000 + int64(n)*50})
	}
	changes := []ExpenseChange{{ID: uuid.New(), Date: "2026-10-02", AmountMinor: 1500, Description: "equipment"}, {ID: uuid.New(), Date: "2026-10-03", AmountMinor: 100, Description: "extra"}}
	result, err := ApplyScenario(base, changes)
	if err != nil {
		t.Fatal(err)
	}
	if base.Days[2].Balance != 1100 || base.Days[2].Outflow != 50 || base.FirstNegativeDate != nil {
		t.Fatal("base mutated")
	}
	if result.Days[1].Balance != 1050 || result.Days[2].Balance != -400 || result.Days[2].Outflow != 1550 || result.Days[3].Balance != -450 || result.Days[29].Balance != 850 || result.FirstNegativeDate == nil || *result.FirstNegativeDate != "2026-10-02" {
		t.Fatal("incorrect propagation", result)
	}
	for _, date := range []ledger.Date{"2026-09-29", "2026-10-30"} {
		if _, err := ApplyScenario(base, []ExpenseChange{{Date: date, AmountMinor: 1}}); err == nil {
			t.Fatal("out-of-horizon expense accepted")
		}
	}
	base.Days[0].Outflow = math.MaxInt64
	if _, err := ApplyScenario(base, []ExpenseChange{{Date: base.Days[0].Date, AmountMinor: 1}}); err == nil {
		t.Fatal("outflow overflow accepted")
	}
	base.Days[0].Outflow = 0
	base.Days[0].Balance = math.MinInt64 + 1
	if _, err := ApplyScenario(base, []ExpenseChange{{Date: base.Days[0].Date, AmountMinor: 2}}); err == nil {
		t.Fatal("balance underflow accepted")
	}
}
