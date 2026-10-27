package forecasting

import (
	"math"
	"testing"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

func TestReceiptDelayAndExpenseResponseConserveParts(t *testing.T) {
	base := salesView(t)
	base.Sales = ScenarioSales(base)
	expense := ExpenseChange{ID: uuid.New(), Date: base.Result.Days[2].Date, AmountMinor: 70000, Description: "equipment"}
	result, err := ApplyScenario(*base.Result, []ExpenseChange{expense})
	if err != nil {
		t.Fatal(err)
	}
	view := base
	view.Result = &result
	view.ScenarioExpenses = []ExpenseChange{expense}
	original, err := ScenarioOccurrences(view)
	if err != nil {
		t.Fatal(err)
	}
	delay := DelayReceiptChange{ID: uuid.New(), Target: ReceiptTarget{ComponentID: base.Sales[0].ID, OccurrenceID: base.Sales[0].ID}, AmountMinor: 80, NewDate: base.Result.Days[3].Date}
	stressed, parts, err := applyReceiptDelays(result, original, []DelayReceiptChange{delay})
	if err != nil {
		t.Fatal(err)
	}
	if stressed.Days[0].Inflow != 9920 || stressed.Days[3].Inflow != 10080 || stressed.Days[2].Balance != 29920 || stressed.Days[3].Balance != 30000 || stressed.Days[29].Balance != 30000 {
		t.Fatal("delay lost amount or wrong dates")
	}
	if original[0].RemainingMinor != 200 || parts[0].RemainingMinor != 120 || parts[len(parts)-1].RemainingMinor != 80 || base.Result.Days[0].Balance != 100000 {
		t.Fatal("original parts or base mutated")
	}
	var target EffectiveOccurrence
	for _, c := range parts {
		if c.ComponentID == expense.ID.String() {
			target = c
		}
	}
	action := &ResponseAction{EffectiveOccurrenceID: target.ID, AmountMinor: 30000, NewDate: base.Result.Days[10].Date, FeasibilityCondition: "Supplier agrees"}
	owned, _ := ApplyScenario(stressed, nil)
	response, responseParts, err := applyResponse(owned, parts, action, "response:one")
	if err != nil {
		t.Fatal(err)
	}
	if response.Days[2].Balance != 59920 || response.Days[3].Balance != 60000 || response.Days[10].Balance != 30000 || response.Days[29].Balance != 30000 || response.Days[2].Outflow != 50000 || response.Days[10].Outflow != 40000 {
		t.Fatal("response discarded stress or wrong flow")
	}
	if stressed.Days[2].Balance != 29920 {
		t.Fatal("parent stress mutated")
	}
	remainder := int64(0)
	for _, c := range responseParts {
		if c.ID == target.ID || c.ParentID == target.ID {
			remainder += c.RemainingMinor
		}
	}
	if remainder != 70000 {
		t.Fatal("expense parts not conserved")
	}
	overflow := *action
	overflow.AmountMinor = 40001
	owned, _ = ApplyScenario(response, nil)
	if _, _, err := applyResponse(owned, responseParts, &overflow, "response:over"); err == nil {
		t.Fatal("already moved budget reused")
	}
	final := *action
	final.AmountMinor = 40000
	final.NewDate = "2026-11-01"
	owned, _ = ApplyScenario(response, nil)
	future, futureParts, err := applyResponse(owned, responseParts, &final, "response:future")
	if err != nil {
		t.Fatal(err)
	}
	if future.Days[29].Balance != 70000 || len(outsideHorizon(futureParts, future)) != 1 {
		t.Fatal("out-of-horizon expense disappeared")
	}
	owned, _ = ApplyScenario(*base.Result, nil)
	delayed, delayedParts, err := applyReceiptDelays(owned, original[:30], []DelayReceiptChange{{ID: uuid.New(), Target: delay.Target, AmountMinor: 80, NewDate: "2026-11-01"}})
	if err != nil || delayed.Days[29].Balance != 99920 || len(outsideHorizon(delayedParts, delayed)) != 1 {
		t.Fatal("out-of-horizon receipt disappeared", err)
	}
}

func TestReschedulingRejectsForeignDirectionDateBudgetAndOverflow(t *testing.T) {
	base := salesView(t)
	base.Sales = ScenarioSales(base)
	original, err := ScenarioOccurrences(base)
	if err != nil {
		t.Fatal(err)
	}
	receipt := original[0]
	for _, mode := range []string{"foreign", "same date", "over budget", "missing condition", "wrong direction", "outside source"} {
		t.Run(mode, func(t *testing.T) {
			action := &ResponseAction{EffectiveOccurrenceID: receipt.ID, AmountMinor: 1, NewDate: "2026-10-02", FeasibilityCondition: "Agreed"}
			parts := append([]EffectiveOccurrence(nil), original...)
			parts[0].Direction = ledger.Outflow
			switch mode {
			case "foreign":
				action.EffectiveOccurrenceID = "foreign"
			case "same date":
				action.NewDate = receipt.Date
			case "over budget":
				action.AmountMinor = 201
			case "missing condition":
				action.FeasibilityCondition = " "
			case "wrong direction":
				parts[0].Direction = ledger.Inflow
			case "outside source":
				parts[0].Date = "2026-11-01"
			}
			owned, _ := ApplyScenario(*base.Result, nil)
			if _, _, err := applyResponse(owned, parts, action, "response:invalid"); err == nil {
				t.Fatal("unsafe response accepted")
			}
		})
	}
	owned, _ := ApplyScenario(*base.Result, nil)
	owned.Days[0].Balance = math.MaxInt64
	parts := append([]EffectiveOccurrence(nil), original...)
	parts[0].Direction = ledger.Outflow
	if _, _, err := applyResponse(owned, parts, &ResponseAction{EffectiveOccurrenceID: receipt.ID, AmountMinor: 1, NewDate: "2026-10-02", FeasibilityCondition: "Agreed"}, "response:overflow"); err == nil {
		t.Fatal("positive balance overflow accepted")
	}
	owned, _ = ApplyScenario(*base.Result, nil)
	owned.Days[2].Inflow = math.MaxInt64
	if _, _, err := applyReceiptDelays(owned, original, []DelayReceiptChange{{ID: uuid.New(), Target: ReceiptTarget{ComponentID: receipt.ComponentID, OccurrenceID: receipt.OccurrenceID}, AmountMinor: 1, NewDate: owned.Days[2].Date}}); err == nil {
		t.Fatal("inflow overflow accepted")
	}
}
