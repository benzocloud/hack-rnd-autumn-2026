package forecasting

import (
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
)

type Fact struct {
	Transaction ledger.Transaction
	DataVersion int64
}

type OpeningBalance struct {
	Observation ledger.BalanceObservation
	DataVersion int64
}

// Calendar describes an explicit exclusion, not a calendar with no obligations.
// Current remaining/status cannot certify the calendar at a historical cutoff.
type Calendar struct {
	ExcludedPayments int64
	Reason           string
	// Captured distinguishes a frozen empty calendar from legacy count-only data.
	Captured              bool
	Payments              []payments.ExpectedPayment
	Groups                []ReconciliationGroup
	KnowledgeMode         string
	AsOf                  ledger.Date
	NoPostCutoffMovements bool
	LatestFactDate        ledger.Date
}

const (
	HistoricalCalendarUnavailable    = "historical_calendar_unavailable"
	HistoricalAvailabilityUnverified = "historical_availability_unverified"
)

type SnapshotData struct {
	ID          uuid.UUID
	Scope       ledger.Scope
	DataVersion int64
	Cutoff      ledger.Date
	CreatedAt   time.Time
	Opening     OpeningBalance
	Facts       []Fact
	Calendar    Calendar
}

type FinancialSnapshot struct {
	data SnapshotData
}

func NewFinancialSnapshot(data SnapshotData) (FinancialSnapshot, error) {
	if err := validateSnapshot(data); err != nil {
		return FinancialSnapshot{}, err
	}
	data.CreatedAt = data.CreatedAt.UTC()
	data.Facts = slices.Clone(data.Facts)
	data.Calendar = cloneCalendar(data.Calendar)
	return FinancialSnapshot{data: data}, nil
}

func validateSnapshot(data SnapshotData) error {
	if data.ID == uuid.Nil || data.DataVersion < 1 || data.CreatedAt.IsZero() {
		return ErrInvalid
	}
	if err := data.Scope.Validate(); err != nil {
		return err
	}
	if err := data.Cutoff.Validate(); err != nil {
		return err
	}
	if data.Opening.Observation.Scope() != data.Scope || data.Opening.Observation.Date() != data.Cutoff ||
		data.Opening.DataVersion < 1 || data.Opening.DataVersion > data.DataVersion {
		return ErrInvalid
	}
	if err := data.Opening.Observation.Amount().Validate(); err != nil {
		return err
	}
	if _, err := ledger.NewBalanceObservation(data.Scope, data.Cutoff, data.Opening.Observation.Amount(), data.Opening.Observation.Source()); err != nil {
		return err
	}
	if err := validateCalendar(data.Calendar, data.Scope, data.Cutoff, data.Facts); err != nil {
		return ErrInvalid
	}
	ids := make(map[uuid.UUID]struct{}, len(data.Facts))
	for _, fact := range data.Facts {
		d := fact.Transaction.Data()
		if d.Scope != data.Scope || d.Date > data.Cutoff || fact.DataVersion < 1 || fact.DataVersion > data.DataVersion {
			return ErrInvalid
		}
		if _, err := ledger.RestoreTransaction(d, fact.Transaction.Version()); err != nil {
			return err
		}
		if _, duplicate := ids[d.ID]; duplicate {
			return ErrInvalid
		}
		ids[d.ID] = struct{}{}
	}
	return nil
}

// Data returns caller-owned slices; later writes cannot alter an existing snapshot.
func (s FinancialSnapshot) Data() SnapshotData {
	data := s.data
	data.Facts = slices.Clone(data.Facts)
	data.Calendar = cloneCalendar(data.Calendar)
	return data
}

// Historical business dates are preserved, but this schema has no certified
// historical knowledge log. This limitation must accompany downstream inputs.
func (s FinancialSnapshot) Limitations() []string {
	if s.data.Calendar.KnowledgeMode == "current" {
		return []string{HistoricalAvailabilityUnverified}
	}
	return []string{HistoricalAvailabilityUnverified, HistoricalCalendarUnavailable}
}

