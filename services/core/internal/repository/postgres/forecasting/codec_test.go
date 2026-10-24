package forecasting

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

func TestCodecPreservesFinancialValuesAndRejectsCorruption(t *testing.T) {
	scope := ledger.Scope{CompanyID: uuid.New(), AccountID: uuid.New()}
	amount, err := money.New(math.MaxInt64, money.RUB)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 24, 1, 2, 3, 123456000, time.UTC)
	source := ledger.Source{Kind: ledger.Imported, Namespace: ledger.Namespace{System: "statement", CompanyID: "external-company"}, ExternalID: "external-operation", ImportedAt: at}
	fact, err := ledger.NewTransaction(ledger.TransactionData{ID: uuid.New(), Scope: scope, Date: "2026-10-23", Direction: ledger.Outflow, Amount: amount, Source: source, Category: ledger.Category{ID: uuid.New(), CompanyID: scope.CompanyID}, Purpose: "rent"})
	if err != nil {
		t.Fatal(err)
	}
	negative, err := money.New(math.MinInt64, money.RUB)
	if err != nil {
		t.Fatal(err)
	}
	balance, err := ledger.NewBalanceObservation(scope, "2026-10-23", negative, ledger.BalanceSource{Kind: ledger.StatementEOD, Namespace: source.Namespace, RecordedAt: at})
	if err != nil {
		t.Fatal(err)
	}
	d := domain.SnapshotData{ID: uuid.New(), Scope: scope, DataVersion: 4, Cutoff: "2026-10-23", CreatedAt: at, Opening: domain.OpeningBalance{Observation: balance, DataVersion: 3}, Facts: []domain.Fact{{Transaction: fact, DataVersion: 2}}, Calendar: domain.Calendar{Reason: domain.HistoricalCalendarUnavailable, ExcludedPayments: 7}}
	snapshot, err := domain.NewFinancialSnapshot(d)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := encodeSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	header := d
	header.Facts = nil
	header.Opening = domain.OpeningBalance{}
	header.Calendar = domain.Calendar{}
	got, err := decodeSnapshot(header, 1, payload)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Data(), snapshot.Data()) {
		t.Fatal("lossy roundtrip")
	}
	for _, bad := range []struct {
		version int
		payload []byte
	}{{2, payload}, {1, []byte(`{}`)}, {1, []byte(`{"opening":{"amount_minor":9223372036854775808}}`)}, {1, []byte(`{"facts":`)}} {
		if _, err := decodeSnapshot(header, bad.version, bad.payload); err == nil {
			t.Fatal("corrupt snapshot accepted")
		}
	}
	for _, target := range []string{"opening", "fact", "facts"} {
		for _, null := range []bool{false, true} {
			broken := corruptPayload(t, payload, target, null)
			if _, err := decodeSnapshot(header, 1, broken); err == nil {
				t.Fatalf("missing/null %s accepted", target)
			}
		}
	}
}

func corruptPayload(t *testing.T, payload []byte, target string, null bool) []byte {
	t.Helper()
	var object map[string]any
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&object); err != nil {
		t.Fatal(err)
	}
	fields, key := object, "amount_minor"
	switch target {
	case "opening":
		fields = object["opening"].(map[string]any)
	case "fact":
		fields = object["facts"].([]any)[0].(map[string]any)
	case "facts":
		key = "facts"
	default:
		t.Fatal("unknown corruption target")
	}
	if null {
		fields[key] = nil
	} else {
		delete(fields, key)
	}
	broken, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	return broken
}
