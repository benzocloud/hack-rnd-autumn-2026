package payments

import (
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

type PaymentMatch struct {
	id            uuid.UUID
	scope         ledger.Scope
	paymentID     uuid.UUID
	transactionID uuid.UUID
	amount        money.Money
	createdAt     time.Time
}

func (m PaymentMatch) ID() uuid.UUID { return m.id }

func (m PaymentMatch) Scope() ledger.Scope { return m.scope }

func (m PaymentMatch) PaymentID() uuid.UUID { return m.paymentID }

func (m PaymentMatch) TransactionID() uuid.UUID { return m.transactionID }

func (m PaymentMatch) Amount() money.Money { return m.amount }

func (m PaymentMatch) CreatedAt() time.Time { return m.createdAt }

// Match returns a new payment and allocation without changing either input.
// The repository must read factAllocated for this exact
// scoped transaction and persist both results under one database transaction.
// Pure domain checks alone do not protect against concurrent allocations or replay.
func (p ExpectedPayment) Match(id uuid.UUID, fact ledger.Transaction, factAllocated, amount money.Money, at time.Time) (ExpectedPayment, PaymentMatch, error) {
	if id == uuid.Nil || at.IsZero() {
		return ExpectedPayment{}, PaymentMatch{}, ErrInvalid
	}
	if p.status != Planned && p.status != PartiallyMatched {
		return ExpectedPayment{}, PaymentMatch{}, ErrState
	}
	if p.version == math.MaxInt64 {
		return ExpectedPayment{}, PaymentMatch{}, ledger.ErrVersion
	}
	tx := fact.Data()
	if tx.ID == uuid.Nil || tx.Scope != p.data.Scope {
		return ExpectedPayment{}, PaymentMatch{}, ledger.ErrScope
	}
	if tx.Direction != p.data.Direction {
		return ExpectedPayment{}, PaymentMatch{}, ErrDirection
	}
	for _, err := range []error{amount.Positive(), factAllocated.Nonnegative(), tx.Amount.Positive()} {
		if err != nil {
			return ExpectedPayment{}, PaymentMatch{}, err
		}
	}
	if factAllocated.Minor() > tx.Amount.Minor() || amount.Minor() > tx.Amount.Minor()-factAllocated.Minor() ||
		amount.Minor() > p.remaining.Minor() {
		return ExpectedPayment{}, PaymentMatch{}, ErrAllocation
	}
	remaining, err := p.remaining.Sub(amount)
	if err != nil {
		return ExpectedPayment{}, PaymentMatch{}, err
	}
	p.remaining = remaining
	p.status = PartiallyMatched
	if remaining.Minor() == 0 {
		p.status = Matched
	}
	p.version++
	match := PaymentMatch{id: id, scope: p.data.Scope, paymentID: p.data.ID,
		transactionID: tx.ID, amount: amount, createdAt: at.UTC()}
	return p, match, nil
}
