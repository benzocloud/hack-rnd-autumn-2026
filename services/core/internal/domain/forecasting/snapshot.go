package forecasting

import (
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
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
	if data.Calendar.Reason != HistoricalCalendarUnavailable || data.Calendar.ExcludedPayments < 0 {
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
	return data
}

// Historical business dates are preserved, but this schema has no certified
// historical knowledge log. This limitation must accompany downstream inputs.
func (s FinancialSnapshot) Limitations() []string {
	return []string{HistoricalAvailabilityUnverified, HistoricalCalendarUnavailable}
}
