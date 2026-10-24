package forecasting

import (
	"context"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

type store interface {
	ReadDataVersion(context.Context, ledger.Scope) (int64, error)
	ReadOpeningBalance(context.Context, ledger.Scope, ledger.Date) (domain.OpeningBalance, bool, error)
	ReadFacts(context.Context, ledger.Scope, ledger.Date) ([]domain.Fact, error)
	ReadCalendar(context.Context, ledger.Scope) (domain.Calendar, error)
	InsertSnapshot(context.Context, domain.FinancialSnapshot) error
	ReadSnapshot(context.Context, ledger.Scope, uuid.UUID) (domain.FinancialSnapshot, error)
}

// Work borrows stores in one REPEATABLE READ transaction owned by app.
type Work func(context.Context, store) error

type service struct {
	withinTx func(context.Context, Work) error
}

func New(withinTx func(context.Context, Work) error) *service {
	return &service{withinTx: withinTx}
}
