package ledger_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
)

func TestCreateTransactionResult(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		id   uuid.UUID
	}{
		{name: "generated"}, {name: "supplied", id: uuid.MustParse("987fbc97-4bed-4078-af07-9141ba07c9f3")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := transactionCommand(t)
			cmd.Record.ID = tc.id
			storage := &captureStore{}
			result, err := ledger.New(storage.withinTx).CreateTransaction(t.Context(), cmd)
			if err != nil {
				t.Fatal(err)
			}
			if tc.id == uuid.Nil && (result.TransactionID == uuid.Nil || result.TransactionID.Version() != 7) {
				t.Fatal("expected generated UUIDv7")
			}
			if tc.id != uuid.Nil && result.TransactionID != tc.id {
				t.Fatal("supplied transaction ID changed")
			}
			if result.TransactionVersion != 1 || result.DataVersion != 2 {
				t.Fatal("transaction result versions changed")
			}
			if storage.actor != cmd.ActorID || storage.key != cmd.IdempotencyKey {
				t.Fatal("command metadata changed")
			}
		})
	}
}

func TestCreateTransactionControlsVersionsAndSource(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		prepare func(*testing.T, *captureStore, *ledger.TransactionCommand)
		want    error
	}{
		{name: "stale version", prepare: func(_ *testing.T, _ *captureStore, cmd *ledger.TransactionCommand) { cmd.ExpectedDataVersion = 2 }, want: ledger.ErrStaleVersion},
		{name: "source conflict", prepare: func(t *testing.T, storage *captureStore, cmd *ledger.TransactionCommand) {
			t.Helper()
			if err := storage.prepare(cmd.Record.Scope); err != nil {
				t.Fatal(err)
			}
			if err := storage.account.BindStatementFacts(domain.Namespace{System: "bank", CompanyID: "C1"}); err != nil {
				t.Fatal(err)
			}
		}, want: domain.ErrSourceConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := transactionCommand(t)
			storage := &captureStore{}
			tc.prepare(t, storage, &cmd)
			result, err := ledger.New(storage.withinTx).CreateTransaction(t.Context(), cmd)
			if !errors.Is(err, tc.want) || result != (ledger.CreateTransactionResult{}) || storage.saves != 0 || storage.inserts != 0 {
				t.Fatalf("rejected command changed storage: result=%+v, err=%v, saves=%d, inserts=%d", result, err, storage.saves, storage.inserts)
			}
		})
	}
}

func TestImportedTransactionReplayDoesNotWrite(t *testing.T) {
	t.Parallel()
	cmd := transactionCommand(t)
	cmd.Record.Source = domain.Source{Kind: domain.Imported, Namespace: domain.Namespace{System: "bank", CompanyID: "C1"}, ExternalID: "OP1", ImportedAt: time.Now()}
	storage := &captureStore{}
	service := ledger.New(storage.withinTx)
	first, err := service.CreateTransaction(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	// Replay must precede optimistic version checks and return the original IDs/revision.
	replay, err := service.CreateTransaction(t.Context(), cmd)
	if err != nil || replay != first || storage.saves != 1 || storage.inserts != 1 || storage.account.DataVersion() != 2 {
		t.Fatalf("replay changed state: result=%+v, err=%v", replay, err)
	}
	changed, err := money.New(101, money.RUB)
	if err != nil {
		t.Fatal(err)
	}
	cmd.Record.Amount = changed
	if _, err := service.CreateTransaction(t.Context(), cmd); !errors.Is(err, domain.ErrExternalRecord) || storage.inserts != 1 {
		t.Fatalf("changed external identity accepted: %v", err)
	}
}
