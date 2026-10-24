package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"

	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
)

// WithinTx owns one transaction. Work must finish using tx before returning.
func WithinTx(ctx context.Context, db *sqlx.DB, timeout time.Duration, work func(context.Context, *sqlx.Tx) error) error {
	return withinTx(ctx, db, timeout, sql.TxOptions{Isolation: sql.LevelReadCommitted}, work)
}

// WithinRepeatableRead retries serialization failures in a fresh MVCC view, up
// to three attempts within one deadline. Work must be replayable and perform no
// external side effects. All reads and writes of an attempt share one transaction.
func WithinRepeatableRead(ctx context.Context, db *sqlx.DB, timeout time.Duration, work func(context.Context, *sqlx.Tx) error) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = withinTx(ctx, db, timeout, sql.TxOptions{Isolation: sql.LevelRepeatableRead}, work)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return storage.StorageError(ctx.Err())
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "40001" {
			return err
		}
	}
	return err
}

func withinTx(ctx context.Context, db *sqlx.DB, timeout time.Duration, options sql.TxOptions, work func(context.Context, *sqlx.Tx) error) (err error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	tx, err := db.BeginTxx(ctx, &options)
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
