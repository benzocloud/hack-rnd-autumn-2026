package payments

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
)

func compatibilityCases() map[string][2]any {
	id := uuid.MustParse("019a0c65-9800-7000-8000-000000000003")
	scope := ledger.Scope{CompanyID: uuid.MustParse("019a0c65-9800-7000-8000-000000000001"), AccountID: uuid.MustParse("019a0c65-9800-7000-8000-000000000002")}
	amount, _ := money.New(123456, money.RUB)
	at := time.Date(2026, 10, 23, 13, 14, 15, 123000000, time.FixedZone("offset", 3*60*60))
	source := ledger.Source{Kind: ledger.Imported, Namespace: ledger.Namespace{System: "bank", CompanyID: "C001"}, ExternalID: "OP1", ImportedAt: at}
	return map[string][2]any{
		"payment_defaults": {paymentRequest(CreateCommand{}), paymentResult{ID: id, Version: 1, DataVersion: 42}},
		"payment_explicit": {paymentRequest(CreateCommand{Record: domain.PaymentData{ID: id, Scope: scope, ScheduledDate: "2026-10-23", Direction: ledger.Inflow, Amount: amount, Source: source, Category: ledger.Category{ID: id, CompanyID: scope.CompanyID}, CounterpartyName: "Example", Description: "Expected", CreatedAt: at}, ExpectedDataVersion: 42}), paymentResult{ID: id, Version: 1, DataVersion: 43}},
		"match_defaults":   {matchRequest(MatchCommand{}), matchResult{ID: id, PaymentVersion: 2, DataVersion: 42}},
		"match_explicit":   {matchRequest(MatchCommand{ID: id, Scope: scope, PaymentID: id, TransactionID: scope.AccountID, Amount: amount, ExpectedVersion: 1, ExpectedDataVersion: 42, At: at}), matchResult{ID: id, PaymentVersion: 2, DataVersion: 43}},
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
