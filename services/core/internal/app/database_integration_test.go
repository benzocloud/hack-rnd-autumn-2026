//go:build integration

package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	domainPayments "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
	ledgerService "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
	paymentService "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/payments"
)

func testApplication(t *testing.T) (*application, config.Config) {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Postgres.Host != "127.0.0.1" || cfg.Postgres.Database != "saldo_core_test" {
		t.Fatal("integration tests require the dedicated loopback saldo_core_test database")
	}
	application, err := New(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := application.Close(); err != nil {
			t.Error(err)
		}
	})
	var marker string
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := application.db.GetContext(ctx, &marker, "SELECT coalesce(current_setting('saldo.test_database',true),'')"); err != nil {
		t.Fatal(err)
	}
	if marker != "saldo-core-integration" {
		t.Fatal("refusing writes: dedicated database marker missing")
	}
	return application, cfg
}

func newScope(t *testing.T, application *application) ledger.Scope {
	t.Helper()
	scope := ledger.Scope{CompanyID: uuid.New(), AccountID: uuid.New()}
	ctx := t.Context()
	if _, err := application.db.ExecContext(ctx, "INSERT INTO core_companies(id) VALUES($1)", scope.CompanyID); err != nil {
		t.Fatal(err)
	}
	if _, err := application.db.ExecContext(ctx, "INSERT INTO core_accounts(id,company_id,currency,data_version) VALUES($1,$2,'RUB',1)", scope.AccountID, scope.CompanyID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
		defer cancel()
		for _, query := range []string{
			"DELETE FROM core_forecast_jobs WHERE company_id=$1",
			"DELETE FROM core_history_coverage WHERE company_id=$1",
			"DELETE FROM core_payment_matches WHERE company_id=$1",
			"DELETE FROM core_expected_payments WHERE company_id=$1",
			"DELETE FROM core_payment_reconciliation_groups WHERE company_id=$1",
			"DELETE FROM core_transactions WHERE company_id=$1",
			"DELETE FROM core_imports WHERE company_id=$1",
			"DELETE FROM core_financial_snapshots WHERE company_id=$1",
			"DELETE FROM core_command_results WHERE company_id=$1",
			"DELETE FROM core_balances WHERE company_id=$1",
			"DELETE FROM core_categories WHERE company_id=$1",
			"UPDATE core_accounts SET factual_kind=NULL,source_system=NULL,source_company_id=NULL WHERE company_id=$1",
			"DELETE FROM core_source_bindings WHERE company_id=$1",
			"DELETE FROM core_accounts WHERE company_id=$1",
			"DELETE FROM core_companies WHERE id=$1",
		} {
			if _, err := application.db.ExecContext(ctx, query, scope.CompanyID); err != nil {
				t.Error(err)
			}
		}
	})
	return scope
}

func rub(t *testing.T, minor int64) money.Money {
	t.Helper()
	amount, err := money.New(minor, money.RUB)
	if err != nil {
		t.Fatal(err)
	}
	return amount
}

func transactionCommand(t *testing.T, scope ledger.Scope) ledgerService.TransactionCommand {
	t.Helper()
	return ledgerService.TransactionCommand{Record: ledger.TransactionData{Scope: scope, Date: "2026-10-23", Direction: ledger.Inflow,
		Amount: rub(t, 100), Source: ledger.Source{Kind: ledger.Manual}, Purpose: "test receipt"},
		ExpectedDataVersion: 1, ActorID: "test-owner", IdempotencyKey: uuid.NewString()}
}

func paymentCommand(t *testing.T, scope ledger.Scope, version int64) paymentService.CreateCommand {
	t.Helper()
	return paymentService.CreateCommand{Record: domainPayments.PaymentData{Scope: scope, ScheduledDate: "2026-10-24", Direction: ledger.Inflow,
		Amount: rub(t, 100), Source: ledger.Source{Kind: ledger.Manual}, Description: "planned receipt"},
		ExpectedDataVersion: version, ActorID: "test-owner", IdempotencyKey: uuid.NewString()}
}

func matchCommand(t *testing.T, scope ledger.Scope, factID, paymentID uuid.UUID, version int64) paymentService.MatchCommand {
	t.Helper()
	return paymentService.MatchCommand{Scope: scope, PaymentID: paymentID, TransactionID: factID, Amount: rub(t, 60),
		ExpectedVersion: 1, ExpectedDataVersion: version, ActorID: "test-owner", IdempotencyKey: uuid.NewString()}
}

func assertRevision(t *testing.T, application *application, scope ledger.Scope, want int64) {
	t.Helper()
	var got int64
	if err := application.db.GetContext(t.Context(), &got, "SELECT data_version FROM core_accounts WHERE company_id=$1 AND id=$2", scope.CompanyID, scope.AccountID); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("data version=%d want=%d", got, want)
	}
}

func assertCount(t *testing.T, application *application, query string, scope ledger.Scope, want int) {
	t.Helper()
	var count int
	if err := application.db.GetContext(t.Context(), &count, query, scope.CompanyID); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("row count=%d want=%d", count, want)
	}
}

func requireError(t *testing.T, got, want error) {
	t.Helper()
	if !errors.Is(got, want) {
		t.Fatalf("error=%v want=%v", got, want)
	}
}

func TestRESTPaymentVersionsAndReplay(t *testing.T) {
	application, _ := testApplication(t)
	scope := newScope(t, application)
	fact, err := application.Ledger.CreateTransaction(t.Context(), transactionCommand(t, scope))
	if err != nil {
		t.Fatal(err)
	}
	create := paymentCommand(t, scope, 0)
	payment, err := application.Payments.CreatePayment(t.Context(), create)
	if err != nil {
		t.Fatal(err)
	}
	cmd := matchCommand(t, scope, fact.TransactionID, payment.PaymentID, 0)
	cmd.ExpectedTransactionVersion = fact.TransactionVersion
	first, err := application.Payments.MatchPayment(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := application.Payments.MatchPayment(t.Context(), cmd)
	if err != nil || replay != first {
		t.Fatalf("resource-version replay failed: %+v %v", replay, err)
	}
	assertRevision(t, application, scope, 4)
	cmd.ExpectedTransactionVersion++
	if _, err = application.Payments.MatchPayment(t.Context(), cmd); !errors.Is(err, paymentService.ErrIdempotencyConflict) {
		t.Fatalf("changed fact version under same key accepted: %v", err)
	}
	cmd.IdempotencyKey = uuid.NewString()
	cmd.ExpectedVersion = first.PaymentVersion
	if _, err = application.Payments.MatchPayment(t.Context(), cmd); !errors.Is(err, paymentService.ErrStaleVersion) {
		t.Fatalf("stale fact version accepted: %v", err)
	}
	assertRevision(t, application, scope, 4)
	var total int64
	if err = application.db.GetContext(t.Context(), &total, "SELECT coalesce(sum(amount_minor),0) FROM core_payment_matches WHERE company_id=$1 AND account_id=$2", scope.CompanyID, scope.AccountID); err != nil {
		t.Fatal(err)
	}
	if total != 60 {
		t.Fatalf("stale or replay request changed allocation: %d", total)
	}
}
