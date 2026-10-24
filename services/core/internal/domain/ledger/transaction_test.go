package ledger_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

func TestTransactionReplay(t *testing.T) {
	t.Parallel()
	amount, err := money.New(100, money.RUB)
	if err != nil {
		t.Fatal(err)
	}
	data := ledger.TransactionData{ID: uuid.MustParse("019a0c65-9800-7000-8000-000000000004"), Scope: ledger.Scope{CompanyID: uuid.MustParse("019a0c65-9800-7000-8000-000000000001"), AccountID: uuid.MustParse("019a0c65-9800-7000-8000-000000000002")},
		Date: "2026-10-23", Direction: ledger.Inflow, Amount: amount, Source: importSource("dataset_v1", "C001", "TX1")}
	stored, err := ledger.NewTransaction(data)
	if err != nil {
		t.Fatal(err)
	}
	categorized, err := stored.Categorize(ledger.Category{ID: uuid.MustParse("019a0c65-9800-7000-8000-000000000006"), CompanyID: data.Scope.CompanyID})
	if err != nil || categorized.Version() != 2 || stored.Version() != 1 {
		t.Fatalf("category resource version: %v", err)
	}
	if replay, err := stored.CheckReplay(categorized); err != nil || !replay {
		t.Fatalf("classification changed imported economics: %v, %v", replay, err)
	}
	for _, tc := range []struct {
		name string
		edit func(*ledger.TransactionData)
		want bool
		err  error
	}{
		{"reimport", func(d *ledger.TransactionData) {
			d.ID = uuid.MustParse("019a0c65-9800-7000-8000-000000000005")
			d.Source.ImportedAt = d.Source.ImportedAt.Add(time.Hour)
		}, true, nil},
		{"other external id", func(d *ledger.TransactionData) { d.Source.ExternalID = "TX2" }, false, nil},
		{"changed fact", func(d *ledger.TransactionData) { d.Direction = ledger.Outflow }, false, ledger.ErrExternalRecord},
		{"other scope", func(d *ledger.TransactionData) {
			d.Scope.CompanyID = uuid.MustParse("019a0c65-9800-7000-8000-000000000003")
		}, false, ledger.ErrScope},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := data
			tc.edit(&d)
			candidate, err := ledger.NewTransaction(d)
			if err != nil {
				t.Fatal(err)
			}
			got, err := candidate.CheckReplay(stored)
			if got != tc.want || !errors.Is(err, tc.err) {
				t.Fatalf("got %v, %v", got, err)
			}
		})
	}
	data.Category = ledger.Category{ID: uuid.MustParse("019a0c65-9800-7000-8000-000000000007"), CompanyID: uuid.MustParse("019a0c65-9800-7000-8000-000000000003")}
	if _, err := ledger.NewTransaction(data); !errors.Is(err, ledger.ErrScope) {
		t.Fatalf("foreign category accepted: %v", err)
	}
}
