package ledger_test

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
)

func manualBalanceCommand(t *testing.T) ledger.ManualBalanceCommand {
	t.Helper()
	base := transactionCommand(t)
	amount, _ := money.New(-500, money.RUB)
	return ledger.ManualBalanceCommand{Scope: base.Record.Scope, Date: "2026-10-23", Amount: amount, Note: "EOD", ExpectedDataVersion: 1, ActorID: base.ActorID, IdempotencyKey: "manual-balance-key", ViewDate: "2026-10-24"}
}

func TestManualBalanceRejectsCrossScopeObservation(t *testing.T) {
	cmd := manualBalanceCommand(t)
	foreign := cmd.Scope
	foreign.CompanyID = uuid.New()
	observation, err := domain.NewBalanceObservation(foreign, cmd.Date, cmd.Amount, domain.BalanceSource{Kind: domain.ManualEOD, RecordedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	storage := &captureStore{latestOverride: &observation}
	result, err := ledger.New(storage.withinTx).RecordManualBalance(t.Context(), cmd)
	if !errors.Is(err, ledger.ErrUnavailable) || !reflect.DeepEqual(result, ledger.RecordManualBalanceResult{}) || len(storage.completed) != 0 {
		t.Fatal("foreign projection reached receipt", result, err)
	}
}

func TestManualBalancePreservesFullResultOnReplay(t *testing.T) {
	cmd := manualBalanceCommand(t)
	storage := &captureStore{}
	service := ledger.New(storage.withinTx)
	first, err := service.RecordManualBalance(t.Context(), cmd)
	if err != nil || first.Account.DataVersion != 2 || first.Balance == nil || first.Balance.Amount().Minor() != -500 || first.Account.Factual != "" || storage.accountReads != 1 {
		t.Fatal(first, err)
	}
	digest := bytes.Clone(storage.digest)
	storage.payload, storage.replay = bytes.Clone(storage.completed), true
	if err := storage.account.BindFacts(domain.Source{Kind: domain.Manual}); err != nil {
		t.Fatal(err)
	}
	if err := storage.account.AdvanceDataVersion(); err != nil {
		t.Fatal(err)
	}
	storage.accountReads = 0
	cmd.ViewDate = "2026-10-25"
	replay, err := service.RecordManualBalance(t.Context(), cmd)
	if err != nil || !reflect.DeepEqual(replay, first) || storage.accountReads != 0 || !bytes.Equal(storage.digest, digest) {
		t.Fatal("replay hydrated current state or included clock in digest", replay, err)
	}
	if storage.operation != "record_manual_balance_v1" {
		t.Fatal(storage.operation)
	}
}

func TestManualBalanceValidationBeforeTransaction(t *testing.T) {
	for name, edit := range map[string]func(*ledger.ManualBalanceCommand){
		"future date":     func(c *ledger.ManualBalanceCommand) { c.Date = "2026-10-25" },
		"invalid date":    func(c *ledger.ManualBalanceCommand) { c.Date = "2026-02-30" },
		"short key":       func(c *ledger.ManualBalanceCommand) { c.IdempotencyKey = "short" },
		"invalid key":     func(c *ledger.ManualBalanceCommand) { c.IdempotencyKey = "bad key !!!" },
		"unicode key":     func(c *ledger.ManualBalanceCommand) { c.IdempotencyKey = "команда-123" },
		"missing actor":   func(c *ledger.ManualBalanceCommand) { c.ActorID = "" },
		"missing version": func(c *ledger.ManualBalanceCommand) { c.ExpectedDataVersion = 0 },
		"empty note":      func(c *ledger.ManualBalanceCommand) { c.Note = "" },
		"long note":       func(c *ledger.ManualBalanceCommand) { c.Note = strings.Repeat("я", 501) },
		"nul note":        func(c *ledger.ManualBalanceCommand) { c.Note = "EOD\x00" },
		"invalid money":   func(c *ledger.ManualBalanceCommand) { c.Amount = money.Money{} },
	} {
		t.Run(name, func(t *testing.T) {
			cmd := manualBalanceCommand(t)
			edit(&cmd)
			storage := &captureStore{}
			_, err := ledger.New(storage.withinTx).RecordManualBalance(t.Context(), cmd)
			if !errors.Is(err, ledger.ErrInvalid) || storage.calls != 0 {
				t.Fatal(err, storage.calls)
			}
		})
	}
}

func TestManualBalanceWithholdsFailedCommitAndStaleResult(t *testing.T) {
	for _, mode := range []string{"commit", "stale"} {
		t.Run(mode, func(t *testing.T) {
			cmd := manualBalanceCommand(t)
			storage := &captureStore{}
			want := ledger.ErrUnavailable
			if mode == "commit" {
				storage.txErr = ledger.ErrUnavailable
			} else {
				cmd.ExpectedDataVersion = 2
				want = ledger.ErrStaleVersion
			}
			result, err := ledger.New(storage.withinTx).RecordManualBalance(t.Context(), cmd)
			if !errors.Is(err, want) || !reflect.DeepEqual(result, ledger.RecordManualBalanceResult{}) {
				t.Fatal(result, err)
			}
		})
	}
}
