package ledger_test

import (
	"errors"
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
	result, err := ledger.New(storage.withinTx).RecordBalance(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if result.Date != cmd.Date || result.DataVersion != 2 {
		t.Fatalf("balance result changed: %+v", result)
	}
	if storage.actor != cmd.ActorID || storage.key != cmd.IdempotencyKey {
		t.Fatal("command metadata changed")
	}
}

func TestStatementBalanceOwnsFactualAuthority(t *testing.T) {
	t.Parallel()
	base := transactionCommand(t)
	cmd := ledger.BalanceCommand{Scope: base.Record.Scope, Date: "2026-10-23", Amount: base.Record.Amount,
		Source: domain.BalanceSource{Kind: domain.StatementEOD, Namespace: domain.Namespace{System: "bank", CompanyID: "C1"}}, ExpectedDataVersion: 1}
	storage := &captureStore{}
	service := ledger.New(storage.withinTx)
	first, err := service.RecordBalance(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	state := storage.account.State()
	if state.Factual != domain.Imported || state.Namespace != cmd.Source.Namespace {
		t.Fatal("statement did not bind factual authority")
	}
	replay, err := service.RecordBalance(t.Context(), cmd)
	if err != nil || replay != first || storage.saves != 1 || storage.inserts != 1 {
		t.Fatalf("balance replay changed storage: %v", err)
	}
	base.ExpectedDataVersion = 2
	if _, err := service.CreateTransaction(t.Context(), base); !errors.Is(err, domain.ErrSourceConflict) || storage.inserts != 1 {
		t.Fatalf("manual facts mixed with statement: %v", err)
	}
	cmd.Date = "2026-10-24"
	storage.balanceFound = false
	if _, err := service.RecordBalance(t.Context(), cmd); !errors.Is(err, ledger.ErrStaleVersion) || storage.inserts != 1 {
		t.Fatalf("stale balance accepted: %v", err)
	}
}
