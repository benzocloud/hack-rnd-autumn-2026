package forecasting

import (
	"math"
	"math/big"
	"slices"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

const scenarioSalesRule = "classified_sales_mean_90_v1"

type SalesComponent struct {
	ID                          string
	Date                        ledger.Date
	AmountMinor, RemainingMinor int64
	CategoryID                  uuid.UUID
}
type SalesTarget struct{ ComponentID, OccurrenceID string }
type SalesReductionChange struct {
	ID                     uuid.UUID
	Targets                []SalesTarget
	PeriodStart, PeriodEnd ledger.Date
	ReductionBasisPoints   uint32
}

func salesCode(code string) bool {
	return slices.Contains([]string{"customer_payment", "agency_revenue", "acquiring_revenue", "cash_revenue", "marketplace_revenue"}, code)
}

// ScenarioSales decomposes only the existing limited mean_90 CSV profiles.
// The original projection stays unchanged. Loans, transfers, unknown inflows,
// and known calendar payments never become sales targets.
func ScenarioSales(view ForecastView) []SalesComponent {
	if view.Status != "succeeded" || view.Result == nil || view.Result.Analysis == nil || view.Result.Analysis.Version != AnalysisVersion || view.Assessment == nil || !AnalysisProfile(view.Model.Version) || len(view.Result.Days) != 30 {
		return nil
	}
	salesIDs := map[uuid.UUID]bool{}
	var categoryID uuid.UUID
	for _, c := range view.Categories {
		if c.CatalogueCode == "sales" && c.CompanyID == view.Scope.CompanyID && c.Allows(ledger.Inflow) {
			salesIDs[c.ID] = true
			categoryID = c.ID
		}
	}
	var perDay map[ledger.Date]int64
	if CSVCalendarProfile(view.Model.Version) {
		perDay = calendarSales(view, salesIDs)
	} else {
		perDay = historicalSales(view, salesIDs)
	}
	out := []SalesComponent{}
	for _, d := range view.Result.Days {
		amount := perDay[d.Date]
		if amount < 0 || amount > d.Inflow {
			return nil
		}
		if amount > 0 {
			out = append(out, SalesComponent{ID: scenarioSalesRule + ":" + view.Snapshot.Data().ID.String() + ":" + string(d.Date), Date: d.Date, AmountMinor: amount, RemainingMinor: amount, CategoryID: categoryID})
		}
	}
	return out
}

func validateSalesChange(change SalesReductionChange) error {
	if change.ID == uuid.Nil || len(change.Targets) < 1 || len(change.Targets) > 1000 || change.PeriodStart.Validate() != nil || change.PeriodEnd.Validate() != nil || change.PeriodStart > change.PeriodEnd || change.ReductionBasisPoints < 1 || change.ReductionBasisPoints > 10000 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, t := range change.Targets {
		if t.ComponentID == "" || len(t.ComponentID) > 128 || t.ComponentID != t.OccurrenceID || seen[t.ComponentID] {
			return ErrInvalid
		}
		seen[t.ComponentID] = true
	}
	return nil
}

// applySalesReductions rounds each selected daily sales reduction half-up in
// kopecks. A target may be changed once; changes never compound accidentally.
func applySalesReductions(result SavedForecast, basis []SalesComponent, changes []SalesReductionChange) (SavedForecast, []SalesComponent, error) {
	components := slices.Clone(basis)
	indices := map[string]int{}
	for i, c := range components {
		indices[c.ID] = i
	}
	used := map[string]bool{}
	for _, change := range changes {
		if validateSalesChange(change) != nil || len(result.Days) != 30 || change.PeriodStart < result.Days[0].Date || change.PeriodEnd > result.Days[29].Date {
			return SavedForecast{}, nil, ErrInvalid
		}
		matched := false
		for _, target := range change.Targets {
			index, ok := indices[target.ComponentID]
			if !ok {
				return SavedForecast{}, nil, ErrInvalid
			}
			c := &components[index]
			if c.Date < change.PeriodStart || c.Date > change.PeriodEnd {
				continue
			}
			if used[c.ID] {
				return SavedForecast{}, nil, ErrInvalid
			}
			used[c.ID] = true
			matched = true
			loss := new(big.Int).Mul(big.NewInt(c.RemainingMinor), new(big.Int).SetUint64(uint64(change.ReductionBasisPoints)))
			loss.Add(loss, big.NewInt(5000))
			loss.Quo(loss, big.NewInt(10000))
			if !loss.IsInt64() || loss.Sign() < 0 || loss.Int64() > c.RemainingMinor {
				return SavedForecast{}, nil, ErrInvalid
			}
			removed := loss.Int64()
			c.RemainingMinor -= removed
			for i := range result.Days {
				d := &result.Days[i]
				if d.Date < c.Date {
					continue
				}
				if d.Date == c.Date {
					if removed > d.Inflow {
						return SavedForecast{}, nil, ErrInvalid
					}
					d.Inflow -= removed
				}
				balance, _ := money.New(d.Balance, money.RUB)
				amount, _ := money.New(removed, money.RUB)
				next, e := balance.Sub(amount)
				if e != nil || next.Minor() == math.MinInt64 {
					return SavedForecast{}, nil, ErrInvalid
				}
				d.Balance = next.Minor()
			}
		}
		if !matched {
			return SavedForecast{}, nil, ErrInvalid
		}
	}
	result.FirstNegativeDate = nil
	for _, d := range result.Days {
		if d.Balance < 0 {
			date := d.Date
			result.FirstNegativeDate = &date
			break
		}
	}
	if len(changes) > 0 {
		result.Assumptions = append(result.Assumptions, scenarioSalesRule+": reduction of classified residual sales only; loans, own transfers and calendar payments unchanged; per-day half-up kopeck rounding")
	}
	return result, components, nil
}

func calendarSales(view ForecastView, salesIDs map[uuid.UUID]bool) map[ledger.Date]int64 {
	perDay := map[ledger.Date]int64{}
	// The model already performed exact replacement/rounding allocation. Reuse
	// those saved residual components rather than reducing calendar payments.
	for _, c := range view.Result.Components {
		eligible := salesCode(c.CategoryCode) || c.CategoryCode == "unknown" && salesIDs[c.CategoryID]
		if c.Kind != "mean_90_uncovered" || c.InflowMinor <= 0 || c.OutflowMinor != 0 || !eligible {
			continue
		}
		if perDay[c.Date] > math.MaxInt64-c.InflowMinor {
			return nil
		}
		perDay[c.Date] += c.InflowMinor
	}
	return perDay
}
func historicalSales(view ForecastView, salesIDs map[uuid.UUID]bool) map[ledger.Date]int64 {
	perDay := map[ledger.Date]int64{}
	labels := map[uuid.UUID]OperationAnalysis{}
	for _, c := range view.Result.Analysis.Operations {
		labels[c.ID] = c
	}
	total := new(big.Int)
	for _, fact := range view.Snapshot.Data().Facts {
		d := fact.Transaction.Data()
		label, ok := labels[d.ID]
		if !ok || label.Version != fact.Transaction.Version() || d.Direction != ledger.Inflow || d.Date < view.Assessment.From || d.Date > view.Cutoff {
			continue
		}
		eligible := label.CategorySource == "rules" && salesCode(label.CategoryCode) || label.CategorySource == "manual" && salesIDs[label.CategoryID]
		if eligible {
			total.Add(total, big.NewInt(d.Amount.Minor()))
		}
	}
	total.Add(total, big.NewInt(45))
	total.Quo(total, big.NewInt(90))
	if !total.IsInt64() || total.Sign() <= 0 {
		return nil
	}
	for _, d := range view.Result.Days {
		perDay[d.Date] = total.Int64()
	}
	return perDay
}
