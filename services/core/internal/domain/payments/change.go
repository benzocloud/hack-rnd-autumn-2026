package payments

import (
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

// PaymentChange preserves identity, provenance and allocation history.
type PaymentChange struct {
	ReconciliationGroupID         *uuid.UUID
	Direction                     *ledger.Direction
	Amount                        *money.Money
	ScheduledDate                 *ledger.Date
	Category                      *ledger.Category
	CounterpartyName, Description *string
}

func (p ExpectedPayment) Update(change PaymentChange) (ExpectedPayment, error) {
	if p.status != Planned && p.status != PartiallyMatched {
		return ExpectedPayment{}, ErrState
	}
	if p.version == math.MaxInt64 {
		return ExpectedPayment{}, ledger.ErrVersion
	}
	if change.Direction == nil && change.Amount == nil && change.ScheduledDate == nil && change.Category == nil && change.CounterpartyName == nil && change.Description == nil && change.ReconciliationGroupID == nil {
		return ExpectedPayment{}, ErrInvalid
	}
	if p.remaining != p.data.Amount && (change.Amount != nil || change.Direction != nil) {
		return ExpectedPayment{}, ErrState
	}
	if change.ReconciliationGroupID != nil {
		p.data.ReconciliationGroupID = *change.ReconciliationGroupID
	}
	if change.Direction != nil {
		p.data.Direction = *change.Direction
	}
	if change.Amount != nil {
		p.data.Amount = *change.Amount
		p.remaining = *change.Amount
	}
	if change.ScheduledDate != nil {
		p.data.ScheduledDate = *change.ScheduledDate
	}
	if change.Category != nil {
		p.data.Category = *change.Category
	}
	if change.CounterpartyName != nil {
		p.data.CounterpartyName = *change.CounterpartyName
	}
	if change.Description != nil {
		p.data.Description = *change.Description
	}
	if err := validateExpectedPayment(p.data); err != nil {
		return ExpectedPayment{}, err
	}
	p.version++
	return p, nil
}

// RemoveMatch is forbidden after cancel: cancellation preserves its settlement
// history. Removing an active allocation restores its exact remaining amount.
func (p ExpectedPayment) RemoveMatch(m PaymentMatch) (ExpectedPayment, error) {
	if p.status != PartiallyMatched && p.status != Matched {
		return ExpectedPayment{}, ErrState
	}
	if m.removedAt != nil {
		return ExpectedPayment{}, ErrState
	}
	if m.scope != p.data.Scope || m.paymentID != p.data.ID {
		return ExpectedPayment{}, ledger.ErrScope
	}
	if p.version == math.MaxInt64 {
		return ExpectedPayment{}, ledger.ErrVersion
	}
	remaining, err := p.remaining.Add(m.amount)
	if err != nil {
		return ExpectedPayment{}, err
	}
	if remaining.Minor() > p.data.Amount.Minor() {
		return ExpectedPayment{}, ErrAllocation
	}
	p.remaining = remaining
	p.status = PartiallyMatched
	if remaining == p.data.Amount {
		p.status = Planned
	}
	p.version++
	return p, nil
}

func RestorePaymentMatch(id, paymentID, transactionID uuid.UUID, scope ledger.Scope, amount money.Money, at time.Time) (PaymentMatch, error) {
	if id == uuid.Nil || paymentID == uuid.Nil || transactionID == uuid.Nil || scope.Validate() != nil || amount.Positive() != nil || at.IsZero() {
		return PaymentMatch{}, ErrInvalid
	}
	return PaymentMatch{id: id, scope: scope, paymentID: paymentID, transactionID: transactionID, amount: amount, createdAt: at.UTC()}, nil
}
