package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
)

func TestRepeatableReadRetriesOnlySerializationFailures(t *testing.T) {
	conflict := &pgconn.PgError{Code: "40001", Message: "private diagnostics"}
	other := errors.New("application failure")
	for _, tc := range []struct {
		name                  string
		failures              int
		failure, commit, want error
		calls                 int
	}{
		{"retry succeeds", 1, conflict, nil, nil, 2},
		{"retry bounded", 3, conflict, nil, conflict, 3},
		{"commit conflict bounded", 0, nil, conflict, conflict, 3},
		{"application failure", 3, other, nil, other, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := &transactionConnection{commitErr: tc.commit}
			db := sqlx.NewDb(sql.OpenDB(transactionConnector{conn: conn}), "postgres")
			defer db.Close()
			calls := 0
			err := WithinRepeatableRead(t.Context(), db, time.Second, func(ctx context.Context, _ *sqlx.Tx) error {
				calls++
				if _, ok := ctx.Deadline(); !ok {
					t.Fatal("missing deadline")
				}
				if calls <= tc.failures {
					return tc.failure
				}
				return nil
			})
			if !errors.Is(err, tc.want) || calls != tc.calls || conn.isolation != driver.IsolationLevel(sql.LevelRepeatableRead) {
				t.Fatalf("error=%v calls=%d isolation=%v", err, calls, conn.isolation)
			}
		})
	}
}
