package ledger

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

func compatibilityCases() map[string][2]any {
	id := uuid.MustParse("019a0c65-9800-7000-8000-000000000003")
	scope := domain.Scope{CompanyID: uuid.MustParse("019a0c65-9800-7000-8000-000000000001"), AccountID: uuid.MustParse("019a0c65-9800-7000-8000-000000000002")}
	amount, _ := money.New(123456, money.RUB)
	at := time.Date(2026, 10, 23, 13, 14, 15, 123000000, time.FixedZone("offset", 3*60*60))
	source := domain.Source{Kind: domain.Imported, Namespace: domain.Namespace{System: "bank", CompanyID: "C001"}, ExternalID: "OP1", ImportedAt: at}
	return map[string][2]any{
		"transaction_defaults": {transactionRequest(TransactionCommand{}), transactionResult{ID: id, Version: 1, DataVersion: 42}},
		"transaction_explicit": {transactionRequest(TransactionCommand{Record: domain.TransactionData{ID: id, Scope: scope, Date: "2026-10-23", Direction: domain.Inflow, Amount: amount, Source: source, Category: domain.Category{ID: id, CompanyID: scope.CompanyID}, CounterpartyINN: "1234567890", CounterpartyName: "Example", Purpose: "Payment"}, ExpectedDataVersion: 42}), transactionResult{ID: id, Version: 1, DataVersion: 43}},
		"balance_defaults":     {balanceRequest(BalanceCommand{}), balanceResult{Date: "2026-10-23", DataVersion: 42}},
		"balance_explicit":     {balanceRequest(BalanceCommand{Scope: scope, Date: "2026-10-23", Amount: amount, Source: domain.BalanceSource{Kind: domain.StatementEOD, Namespace: source.Namespace, RecordedAt: at}, Note: "counted", ExpectedDataVersion: 42}), balanceResult{Date: "2026-10-23", DataVersion: 43}},
	}
}

func TestReceiptCompatibility(t *testing.T) {
	t.Parallel()
	encoded, err := os.ReadFile("testdata/receipts_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures map[string]struct{ Body, Digest, Receipt string }
	if err := json.Unmarshal(encoded, &fixtures); err != nil {
		t.Fatal(err)
	}
	for name, pair := range compatibilityCases() {
		t.Run(name, func(t *testing.T) {
			body, err := json.Marshal(pair[0])
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := json.Marshal(pair[1])
			if err != nil {
				t.Fatal(err)
			}
			want := fixtures[name]
			if string(body) != want.Body || fmt.Sprintf("%x", sha256.Sum256(body)) != want.Digest || string(receipt) != want.Receipt {
				t.Fatalf("persisted compatibility changed: body=%s receipt=%s", body, receipt)
			}
		})
	}
}
