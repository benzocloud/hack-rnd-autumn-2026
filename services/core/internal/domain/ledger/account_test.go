package ledger_test

import (
	"errors"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"testing"
)

func TestAccountSourceBindings(t *testing.T) {
	t.Parallel()
	scope := ledger.Scope{CompanyID: "company", AccountID: "account"}
	account, err := ledger.NewBankAccount(scope)
	if err != nil {
		t.Fatal(err)
	}
	if err := account.BindSupplemental(ledger.Namespace{System: "one_c", CompanyID: "C001"}); err != nil {
		t.Fatal(err)
	}
	if err := account.BindSupplemental(ledger.Namespace{System: "one_c", CompanyID: "C002"}); !errors.Is(err, ledger.ErrBinding) {
		t.Fatalf("different external company accepted: %v", err)
	}
	manual := ledger.Source{Kind: ledger.Manual}
	if err := account.BindFacts(manual); err != nil {
		t.Fatalf("supplemental binding chose factual authority: %v", err)
	}
	if err := account.BindFacts(importSource("dataset_v1", "C001", "TX1")); !errors.Is(err, ledger.ErrSourceConflict) {
		t.Fatalf("external history mixed with manual: %v", err)
	}
	if err := account.BindFacts(manual); err != nil {
		t.Fatalf("repeated authority rejected: %v", err)
	}
	if account.DataVersion() != 1 {
		t.Fatal("binding unexpectedly changed data version")
	}
	if err := account.AdvanceDataVersion(); err != nil || account.DataVersion() != 2 {
		t.Fatalf("revision: %v", err)
	}
	other, err := ledger.NewBankAccount(scope)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []ledger.Source{importSource("dataset_v1", "C001", "TX1"), importSource("dataset_v1", "C001", "TX2")} {
		if err := other.BindFacts(source); err != nil {
			t.Fatal(err)
		}
	}
	for _, source := range []ledger.Source{manual, importSource("dataset_v1", "C002", "TX3"), importSource("bank", "C001", "TX4")} {
		if err := other.BindFacts(source); !errors.Is(err, ledger.ErrSourceConflict) {
			t.Fatalf("source switch accepted: %v", err)
		}
	}
}
