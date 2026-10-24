//go:build integration

package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/jmoiron/sqlx"

	appPostgres "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/app/postgres"
	commandsSQL "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres/commands"
	ledgerSQL "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
)

func TestManualBalanceCommitFailureRollsBackReceiptAndAllowsRetry(t *testing.T) {
	application, cfg := testApplication(t)
	scope := newScope(t, application)
	inject := true
	service := ledger.New(func(ctx context.Context, work ledger.Work) error {
		err := appPostgres.WithinTx(ctx, application.db, cfg.Postgres.QueryTimeout, func(ctx context.Context, tx *sqlx.Tx) error {
			if err := work(ctx, ledgerSQL.New(tx), commandsSQL.New(tx)); err != nil {
				return err
			}
			if inject {
				_, err := tx.ExecContext(ctx, "UPDATE core_accounts SET factual_kind='imported',source_system='missing',source_company_id='missing' WHERE company_id=$1 AND id=$2", scope.CompanyID, scope.AccountID)
				return err
			}
			return nil
		})
		return ledgerError(err)
	})
	cmd := ledger.ManualBalanceCommand{Scope: scope, Date: "2026-10-23", ViewDate: "2026-10-24", Amount: rub(t, 0), Note: "counted", ExpectedDataVersion: 1, ActorID: "auth-user", IdempotencyKey: "commit-failure-key"}
	result, err := service.RecordManualBalance(t.Context(), cmd)
	if !errors.Is(err, ledger.ErrConflict) || !reflect.DeepEqual(result, ledger.RecordManualBalanceResult{}) {
		t.Fatal("failed commit returned success", result, err)
	}
	for _, query := range []string{"SELECT count(*) FROM core_balances WHERE company_id=$1", "SELECT count(*) FROM core_command_results WHERE company_id=$1"} {
		var count int
		if err := application.db.GetContext(t.Context(), &count, query, scope.CompanyID); err != nil || count != 0 {
			t.Fatal("partial result survived commit failure", count, err)
		}
	}
	assertRevision(t, application, scope, 1)
	inject = false
	result, err = service.RecordManualBalance(t.Context(), cmd)
	if err != nil || result.Account.DataVersion != 2 || result.Balance == nil || result.Balance.Amount().Minor() != 0 {
		t.Fatal("rollback poisoned retry", result, err)
	}
}
