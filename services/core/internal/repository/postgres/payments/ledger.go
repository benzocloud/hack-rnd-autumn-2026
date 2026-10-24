package payments

import (
	"context"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	ledgerSQL "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres/ledger"
)

func (r *repository) LoadAccount(ctx context.Context, scope ledger.Scope) (*ledger.BankAccount, error) {
	return ledgerSQL.LoadAccount(ctx, r.tx, scope)
}

func (r *repository) SaveAccount(ctx context.Context, account *ledger.BankAccount) error {
	return ledgerSQL.SaveAccount(ctx, r.tx, account)
}

func (r *repository) FindTransaction(ctx context.Context, scope ledger.Scope, id uuid.UUID, source ledger.Source) (ledger.Transaction, int64, bool, error) {
	return ledgerSQL.New(r.tx).FindTransaction(ctx, scope, id, source)
}
