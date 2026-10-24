package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"

	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
)

// WithinTx owns one transaction. Work must finish using tx before returning.
func WithinTx(ctx context.Context, db *sqlx.DB, timeout time.Duration, work func(context.Context, *sqlx.Tx) error) (err error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	tx, err := db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return storage.StorageError(err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, storage.WithCause(storage.ErrUnavailable, rollbackErr))
		}
	}()
	if err := work(ctx, tx); err != nil {
		return err
	}
	return storage.StorageError(tx.Commit())
}
