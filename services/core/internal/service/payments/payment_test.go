package payments_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/payments"
)

func TestCreatePaymentResult(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		id   uuid.UUID
	}{
		{name: "generated"}, {name: "supplied", id: uuid.MustParse("987fbc97-4bed-4078-af07-9141ba07c9f3")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := paymentCommand(t)
			cmd.Record.ID = tc.id
			storage := &captureStore{}
			result, err := payments.New(storage.withinTx).CreatePayment(t.Context(), cmd)
			if err != nil {
				t.Fatal(err)
			}
			if tc.id == uuid.Nil && (result.PaymentID == uuid.Nil || result.PaymentID.Version() != 7) {
				t.Fatal("expected generated UUIDv7")
			}
			if tc.id != uuid.Nil && result.PaymentID != tc.id {
				t.Fatal("supplied payment ID changed")
			}
			if result.PaymentVersion != 1 || result.DataVersion != 2 {
				t.Fatal("payment result versions changed")
			}
			if storage.actor != cmd.ActorID || storage.key != cmd.IdempotencyKey {
				t.Fatal("command metadata changed")
			}
		})
	}
}

func TestImportedPaymentReplayAndBinding(t *testing.T) {
	t.Parallel()
	cmd := paymentCommand(t)
	cmd.Record.Source = ledger.Source{Kind: ledger.Imported, Namespace: ledger.Namespace{System: "planning", CompanyID: "C1"}, ExternalID: "PAY1", ImportedAt: time.Now()}
	storage := &captureStore{}
	service := payments.New(storage.withinTx)
	first, err := service.CreatePayment(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := service.CreatePayment(t.Context(), cmd)
	if err != nil || replay != first || storage.saves != 1 || storage.inserts != 1 {
		t.Fatalf("external replay changed storage: result=%+v, err=%v", replay, err)
	}
	if storage.account.State().Factual != "" {
		t.Fatal("expected payment selected factual authority")
	}
	cmd.Record.Source.ExternalID = "PAY2"
	cmd.Record.Source.Namespace.CompanyID = "other-company"
	cmd.ExpectedDataVersion = 2
	storage.paymentFound = false
	if _, err := service.CreatePayment(t.Context(), cmd); !errors.Is(err, ledger.ErrBinding) || storage.inserts != 1 {
		t.Fatalf("conflicting external company accepted: %v", err)
	}
	cmd.ExpectedDataVersion = 3
	if _, err := service.CreatePayment(t.Context(), cmd); !errors.Is(err, payments.ErrStaleVersion) || storage.inserts != 1 {
		t.Fatalf("stale version accepted: %v", err)
	}
}
