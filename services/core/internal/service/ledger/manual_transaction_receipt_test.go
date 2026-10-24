package ledger

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

func TestManualTransactionReceiptRejectsCorruption(t *testing.T) {
	amount, _ := money.New(100, money.RUB)
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	cmd := ManualTransactionCommand{Record: domain.TransactionData{Scope: domain.Scope{CompanyID: uuid.New(), AccountID: uuid.New()}, Date: "2026-10-23", Direction: domain.Inflow, Amount: amount, Source: domain.Source{Kind: domain.Manual}}, ExpectedDataVersion: 1}
	body := transactionRequest(manualTransactionCommand(cmd))
	body.ID = id
	fixture := manualTransactionReceipt{SchemaVersion: 1, Record: body, CreatedAt: time.Now().UTC().Truncate(time.Microsecond), DataVersion: 2}
	for name, edit := range map[string]func(*manualTransactionReceipt){
		"roundtrip":          func(*manualTransactionReceipt) {},
		"schema":             func(r *manualTransactionReceipt) { r.SchemaVersion = 0 },
		"missing id":         func(r *manualTransactionReceipt) { r.Record.ID = uuid.Nil },
		"wrong UUID version": func(r *manualTransactionReceipt) { r.Record.ID = uuid.New() },
		"scope":              func(r *manualTransactionReceipt) { r.Record.Scope.CompanyID = uuid.New() },
		"missing time":       func(r *manualTransactionReceipt) { r.CreatedAt = time.Time{} },
		"timestamp range":    func(r *manualTransactionReceipt) { r.CreatedAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) },
		"version":            func(r *manualTransactionReceipt) { r.DataVersion = 3 },
		"amount":             func(r *manualTransactionReceipt) { r.Record.Amount = 101 },
		"category": func(r *manualTransactionReceipt) {
			r.Record.Category = domain.Category{ID: uuid.New(), CompanyID: cmd.Record.Scope.CompanyID}
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := fixture
			edit(&r)
			if name == "timestamp range" {
				if _, err := restoreManualTransaction(cmd, r); !errors.Is(err, ErrUnavailable) {
					t.Fatal("invalid timestamp accepted", err)
				}
				return
			}
			data, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			var decoded manualTransactionReceipt
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatal(err)
			}
			result, err := restoreManualTransaction(cmd, decoded)
			if name == "roundtrip" {
				if err != nil || result.Transaction.Data().ID != id || !result.CreatedAt.Equal(fixture.CreatedAt) {
					t.Fatal(result, err)
				}
			} else if !errors.Is(err, ErrUnavailable) {
				t.Fatal("corrupt result accepted", err)
			}
		})
	}
}
