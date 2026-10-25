package forecasting

import (
	"crypto/sha256"
	"fmt"
	"math"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

// EffectiveOccurrence retains the unused original portion and every moved part.
// ParentID provides provenance; calendar/ledger entities never change.
type EffectiveOccurrence struct {
	ID, ComponentID, OccurrenceID, ParentID, Kind string
	Direction                                     ledger.Direction
	Date                                          ledger.Date
	AmountMinor, RemainingMinor                   int64
	PaymentID, CategoryID                         uuid.UUID
}
type ReceiptTarget struct{ ComponentID, OccurrenceID string }
type DelayReceiptChange struct {
	ID          uuid.UUID
	Target      ReceiptTarget
	AmountMinor int64
	NewDate     ledger.Date
}
type ResponseAction struct {
	EffectiveOccurrenceID string
	AmountMinor           int64
	NewDate               ledger.Date
	FeasibilityCondition  string
}

func effectiveID(component, occurrence string) string {
	return fmt.Sprintf("effective:%x", sha256.Sum256([]byte(component+"\x00"+occurrence)))
}

func ScenarioOccurrences(view ForecastView) ([]EffectiveOccurrence, error) {
	out := []EffectiveOccurrence{}
	for _, c := range view.Sales {
		out = append(out, EffectiveOccurrence{ID: effectiveID(c.ID, c.ID), ComponentID: c.ID, OccurrenceID: c.ID, Kind: "ml_sales_receipt", Direction: ledger.Inflow, Date: c.Date, AmountMinor: c.AmountMinor, RemainingMinor: c.RemainingMinor, CategoryID: c.CategoryID})
	}
	categories := map[uuid.UUID]uuid.UUID{}
	for _, p := range view.Snapshot.Data().Calendar.Payments {
		categories[p.Data().ID] = p.Data().Category.ID
	}
	for _, c := range view.Result.Components {
		if c.Kind != "known_remaining_payment" {
			continue
		}
		if len(c.SourceIDs) != 1 {
			return nil, ErrUnavailable
		}
		payment, err := uuid.Parse(c.SourceIDs[0])
		if err != nil || payment == uuid.Nil {
			return nil, ErrUnavailable
		}
		direction := ledger.Inflow
		amount := c.InflowMinor
		if c.OutflowMinor > 0 {
			direction = ledger.Outflow
			amount = c.OutflowMinor
		}
		if amount <= 0 || c.InflowMinor > 0 && c.OutflowMinor > 0 {
			return nil, ErrUnavailable
		}
		out = append(out, EffectiveOccurrence{ID: effectiveID(c.ID, c.ID), ComponentID: c.ID, OccurrenceID: c.ID, Kind: "known_payment", Direction: direction, Date: c.Date, AmountMinor: amount, RemainingMinor: amount, PaymentID: payment, CategoryID: categories[payment]})
	}
	for _, c := range view.ScenarioExpenses {
		id := c.ID.String()
		out = append(out, EffectiveOccurrence{ID: effectiveID(id, id), ComponentID: id, OccurrenceID: id, Kind: "scenario_expense", Direction: ledger.Outflow, Date: c.Date, AmountMinor: c.AmountMinor, RemainingMinor: c.AmountMinor})
	}
	seen := map[string]bool{}
	for _, c := range out {
		if c.ID == "" || seen[c.ID] || c.Date.Validate() != nil || c.RemainingMinor < 0 || c.RemainingMinor > c.AmountMinor {
			return nil, ErrUnavailable
		}
		seen[c.ID] = true
	}
	return out, nil
}

func validateDelay(c DelayReceiptChange) error {
	if c.ID == uuid.Nil || c.Target.ComponentID == "" || len(c.Target.ComponentID) > 128 || c.Target.OccurrenceID == "" || len(c.Target.OccurrenceID) > 128 || c.AmountMinor <= 0 || c.NewDate.Validate() != nil {
		return ErrInvalid
	}
	return nil
}
func validateResponse(c *ResponseAction) error {
	if c == nil || c.EffectiveOccurrenceID == "" || len(c.EffectiveOccurrenceID) > 128 || c.AmountMinor <= 0 || c.NewDate.Validate() != nil || strings.TrimSpace(c.FeasibilityCondition) == "" || !utf8.ValidString(c.FeasibilityCondition) || utf8.RuneCountInString(c.FeasibilityCondition) > 1000 {
		return ErrInvalid
	}
	return nil
}

// shiftFlow conserves the amount: remove it from the old day and add it to the
// new day when in horizon. A later part stays in the occurrence ledger otherwise.
func shiftFlow(result *SavedForecast, from, to ledger.Date, direction ledger.Direction, amount int64) error {
	if len(result.Days) != 30 || from < result.Days[0].Date || from > result.Days[29].Date || to.Validate() != nil || to <= from || amount <= 0 || (direction != ledger.Inflow && direction != ledger.Outflow) {
		return ErrInvalid
	}
	for i := range result.Days {
		d := &result.Days[i]
		if d.Date == from {
			flow := &d.Inflow
			if direction == ledger.Outflow {
				flow = &d.Outflow
			}
			if *flow < amount {
				return ErrInvalid
			}
			*flow -= amount
		}
		if d.Date == to {
			flow := &d.Inflow
			if direction == ledger.Outflow {
				flow = &d.Outflow
			}
			if *flow > math.MaxInt64-amount {
				return ErrInvalid
			}
			*flow += amount
		}
		if d.Date >= from && d.Date < to {
			delta := -amount
			if direction == ledger.Outflow {
				delta = amount
			}
			balance, _ := money.New(d.Balance, money.RUB)
			change, _ := money.New(delta, money.RUB)
			next, err := balance.Add(change)
			if err != nil || next.Minor() == math.MinInt64 {
				return ErrInvalid
			}
			d.Balance = next.Minor()
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
	return nil
}

func movePart(result *SavedForecast, occurrences []EffectiveOccurrence, index int, amount int64, date ledger.Date, partID string) ([]EffectiveOccurrence, error) {
	source := occurrences[index]
	if source.RemainingMinor < amount || amount <= 0 {
		return nil, ErrInvalid
	}
	if err := shiftFlow(result, source.Date, date, source.Direction, amount); err != nil {
		return nil, err
	}
	for _, c := range occurrences {
		if c.ComponentID == partID {
			return nil, ErrInvalid
		}
	}
	occurrences[index].RemainingMinor -= amount
	part := source
	part.ID = effectiveID(partID, partID)
	part.ComponentID = partID
	part.OccurrenceID = partID
	part.ParentID = source.ID
	part.Date = date
	part.AmountMinor = amount
	part.RemainingMinor = amount
	return append(occurrences, part), nil
}
func applyReceiptDelays(result SavedForecast, original []EffectiveOccurrence, changes []DelayReceiptChange) (SavedForecast, []EffectiveOccurrence, error) {
	occurrences := slices.Clone(original)
	for _, change := range changes {
		if validateDelay(change) != nil {
			return SavedForecast{}, nil, ErrInvalid
		}
		index := -1
		for i, c := range occurrences {
			if c.ComponentID == change.Target.ComponentID && c.OccurrenceID == change.Target.OccurrenceID {
				index = i
				break
			}
		}
		if index < 0 || occurrences[index].Direction != ledger.Inflow {
			return SavedForecast{}, nil, ErrInvalid
		}
		var err error
		occurrences, err = movePart(&result, occurrences, index, change.AmountMinor, change.NewDate, "delay:"+change.ID.String())
		if err != nil {
			return SavedForecast{}, nil, err
		}
	}
	if len(changes) > 0 {
		result.Assumptions = append(result.Assumptions, "Поступление перенесено на указанную дату; часть за пределами горизонта сохранена и не включается в остаток этого периода.")
	}
	return result, occurrences, nil
}
func applyResponse(result SavedForecast, original []EffectiveOccurrence, action *ResponseAction, partID string) (SavedForecast, []EffectiveOccurrence, error) {
	if validateResponse(action) != nil {
		return SavedForecast{}, nil, ErrInvalid
	}
	occurrences := slices.Clone(original)
	index := -1
	for i, c := range occurrences {
		if c.ID == action.EffectiveOccurrenceID {
			index = i
			break
		}
	}
	if index < 0 || occurrences[index].Direction != ledger.Outflow {
		return SavedForecast{}, nil, ErrInvalid
	}
	occurrences, err := movePart(&result, occurrences, index, action.AmountMinor, action.NewDate, partID)
	if err != nil {
		return SavedForecast{}, nil, err
	}
	result.Assumptions = append(result.Assumptions, "Часть расхода перенесена при условии: "+action.FeasibilityCondition+". Исходные платежи и календарь не изменены.")
	return result, occurrences, nil
}
func outsideHorizon(occurrences []EffectiveOccurrence, result SavedForecast) []string {
	ids := []string{}
	if len(result.Days) != 30 {
		return ids
	}
	for _, c := range occurrences {
		if c.RemainingMinor > 0 && c.Date > result.Days[29].Date {
			ids = append(ids, c.ID)
		}
	}
	slices.Sort(ids)
	return ids
}
