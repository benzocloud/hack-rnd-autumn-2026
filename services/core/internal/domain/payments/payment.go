package payments

import (
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

type Status string

const (
	Planned          Status = "planned"
	PartiallyMatched Status = "partially_matched"
	Matched          Status = "matched"
	Cancelled        Status = "cancelled"
)

type PaymentData struct {
	ID                    uuid.UUID
	ReconciliationGroupID uuid.UUID
	Scope                 ledger.Scope
	Direction             ledger.Direction
	Amount                money.Money
	ScheduledDate         ledger.Date
	Source                ledger.Source
	Category              ledger.Category
	CounterpartyName      string
	Description           string
	CreatedAt             time.Time
}

type ExpectedPayment struct {
	data      PaymentData
	remaining money.Money
	status    Status
	version   int64
}

func NewExpectedPayment(data PaymentData) (ExpectedPayment, error) {
	if err := validateExpectedPayment(data); err != nil {
		return ExpectedPayment{}, err
	}
	data.CreatedAt = data.CreatedAt.UTC()
	data.Source.ImportedAt = data.Source.ImportedAt.UTC()
	return ExpectedPayment{data: data, remaining: data.Amount, status: Planned, version: 1}, nil
}

func validateExpectedPayment(data PaymentData) error {
	if data.ID == uuid.Nil || data.CreatedAt.IsZero() {
		return ErrInvalid
	}
	for _, err := range []error{data.Scope.Validate(), data.Direction.Validate(), data.Amount.Positive(),
		data.ScheduledDate.Validate(), data.Source.Validate(), data.Category.Validate(data.Scope.CompanyID)} {
		if err != nil {
			return err
		}
	}
	return nil
}

func (p ExpectedPayment) Data() PaymentData { return p.data }

func (p ExpectedPayment) Remaining() money.Money { return p.remaining }

func (p ExpectedPayment) Status() Status { return p.status }

func (p ExpectedPayment) Version() int64 { return p.version }

func (p ExpectedPayment) IsOverdue(on ledger.Date) (bool, error) {
	if err := on.Validate(); err != nil {
		return false, err
	}
	if p.version < 1 {
		return false, ErrInvalid
	}
	return (p.status == Planned || p.status == PartiallyMatched) && p.data.ScheduledDate < on, nil
}

// Cancel preserves the allocated history and never creates a transaction.
func (p ExpectedPayment) Cancel() (ExpectedPayment, error) {
	if p.status != Planned && p.status != PartiallyMatched {
		return ExpectedPayment{}, ErrState
	}
	if p.version == math.MaxInt64 {
		return ExpectedPayment{}, ledger.ErrVersion
	}
	p.status = Cancelled
	p.version++
	return p, nil
}
