package ledger_test

import (
	"testing"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
)

func TestRecordBalanceResult(t *testing.T) {
	t.Parallel()
	base := transactionCommand(t)
	cmd := ledger.BalanceCommand{Scope: base.Record.Scope, Date: "2026-10-23", Amount: base.Record.Amount,
		Source: domain.BalanceSource{Kind: domain.ManualEOD}, Note: "initial balance", ExpectedDataVersion: 1,
		ActorID: base.ActorID, IdempotencyKey: base.IdempotencyKey}
	storage := &captureStore{}
	result, err := ledger.New(storage).RecordBalance(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if result.Date != cmd.Date || result.DataVersion != 42 {
		t.Fatalf("balance result changed: %+v", result)
	}
	if storage.actor != cmd.ActorID || storage.key != cmd.IdempotencyKey {
		t.Fatal("command metadata changed")
	}
}
