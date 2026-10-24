package payments_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/payments"
)

func TestMatchPaymentResult(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		id   uuid.UUID
	}{
		{name: "generated"}, {name: "supplied", id: uuid.MustParse("987fbc97-4bed-4078-af07-9141ba07c9f3")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := matchCommand(t)
			cmd.ID = tc.id
			storage := matchStore(t, cmd)
			result, err := payments.New(storage.withinTx).MatchPayment(t.Context(), cmd)
			if err != nil {
				t.Fatal(err)
			}
			if tc.id == uuid.Nil && (result.MatchID == uuid.Nil || result.MatchID.Version() != 7) {
				t.Fatal("expected generated UUIDv7")
			}
			if tc.id != uuid.Nil && result.MatchID != tc.id {
				t.Fatal("supplied match ID changed")
			}
			if result.PaymentVersion != 2 || result.DataVersion != 2 {
				t.Fatal("payment/data versions changed")
			}
			if storage.actor != cmd.ActorID || storage.key != cmd.IdempotencyKey {
				t.Fatal("command metadata changed")
			}
		})
	}
}

func TestMatchPaymentValidation(t *testing.T) {
	t.Parallel()
	zero, err := money.New(0, money.RUB)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		edit func(*payments.MatchCommand)
		want error
	}{
		{"empty identity", func(cmd *payments.MatchCommand) { cmd.ActorID, cmd.IdempotencyKey = "", "" }, payments.ErrInvalid},
		{"actor only", func(cmd *payments.MatchCommand) { cmd.IdempotencyKey = "" }, payments.ErrInvalid},
		{"key only", func(cmd *payments.MatchCommand) { cmd.ActorID = "" }, payments.ErrInvalid},
		{"identity before scope", func(cmd *payments.MatchCommand) { cmd.ActorID = ""; cmd.Scope = ledger.Scope{} }, payments.ErrInvalid},
		{"scope before amount", func(cmd *payments.MatchCommand) { cmd.Scope = ledger.Scope{}; cmd.Amount = zero }, ledger.ErrScope},
		{"amount before version", func(cmd *payments.MatchCommand) { cmd.Amount = zero; cmd.ExpectedVersion = 0 }, money.ErrAmount},
		{"resource version", func(cmd *payments.MatchCommand) { cmd.ExpectedVersion = 0 }, payments.ErrInvalid},
		{"data version", func(cmd *payments.MatchCommand) { cmd.ExpectedDataVersion = 0 }, payments.ErrInvalid},
		{"payment id", func(cmd *payments.MatchCommand) { cmd.PaymentID = uuid.Nil }, payments.ErrInvalid},
		{"transaction id", func(cmd *payments.MatchCommand) { cmd.TransactionID = uuid.Nil }, payments.ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := matchCommand(t)
			tc.edit(&cmd)
			storage := &captureStore{}
			result, err := payments.New(storage.withinTx).MatchPayment(t.Context(), cmd)
			if !errors.Is(err, tc.want) || storage.calls != 0 || result != (payments.MatchPaymentResult{}) {
				t.Fatalf("invalid match reached storage: calls=%d, err=%v", storage.calls, err)
			}
		})
	}
}

func TestMatchPaymentOrchestratesGuards(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		prepare func(*testing.T, *captureStore, *payments.MatchCommand)
		want    error
	}{
		{name: "stale data", prepare: func(_ *testing.T, _ *captureStore, cmd *payments.MatchCommand) { cmd.ExpectedDataVersion = 2 }, want: payments.ErrStaleVersion},
		{name: "stale payment", prepare: func(_ *testing.T, _ *captureStore, cmd *payments.MatchCommand) { cmd.ExpectedVersion = 2 }, want: payments.ErrStaleVersion},
		{name: "missing payment", prepare: func(_ *testing.T, storage *captureStore, _ *payments.MatchCommand) { storage.paymentFound = false }, want: payments.ErrNotFound},
		{name: "missing transaction", prepare: func(_ *testing.T, storage *captureStore, _ *payments.MatchCommand) { storage.factFound = false }, want: payments.ErrNotFound},
		{name: "fact already allocated", prepare: func(_ *testing.T, storage *captureStore, cmd *payments.MatchCommand) { storage.allocated = cmd.Amount }, want: domain.ErrAllocation},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := matchCommand(t)
			storage := matchStore(t, cmd)
			tc.prepare(t, storage, &cmd)
			result, err := payments.New(storage.withinTx).MatchPayment(t.Context(), cmd)
			if !errors.Is(err, tc.want) || result != (payments.MatchPaymentResult{}) || storage.saves != 0 || storage.inserts != 0 {
				t.Fatalf("rejected allocation wrote state: result=%+v, err=%v, saves=%d, inserts=%d", result, err, storage.saves, storage.inserts)
			}
		})
	}
}
