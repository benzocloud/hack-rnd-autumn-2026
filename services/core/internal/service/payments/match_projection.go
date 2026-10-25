package payments

import (
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
)

type matchProjection struct {
	ID, PaymentID, TransactionID uuid.UUID
	Scope                        ledger.Scope
	Amount                       int64
	CreatedAt                    time.Time
}

func newMatchProjection(m domain.PaymentMatch) *matchProjection {
	return &matchProjection{m.ID(), m.PaymentID(), m.TransactionID(), m.Scope(), m.Amount().Minor(), m.CreatedAt()}
}
func (p matchProjection) match() (domain.PaymentMatch, error) {
	amount, err := money.New(p.Amount, money.RUB)
	if err != nil {
		return domain.PaymentMatch{}, ErrUnavailable
	}
	m, err := domain.RestorePaymentMatch(p.ID, p.PaymentID, p.TransactionID, p.Scope, amount, p.CreatedAt)
	if err != nil {
		return domain.PaymentMatch{}, ErrUnavailable
	}
	return m, nil
}
