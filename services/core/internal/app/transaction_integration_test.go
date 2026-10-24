//go:build integration

package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	appPostgres "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/app/postgres"
	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
)

func TestWithinTxLifecycle(t *testing.T) {
	application, _ := testApplication(t)
	for _, mode := range []string{"commit", "error", "cancel", "panic", "commit failure"} {
		t.Run(mode, func(t *testing.T) {
			scope := newScope(t, application)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			want := errors.New("callback failed")
			var err error
			var recovered any
			func() {
				defer func() { recovered = recover() }()
				err = appPostgres.WithinTx(ctx, application.db, time.Second, func(ctx context.Context, tx *sqlx.Tx) error {
					if _, err := tx.ExecContext(ctx, "UPDATE core_accounts SET data_version=2 WHERE company_id=$1", scope.CompanyID); err != nil {
						return storage.StorageError(err)
					}
					switch mode {
					case "error":
						return want
					case "cancel":
						cancel()
						return ctx.Err()
					case "panic":
						panic(want)
					case "commit failure":
						return tx.Rollback()
					}
					return nil
				})
			}()
			if mode == "commit" {
				if err != nil {
					t.Fatal(err)
				}
				assertRevision(t, application, scope, 2)
			} else {
				assertRevision(t, application, scope, 1)
				switch mode {
				case "error":
					requireError(t, err, want)
				case "cancel":
					requireError(t, err, context.Canceled)
				case "panic":
					got, ok := recovered.(error)
					if !ok || !errors.Is(got, want) {
						t.Fatalf("panic lost: %v", recovered)
					}
				case "commit failure":
					requireError(t, err, storage.ErrUnavailable)
				}
			}
			if application.db.Stats().InUse != 0 {
				t.Fatal("transaction leaked a connection")
			}
		})
	}
}
