package postgres

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

// WithTx binds the transaction and all callback operations to one deadline.
// database/sql rolls back when this context is cancelled, including during Commit.
func WithTx(ctx context.Context, db *sqlx.DB, timeout time.Duration, fn func(context.Context, *sqlx.Tx) error) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return storageError(err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(ctx, tx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return storageError(tx.Commit())
}
