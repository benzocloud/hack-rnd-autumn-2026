package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"

	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
)

func TestWithinTxErrors(t *testing.T) {
	t.Parallel()
	callbackErr := errors.New("application conflict")
	cause := &pgconn.PgError{Code: "08006", Message: "private connection detail"}
	for _, tc := range []struct {
		name                          string
		begin, commit, rollback, work error
		want                          error
		calls                         int
	}{
		{name: "begin", begin: cause, want: storage.ErrUnavailable},
		{name: "callback", work: callbackErr, want: callbackErr, calls: 1},
		{name: "commit", commit: cause, want: storage.ErrUnavailable, calls: 1},
		{name: "callback and rollback", work: callbackErr, rollback: cause, want: callbackErr, calls: 1},
		{name: "cancellation", work: context.Canceled, want: context.Canceled, calls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := &transactionConnection{beginErr: tc.begin, commitErr: tc.commit, rollbackErr: tc.rollback}
			db := sqlx.NewDb(sql.OpenDB(transactionConnector{conn: conn}), "postgres")
			defer db.Close()
			calls := 0
			err := WithinTx(t.Context(), db, time.Second, func(ctx context.Context, _ *sqlx.Tx) error {
				calls++
				if _, ok := ctx.Deadline(); !ok {
					t.Fatal("transaction has no deadline")
				}
				return tc.work
			})
			if !errors.Is(err, tc.want) || calls != tc.calls {
				t.Fatalf("lifecycle error/calls: %v %d", err, calls)
			}
			if tc.begin != nil || tc.commit != nil || tc.rollback != nil {
				var got *pgconn.PgError
				if !errors.As(err, &got) || got != cause || strings.Contains(err.Error(), cause.Message) {
					t.Fatalf("cause lost or exposed: %v", err)
				}
			}
			if conn.isolation != driver.IsolationLevel(sql.LevelReadCommitted) {
				t.Fatal("isolation changed")
			}
		})
	}
}

type transactionConnector struct{ conn *transactionConnection }

func (c transactionConnector) Connect(context.Context) (driver.Conn, error) { return c.conn, nil }
func (transactionConnector) Driver() driver.Driver                          { return transactionDriver{} }

type transactionDriver struct{}

func (transactionDriver) Open(string) (driver.Conn, error) { return nil, errors.New("use connector") }

type transactionConnection struct {
	beginErr, commitErr, rollbackErr error
	isolation                        driver.IsolationLevel
}

func (*transactionConnection) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("queries unsupported")
}
func (*transactionConnection) Close() error                { return nil }
func (c *transactionConnection) Begin() (driver.Tx, error) { return c, c.beginErr }
func (c *transactionConnection) BeginTx(_ context.Context, opts driver.TxOptions) (driver.Tx, error) {
	c.isolation = opts.Isolation
	return c, c.beginErr
}
func (c *transactionConnection) Commit() error   { return c.commitErr }
func (c *transactionConnection) Rollback() error { return c.rollbackErr }