func cloneCalendar(c Calendar) Calendar {
	c.Payments = slices.Clone(c.Payments)
	c.Groups = slices.Clone(c.Groups)
	if len(c.Groups) == 0 {
		c.Groups = nil
	}
	if len(c.Payments) == 0 {
		c.Payments = nil
	}
	for i := range c.Groups {
		c.Groups[i].Operations = slices.Clone(c.Groups[i].Operations)
	}
	return c
}
func validateCalendar(c Calendar, scope ledger.Scope, cutoff ledger.Date, facts []Fact) error {
	if c.ExcludedPayments < 0 || (c.Reason != HistoricalCalendarUnavailable && c.Reason != "") {
		return ErrInvalid
	}
	if !c.Captured {
		if c.Reason != HistoricalCalendarUnavailable || len(c.Payments) > 0 || len(c.Groups) > 0 || c.KnowledgeMode != "" || c.AsOf != "" || c.NoPostCutoffMovements || c.LatestFactDate != "" {
			return ErrInvalid
		}
		return nil
	}
	if c.KnowledgeMode != "unavailable" && c.KnowledgeMode != "current" {
		return ErrInvalid
	}
	if c.KnowledgeMode == "unavailable" && (c.Reason != HistoricalCalendarUnavailable || c.AsOf != "" || c.NoPostCutoffMovements || c.ExcludedPayments != int64(len(c.Payments))) {
		return ErrInvalid
	}
	if c.KnowledgeMode == "current" && (c.Reason != "" || c.AsOf.Validate() != nil) {
		return ErrInvalid
	}
	if len(c.Payments) > 1000 || len(c.Groups) > 1000 || c.LatestFactDate != "" && c.LatestFactDate.Validate() != nil {
		return ErrInvalid
	}
	groups := map[uuid.UUID]bool{}
	replaced := map[uuid.UUID]bool{}
	operations := map[uuid.UUID]int64{}
	for _, f := range facts {
		operations[f.Transaction.Data().ID] = f.Transaction.Version()
	}
	at, _ := time.Parse(time.DateOnly, string(cutoff))
	start, end := ledger.Date(at.AddDate(0, 0, 1).Format(time.DateOnly)), ledger.Date(at.AddDate(0, 0, 30).Format(time.DateOnly))
	if c.KnowledgeMode == "current" && (c.AsOf < cutoff || c.AsOf > start || c.AsOf > cutoff && !c.NoPostCutoffMovements || c.LatestFactDate > cutoff) {
		return ErrInvalid
	}
	for _, g := range c.Groups {
		if g.ID == uuid.Nil || g.Scope != scope || g.Version < 1 || g.Status != "active" || groups[g.ID] || g.Cutoff.Validate() != nil || g.HorizonStart.Validate() != nil || g.HorizonEnd.Validate() != nil || g.ConfirmedBy == "" || g.ConfirmedAt.IsZero() || g.Additional != (len(g.Operations) == 0) || len(g.Operations) > 20000 {
			return ErrInvalid
		}
		groupCutoff, _ := time.Parse(time.DateOnly, string(g.Cutoff))
		if string(g.HorizonStart) != groupCutoff.AddDate(0, 0, 1).Format(time.DateOnly) || string(g.HorizonEnd) != groupCutoff.AddDate(0, 0, 30).Format(time.DateOnly) {
			return ErrInvalid
		}
		local := map[uuid.UUID]bool{}
		for _, ref := range g.Operations {
			if ref.ID == uuid.Nil || ref.Version < 1 || local[ref.ID] {
				return ErrInvalid
			}
			local[ref.ID] = true
			if c.KnowledgeMode == "current" {
				if replaced[ref.ID] || operations[ref.ID] != ref.Version {
					return ErrInvalid
				}
				replaced[ref.ID] = true
			}
		}
		if c.KnowledgeMode == "current" && (g.Cutoff != cutoff || g.HorizonStart != start || g.HorizonEnd != end) {
			return ErrInvalid
		}
		groups[g.ID] = true
	}
	seen := map[uuid.UUID]bool{}
	for _, p := range c.Payments {
		d := p.Data()
		if d.Scope != scope || seen[d.ID] {
			return ErrInvalid
		}
		if _, err := payments.RestoreExpectedPayment(d, p.Remaining(), p.Status(), p.Version()); err != nil {
			return ErrInvalid
		}
		if c.KnowledgeMode == "current" && (p.Status() == payments.Planned || p.Status() == payments.PartiallyMatched) && d.ScheduledDate <= end {
			if d.ScheduledDate < start || !groups[d.ReconciliationGroupID] {
				return ErrInvalid
			}
		}
		seen[d.ID] = true
	}
	return nil
}
