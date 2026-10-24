package ledger_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

func TestBalanceAndBusinessDate(t *testing.T) {
	t.Parallel()
	negative, err := money.New(-500, money.RUB)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 24, 12, 0, 0, 0, time.FixedZone("business", 3*60*60))
	balance, err := ledger.NewBalanceObservation(ledger.Scope{CompanyID: uuid.MustParse("019a0c65-9800-7000-8000-000000000001"), AccountID: uuid.MustParse("019a0c65-9800-7000-8000-000000000002")},
		"2026-10-23", negative, ledger.BalanceSource{Kind: ledger.ManualEOD, RecordedAt: at})
	if err != nil || balance.Amount() != negative || balance.Date() != "2026-10-23" || balance.Source().RecordedAt.Location() != time.UTC {
		t.Fatalf("balance: %v, %v", balance, err)
	}
	for _, date := range []ledger.Date{"", "2026-02-29", "2026-1-01", "2026-10-23T00:00:00Z"} {
		if err := date.Validate(); err == nil {
			t.Errorf("invalid date accepted: %q", date)
		}
	}
	if err := (ledger.Date("2024-02-29")).Validate(); err != nil {
		t.Fatal(err)
	}
	repeated, err := ledger.NewBalanceObservation(balance.Scope(), balance.Date(), negative,
		ledger.BalanceSource{Kind: ledger.ManualEOD, RecordedAt: at.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if replay, err := repeated.CheckReplay(balance); err != nil || !replay {
		t.Fatalf("EOD repeated on reimport: %v, %v", replay, err)
	}
	changed, err := money.New(0, money.RUB)
	if err != nil {
		t.Fatal(err)
	}
	conflict, err := ledger.NewBalanceObservation(balance.Scope(), balance.Date(), changed, balance.Source())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conflict.CheckReplay(balance); !errors.Is(err, ledger.ErrBalanceConflict) {
		t.Fatalf("conflicting EOD accepted: %v", err)
	}
}
