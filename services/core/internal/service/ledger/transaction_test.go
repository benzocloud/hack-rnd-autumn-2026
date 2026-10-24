package ledger_test

import (
	"testing"

	"github.com/google/uuid"

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
			result, err := ledger.New(storage).CreateTransaction(t.Context(), cmd)
			if err != nil {
				t.Fatal(err)
			}
			if tc.id == uuid.Nil && (result.TransactionID == uuid.Nil || result.TransactionID.Version() != 7) {
				t.Fatal("expected generated UUIDv7")
			}
			if tc.id != uuid.Nil && result.TransactionID != tc.id {
				t.Fatal("supplied transaction ID changed")
			}
			if result.TransactionVersion != 7 || result.DataVersion != 42 {
				t.Fatal("transaction result versions changed")
			}
			if storage.actor != cmd.ActorID || storage.key != cmd.IdempotencyKey {
				t.Fatal("command metadata changed")
			}
		})
	}
}
