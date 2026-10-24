package payments

import (
	"context"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

type store interface {
	paymentStore
	matchStore
}
type commandStore interface {
	Reserve(context.Context, ledger.Scope, string, string, string, string, []byte) ([]byte, bool, error)
	Complete(context.Context, ledger.Scope, string, string, string, string, []byte) error
}

// Work runs synchronously with all stores bound to one transaction.
// The provider owns commit/rollback; stores must not escape the callback.
type Work func(context.Context, store, commandStore) error
