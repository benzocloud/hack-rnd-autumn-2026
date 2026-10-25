//go:build integration

package app

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"

	appPostgres "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/app/postgres"
	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
	commandsSQL "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres/commands"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	service "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
)

func TestTransactionReplayAfterReconnect(t *testing.T) {
	application, cfg := testApplication(t)
	scope := newScope(t, application)
	cmd := transactionCommand(t, scope)
	first, err := application.Ledger.CreateTransaction(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := New(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	replay, err := reopened.Ledger.CreateTransaction(t.Context(), cmd)
	if err != nil || replay != first {
		t.Fatalf("reconnect replay: %+v %+v %v", first, replay, err)
	}
	if first.TransactionID.Version() != 7 || first.TransactionVersion != 1 || first.DataVersion != 2 {
		t.Fatal("incorrect result")
	}
	cmd.Record.Amount = rub(t, 101)
	_, err = application.Ledger.CreateTransaction(t.Context(), cmd)
	requireError(t, err, service.ErrIdempotencyConflict)
	assertRevision(t, application, scope, 2)
	assertCount(t, application, "SELECT count(*) FROM core_transactions WHERE company_id=$1", scope, 1)
}

func TestRollbackAndReuseCommandKey(t *testing.T) {
	application, _ := testApplication(t)
	scope := newScope(t, application)
	cmd := transactionCommand(t, scope)
	categoryID := uuid.New()
	cmd.Record.Category = ledger.Category{CompanyID: scope.CompanyID, ID: categoryID}
	_, err := application.Ledger.CreateTransaction(t.Context(), cmd)
	requireError(t, err, service.ErrConflict)
	var cause *pgconn.PgError
	if !errors.As(err, &cause) || cause.Code != "23503" {
		t.Fatal("service boundary lost SQLSTATE")
	}
	if err.Error() != service.ErrConflict.Error() {
		t.Fatal("service boundary exposed driver details")
	}

	assertRevision(t, application, scope, 1)
	assertCount(t, application, "SELECT count(*) FROM core_command_results WHERE company_id=$1", scope, 0)
	assertCount(t, application, "SELECT count(*) FROM core_transactions WHERE company_id=$1", scope, 0)
	var authority *string
	if err := application.db.GetContext(t.Context(), &authority, "SELECT factual_kind FROM core_accounts WHERE company_id=$1", scope.CompanyID); err != nil {
		t.Fatal(err)
	}
	if authority != nil {
		t.Fatal("failed transaction bound factual source")
	}
	if _, err := application.db.ExecContext(t.Context(), "INSERT INTO core_categories(company_id,id,name) VALUES($1,$2,'sales')", scope.CompanyID, categoryID); err != nil {
		t.Fatal(err)
	}
	if _, err := application.Ledger.CreateTransaction(t.Context(), cmd); err != nil {
		t.Fatal(err)
	}
	assertRevision(t, application, scope, 2)
}

func TestExternalTransactionReplayAndBinding(t *testing.T) {
	application, _ := testApplication(t)
	scope := newScope(t, application)
	cmd := transactionCommand(t, scope)
	cmd.ActorID, cmd.IdempotencyKey = "", ""
	cmd.Record.Source = ledger.Source{Kind: ledger.Imported, Namespace: ledger.Namespace{System: "bank", CompanyID: "C001"}, ExternalID: "OP1", ImportedAt: time.Now()}
	first, err := application.Ledger.CreateTransaction(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	cmd.Record.Source.ImportedAt = time.Now().Add(time.Minute)
	replay, err := application.Ledger.CreateTransaction(t.Context(), cmd)
	if err != nil || first != replay {
		t.Fatalf("external replay %+v %v", replay, err)
	}
	cmd.Record.Amount = rub(t, 101)
	_, err = application.Ledger.CreateTransaction(t.Context(), cmd)
	requireError(t, err, ledger.ErrExternalRecord)
	cmd.Record.Amount = rub(t, 100)
	cmd.Record.Source.ExternalID = "OP2"
	cmd.Record.Source.Namespace.CompanyID = "C002"
	cmd.ExpectedDataVersion = 2
	_, err = application.Ledger.CreateTransaction(t.Context(), cmd)
	requireError(t, err, ledger.ErrSourceConflict)
	manual := transactionCommand(t, scope)
	manual.ExpectedDataVersion = 2
	_, err = application.Ledger.CreateTransaction(t.Context(), manual)
	requireError(t, err, service.ErrSuspectedDuplicate)
	assertRevision(t, application, scope, 2)
	manual.Record.Purpose += " manual supplement"
	if _, err := application.Ledger.CreateTransaction(t.Context(), manual); err != nil {
		t.Fatal(err)
	}
	assertRevision(t, application, scope, 3)
}

func TestBalanceReplayAndAuthority(t *testing.T) {
	application, _ := testApplication(t)
	scope := newScope(t, application)
	cmd := service.BalanceCommand{Scope: scope, Date: "2026-10-23", Amount: rub(t, 500), Source: ledger.BalanceSource{Kind: ledger.ManualEOD}, Note: "counted", ExpectedDataVersion: 1, ActorID: "test-owner", IdempotencyKey: uuid.NewString()}
	first, err := application.Ledger.RecordBalance(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := application.Ledger.RecordBalance(t.Context(), cmd)
	if err != nil || first != replay {
		t.Fatalf("balance replay %+v %v", replay, err)
	}
	cmd.IdempotencyKey = uuid.NewString()
	replay, err = application.Ledger.RecordBalance(t.Context(), cmd)
	if err != nil || first != replay {
		t.Fatalf("same observation changed revision %+v %v", replay, err)
	}
	cmd.IdempotencyKey = uuid.NewString()
	cmd.Amount = rub(t, 501)
	_, err = application.Ledger.RecordBalance(t.Context(), cmd)
	requireError(t, err, ledger.ErrBalanceConflict)
	fact := transactionCommand(t, scope)
	fact.ExpectedDataVersion = 2
	fact.Record.Source = ledger.Source{Kind: ledger.Imported, Namespace: ledger.Namespace{System: "bank", CompanyID: "C001"}, ExternalID: "OP1", ImportedAt: time.Now()}
	if _, err := application.Ledger.CreateTransaction(t.Context(), fact); err != nil {
		t.Fatal("manual EOD chose factual authority:", err)
	}
	assertRevision(t, application, scope, 3)
	assertCount(t, application, "SELECT count(*) FROM core_balances WHERE company_id=$1", scope, 1)
}

func TestContextCancellationReleasesConnections(t *testing.T) {
	application, _ := testApplication(t)
	scope := newScope(t, application)
	// Hold the account so cancellation happens after command-key reservation.
	tx, err := application.db.BeginTxx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var version int64
	if err := tx.GetContext(t.Context(), &version, "SELECT data_version FROM core_accounts WHERE company_id=$1 AND id=$2 FOR UPDATE", scope.CompanyID, scope.AccountID); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	cmd := transactionCommand(t, scope)
	_, err = application.Ledger.CreateTransaction(ctx, cmd)
	requireError(t, err, context.DeadlineExceeded)
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := application.Ledger.CreateTransaction(t.Context(), cmd); err != nil {
		t.Fatal("canceled reservation survived:", err)
	}
	assertRevision(t, application, scope, 2)
	if application.db.Stats().InUse != 0 {
		t.Fatal("connection leaked")
	}
}

func TestIdempotencyScopeAndExplicitInput(t *testing.T) {
	application, _ := testApplication(t)
	scope := newScope(t, application)
	other := newScope(t, application)
	cmd := transactionCommand(t, scope)
	cmd.Record.ID = uuid.New()
	first, err := application.Ledger.CreateTransaction(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	cmd.Record.ID = uuid.New()
	_, err = application.Ledger.CreateTransaction(t.Context(), cmd)
	requireError(t, err, service.ErrIdempotencyConflict)
	cmd.Record.ID = uuid.Nil
	cmd.ActorID = "another-owner"
	cmd.ExpectedDataVersion = 2
	second, err := application.Ledger.CreateTransaction(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if second.TransactionID == first.TransactionID {
		t.Fatal("different actor shared a reservation")
	}
	cmd.Record.Scope = other
	cmd.ExpectedDataVersion = 1
	if _, err := application.Ledger.CreateTransaction(t.Context(), cmd); err != nil {
		t.Fatal("tenant scope collision:", err)
	}
	pay := paymentCommand(t, scope, 3)
	pay.IdempotencyKey = cmd.IdempotencyKey
	pay.ActorID = cmd.ActorID
	if _, err := application.Payments.CreatePayment(t.Context(), pay); err != nil {
		t.Fatal("operation scope collision:", err)
	}
	assertRevision(t, application, scope, 4)
}

func TestStatementEODFactualAuthority(t *testing.T) {
	application, _ := testApplication(t)
	for _, first := range []string{"statement", "manual", "other bank"} {
		t.Run(first, func(t *testing.T) {
			scope := newScope(t, application)
			balance := service.BalanceCommand{Scope: scope, Date: "2026-10-23", Amount: rub(t, 500), Source: ledger.BalanceSource{Kind: ledger.StatementEOD, Namespace: ledger.Namespace{System: "bank", CompanyID: "C001"}, RecordedAt: time.Now()}, ExpectedDataVersion: 1}
			fact := transactionCommand(t, scope)
			if first == "statement" {
				if _, err := application.Ledger.RecordBalance(t.Context(), balance); err != nil {
					t.Fatal(err)
				}
				fact.ExpectedDataVersion = 2
				_, err := application.Ledger.CreateTransaction(t.Context(), fact)
				if err != nil {
					t.Fatal("manual supplement rejected", err)
				}
				fact.ExpectedDataVersion = 3
				fact.IdempotencyKey = uuid.NewString()
				fact.Record.Source = ledger.Source{Kind: ledger.Imported, Namespace: ledger.Namespace{System: "other_bank", CompanyID: "C002"}, ExternalID: "OP1", ImportedAt: time.Now()}
				_, err = application.Ledger.CreateTransaction(t.Context(), fact)
				requireError(t, err, ledger.ErrSourceConflict)
				balance.Date = "2026-10-24"
				balance.ExpectedDataVersion = 3
				balance.Source.Namespace.System = "other_bank"
				_, err = application.Ledger.RecordBalance(t.Context(), balance)
				requireError(t, err, ledger.ErrSourceConflict)
				assertRevision(t, application, scope, 3)
				assertCount(t, application, "SELECT count(*) FROM core_transactions WHERE company_id=$1", scope, 1)
				assertCount(t, application, "SELECT count(*) FROM core_balances WHERE company_id=$1", scope, 1)
				// The original namespace accepts both new facts and subsequent observations.
				fact.Record.Source.Namespace = ledger.Namespace{System: "bank", CompanyID: "C001"}
				if _, err := application.Ledger.CreateTransaction(t.Context(), fact); err != nil {
					t.Fatal(err)
				}
				balance.Source.Namespace = fact.Record.Source.Namespace
				balance.ExpectedDataVersion = 4
				if _, err := application.Ledger.RecordBalance(t.Context(), balance); err != nil {
					t.Fatal(err)
				}
				assertRevision(t, application, scope, 5)
				return
			}
			if first == "other bank" {
				fact.Record.Source = ledger.Source{Kind: ledger.Imported, Namespace: ledger.Namespace{System: "other_bank", CompanyID: "C002"}, ExternalID: "OP1", ImportedAt: time.Now()}
			}
			if _, err := application.Ledger.CreateTransaction(t.Context(), fact); err != nil {
				t.Fatal(err)
			}
			balance.ExpectedDataVersion = 2
			_, err := application.Ledger.RecordBalance(t.Context(), balance)
			requireError(t, err, ledger.ErrSourceConflict)
			assertRevision(t, application, scope, 2)
			assertCount(t, application, "SELECT count(*) FROM core_balances WHERE company_id=$1", scope, 0)
		})
	}
}

func TestSQLFailureRollsBackWholeWrite(t *testing.T) {
	application, _ := testApplication(t)
	scope := newScope(t, application)
	digest := sha256.Sum256([]byte("fixture-body"))
	err := appPostgres.WithinTx(t.Context(), application.db, time.Second, func(ctx context.Context, tx *sqlx.Tx) error {
		if _, _, err := commandsSQL.New(tx).Reserve(ctx, scope, "test-owner", "sql-failure-fixture", "", uuid.NewString(), digest[:]); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE core_accounts SET data_version=2 WHERE company_id=$1", scope.CompanyID); err != nil {
			return storage.StorageError(err)
		}
		_, err := tx.ExecContext(ctx, "SELECT * FROM saldo_deliberately_missing_table")
		return storage.StorageError(err)
	})
	requireError(t, err, storage.ErrUnavailable)
	assertRevision(t, application, scope, 1)
	assertCount(t, application, "SELECT count(*) FROM core_command_results WHERE company_id=$1", scope, 0)
	if application.db.Stats().InUse != 0 {
		t.Fatal("SQL failure leaked a connection")
	}
}
