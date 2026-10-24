//go:build integration

package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/jmoiron/sqlx"

	appPostgres "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/app/postgres"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	commandsSQL "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres/commands"
	ledgerSQL "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
)

func TestManualTransactionCommitFailureRollsBackFactAndReceipt(t *testing.T) {
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
	cmd := ledger.ManualTransactionCommand{Record: domain.TransactionData{Scope: scope, Date: "2026-10-23", Direction: domain.Inflow, Amount: rub(t, 100), Source: domain.Source{Kind: domain.Manual}}, ExpectedDataVersion: 1, ActorID: "auth-user", IdempotencyKey: "manual-commit-failure", ViewDate: "2026-10-24"}
	result, err := service.CreateManualTransaction(t.Context(), cmd)
	if !errors.Is(err, ledger.ErrConflict) || !reflect.DeepEqual(result, ledger.CreateManualTransactionResult{}) {
		t.Fatal(result, err)
	}
	assertRevision(t, application, scope, 1)
	assertCount(t, application, "SELECT count(*) FROM core_transactions WHERE company_id=$1", scope, 0)
	assertCount(t, application, "SELECT count(*) FROM core_command_results WHERE company_id=$1", scope, 0)
	inject = false
	result, err = service.CreateManualTransaction(t.Context(), cmd)
	if err != nil || result.CreatedAt.IsZero() || result.DataVersion != 2 {
		t.Fatal(result, err)
	}
}
