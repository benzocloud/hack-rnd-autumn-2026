//go:build integration

package app

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
	service "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/payments"
)

func TestMatchAtomicReplayAndAllocation(t *testing.T) {
	application, cfg := testApplication(t)
	scope := newScope(t, application)
	fact, err := application.Ledger.CreateTransaction(t.Context(), transactionCommand(t, scope))
	if err != nil {
		t.Fatal(err)
	}
	payment, err := application.Payments.CreatePayment(t.Context(), paymentCommand(t, scope, 2))
	if err != nil {
		t.Fatal(err)
	}
	cmd := matchCommand(t, scope, fact.TransactionID, payment.PaymentID, 3)
	first, err := application.Payments.MatchPayment(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := New(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	replay, err := reopened.Payments.MatchPayment(t.Context(), cmd)
	if err != nil || first != replay {
		t.Fatalf("match replay %+v %v", replay, err)
	}
	if first.MatchID.Version() != 7 || first.PaymentVersion != 2 || first.DataVersion != 4 {
		t.Fatal("incorrect match result")
	}
	cmd.Amount = rub(t, 61)
	_, err = application.Payments.MatchPayment(t.Context(), cmd)
	requireError(t, err, service.ErrIdempotencyConflict)
	cmd.IdempotencyKey = uuid.NewString()
	cmd.ExpectedVersion = 2
	cmd.ExpectedDataVersion = 4
	_, err = application.Payments.MatchPayment(t.Context(), cmd)
	requireError(t, err, domain.ErrAllocation)
	assertRevision(t, application, scope, 4)
	// A failed allocation rolls back both command reservation and payment update.
	cmd.Amount = rub(t, 40)
	final, err := application.Payments.MatchPayment(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if final.PaymentVersion != 3 || final.DataVersion != 5 {
		t.Fatal("incorrect final versions")
	}
	var remaining int64
	var status string
	if err := application.db.QueryRowxContext(t.Context(), "SELECT remaining_minor,status FROM core_expected_payments WHERE company_id=$1 AND id=$2", scope.CompanyID, payment.PaymentID).Scan(&remaining, &status); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 || status != "matched" {
		t.Fatal("match did not update remaining/status atomically")
	}
	assertCount(t, application, "SELECT count(*) FROM core_payment_matches WHERE company_id=$1", scope, 2)
	assertCount(t, application, "SELECT count(*) FROM core_transactions WHERE company_id=$1", scope, 1)
}

func TestPaymentImportReplayAndSupplementalBinding(t *testing.T) {
	application, _ := testApplication(t)
	scope := newScope(t, application)
	cmd := paymentCommand(t, scope, 1)
	cmd.ActorID, cmd.IdempotencyKey = "", ""
	cmd.Record.Source = ledger.Source{Kind: ledger.Imported, Namespace: ledger.Namespace{System: "one_c", CompanyID: "C001"}, ExternalID: "PAY1", ImportedAt: time.Now()}
	first, err := application.Payments.CreatePayment(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	cmd.Record.Source.ImportedAt = time.Now().Add(time.Minute)
	replay, err := application.Payments.CreatePayment(t.Context(), cmd)
	if err != nil || first != replay {
		t.Fatalf("payment external replay %+v %v", replay, err)
	}
	cmd.Record.Amount = rub(t, 101)
	_, err = application.Payments.CreatePayment(t.Context(), cmd)
	requireError(t, err, ledger.ErrExternalRecord)
	cmd.Record.Amount = rub(t, 100)
	cmd.Record.Source.ExternalID = "PAY2"
	cmd.Record.Source.Namespace.CompanyID = "C002"
	cmd.ExpectedDataVersion = 2
	_, err = application.Payments.CreatePayment(t.Context(), cmd)
	requireError(t, err, ledger.ErrBinding)
	fact := transactionCommand(t, scope)
	fact.ExpectedDataVersion = 2
	if _, err := application.Ledger.CreateTransaction(t.Context(), fact); err != nil {
		t.Fatal("planned payment chose factual authority:", err)
	}
	assertRevision(t, application, scope, 3)
}

func TestPaymentManualReplay(t *testing.T) {
	application, _ := testApplication(t)
	scope := newScope(t, application)
	cmd := paymentCommand(t, scope, 1)
	first, err := application.Payments.CreatePayment(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	second, err := application.Payments.CreatePayment(t.Context(), cmd)
	if err != nil || first != second {
		t.Fatalf("generated payment defaults changed digest: %+v %v", second, err)
	}
	cmd.Record.Description = "different"
	_, err = application.Payments.CreatePayment(t.Context(), cmd)
	requireError(t, err, service.ErrIdempotencyConflict)
	assertCount(t, application, "SELECT count(*) FROM core_expected_payments WHERE company_id=$1", scope, 1)
}

func TestCrossTenantMatchAndStalePayment(t *testing.T) {
	application, _ := testApplication(t)
	scope := newScope(t, application)
	other := newScope(t, application)
	fact, err := application.Ledger.CreateTransaction(t.Context(), transactionCommand(t, other))
	if err != nil {
		t.Fatal(err)
	}
	payment, err := application.Payments.CreatePayment(t.Context(), paymentCommand(t, scope, 1))
	if err != nil {
		t.Fatal(err)
	}
	cmd := matchCommand(t, scope, fact.TransactionID, payment.PaymentID, 2)
	cmd.ExpectedVersion = 2
	_, err = application.Payments.MatchPayment(t.Context(), cmd)
	requireError(t, err, service.ErrStaleVersion)
	cmd.ExpectedVersion = 1
	_, err = application.Payments.MatchPayment(t.Context(), cmd)
	requireError(t, err, service.ErrNotFound)
	assertRevision(t, application, scope, 2)
	assertCount(t, application, "SELECT count(*) FROM core_payment_matches WHERE company_id=$1", scope, 0)
}

func TestPaymentExternalReplayAfterMatch(t *testing.T) {
	application, _ := testApplication(t)
	scope := newScope(t, application)
	cmd := paymentCommand(t, scope, 1)
	cmd.ActorID = ""
	cmd.IdempotencyKey = ""
	cmd.Record.Source = ledger.Source{Kind: ledger.Imported, Namespace: ledger.Namespace{System: "one_c", CompanyID: "C001"}, ExternalID: "PAY1", ImportedAt: time.Now()}
	payment, err := application.Payments.CreatePayment(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	factCmd := transactionCommand(t, scope)
	factCmd.ExpectedDataVersion = 2
	fact, err := application.Ledger.CreateTransaction(t.Context(), factCmd)
	if err != nil {
		t.Fatal(err)
	}
	match := matchCommand(t, scope, fact.TransactionID, payment.PaymentID, 3)
	if _, err := application.Payments.MatchPayment(t.Context(), match); err != nil {
		t.Fatal(err)
	}
	replay, err := application.Payments.CreatePayment(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if replay.PaymentID != payment.PaymentID || replay.PaymentVersion != 2 || replay.DataVersion != 2 {
		t.Fatalf("external replay reset state: %+v", replay)
	}
	var remaining int64
	if err := application.db.GetContext(t.Context(), &remaining, "SELECT remaining_minor FROM core_expected_payments WHERE company_id=$1 AND id=$2", scope.CompanyID, payment.PaymentID); err != nil {
		t.Fatal(err)
	}
	if remaining != 40 {
		t.Fatal("reimport reset allocation")
	}
	assertRevision(t, application, scope, 4)
}
