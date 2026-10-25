package ledger_test

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
)

func manualFact(t *testing.T) ledger.ManualTransactionCommand {
	t.Helper()
	base := transactionCommand(t)
	base.Record.CounterpartyINN = "0012345678"
	return ledger.ManualTransactionCommand{Record: base.Record, ExpectedDataVersion: 1, ActorID: base.ActorID, IdempotencyKey: "manual-transaction-key", ViewDate: "2026-10-24"}
}

func TestManualTransactionFullReplayAndAuthority(t *testing.T) {
	cmd := manualFact(t)
	storage := &captureStore{}
	service := ledger.New(storage.withinTx)
	first, err := service.CreateManualTransaction(t.Context(), cmd)
	if err != nil || first.DataVersion != 2 || first.Transaction.Version() != 1 || first.Transaction.Data().ID.Version() != 7 || first.CreatedAt.IsZero() || storage.account.State().Factual != domain.Manual || storage.saves != 1 || storage.inserts != 1 {
		t.Fatal(first, err)
	}
	digest := bytes.Clone(storage.digest)
	storage.payload, storage.replay, storage.accountReads = bytes.Clone(storage.completed), true, 0
	cmd.ViewDate = "2026-10-25"
	replay, err := service.CreateManualTransaction(t.Context(), cmd)
	if err != nil || !reflect.DeepEqual(first, replay) || storage.accountReads != 0 || !bytes.Equal(digest, storage.digest) || storage.operation != "create_manual_transaction_v1" {
		t.Fatal("replay changed result or depended on mutable state/clock", replay, err)
	}
}

func TestManualSupplementPreservesImportedAuthority(t *testing.T) {
	cmd := manualFact(t)
	storage := &captureStore{}
	storage.account, _ = domain.NewBankAccount(cmd.Record.Scope)
	namespace := domain.Namespace{System: "dataset_v1", CompanyID: "C001"}
	if err := storage.account.BindStatementFacts(namespace); err != nil {
		t.Fatal(err)
	}
	result, err := ledger.New(storage.withinTx).CreateManualTransaction(t.Context(), cmd)
	if err != nil || result.Transaction.Data().Source.Kind != domain.Manual || result.DataVersion != 2 || storage.account.State().Factual != domain.Imported || storage.account.State().Namespace != namespace {
		t.Fatal(result, err)
	}
}

func TestManualTransactionValidationBeforeSQL(t *testing.T) {
	for name, edit := range map[string]func(*ledger.ManualTransactionCommand){
		"future date":   func(c *ledger.ManualTransactionCommand) { c.Record.Date = "2026-10-25" },
		"invalid date":  func(c *ledger.ManualTransactionCommand) { c.Record.Date = "2026-02-30" },
		"zero money":    func(c *ledger.ManualTransactionCommand) { c.Record.Amount, _ = money.New(0, money.RUB) },
		"missing scope": func(c *ledger.ManualTransactionCommand) { c.Record.Scope.AccountID = uuid.Nil },
		"direction":     func(c *ledger.ManualTransactionCommand) { c.Record.Direction = "unknown" },
		"import source": func(c *ledger.ManualTransactionCommand) { c.Record.Source.Kind = domain.Imported },
		"client id":     func(c *ledger.ManualTransactionCommand) { c.Record.ID = uuid.New() },
		"short key":     func(c *ledger.ManualTransactionCommand) { c.IdempotencyKey = "short" },
		"actor":         func(c *ledger.ManualTransactionCommand) { c.ActorID = "" },
		"version":       func(c *ledger.ManualTransactionCommand) { c.ExpectedDataVersion = 0 },
		"INN":           func(c *ledger.ManualTransactionCommand) { c.Record.CounterpartyINN = "wrong" },
		"long name":     func(c *ledger.ManualTransactionCommand) { c.Record.CounterpartyName = strings.Repeat("я", 501) },
		"identical overlong strings": func(c *ledger.ManualTransactionCommand) {
			c.Record.CounterpartyName = strings.Repeat("x", 501)
			c.Record.Purpose = c.Record.CounterpartyName
		},
		"long purpose": func(c *ledger.ManualTransactionCommand) { c.Record.Purpose = strings.Repeat("x", 2001) },
		"NUL":          func(c *ledger.ManualTransactionCommand) { c.Record.Purpose = "a\x00b" },
		"invalid UTF8": func(c *ledger.ManualTransactionCommand) { c.Record.CounterpartyName = "\xff" },
		"foreign category": func(c *ledger.ManualTransactionCommand) {
			c.Record.Category = domain.Category{ID: uuid.New(), CompanyID: uuid.New()}
		},
	} {
		t.Run(name, func(t *testing.T) {
			cmd := manualFact(t)
			edit(&cmd)
			storage := &captureStore{}
			_, err := ledger.New(storage.withinTx).CreateManualTransaction(t.Context(), cmd)
			if !errors.Is(err, ledger.ErrInvalid) || storage.calls != 0 {
				t.Fatal(err, storage.calls)
			}
		})
	}
}

func TestManualTransactionStorageFailures(t *testing.T) {
	for _, name := range []string{"commit", "category", "stale", "duplicate imported", "scope"} {
		t.Run(name, func(t *testing.T) {
			cmd, storage := manualFact(t), &captureStore{}
			want := ledger.ErrUnavailable
			switch name {
			case "commit":
				storage.txErr = ledger.ErrUnavailable
			case "category":
				cmd.Record.Category = domain.Category{ID: uuid.New(), CompanyID: cmd.Record.Scope.CompanyID}
				want = ledger.ErrInvalid
			case "stale":
				cmd.ExpectedDataVersion = 2
				want = ledger.ErrStaleVersion
			case "duplicate imported":
				storage.account, _ = domain.NewBankAccount(cmd.Record.Scope)
				if err := storage.account.BindStatementFacts(domain.Namespace{System: "dataset_v1", CompanyID: "C001"}); err != nil {
					t.Fatal(err)
				}
				storage.importedEquivalent = true
				want = ledger.ErrSuspectedDuplicate
			case "scope":
				storage.account, _ = domain.NewBankAccount(domain.Scope{CompanyID: uuid.New(), AccountID: cmd.Record.Scope.AccountID})
			}
			result, err := ledger.New(storage.withinTx).CreateManualTransaction(t.Context(), cmd)
			if !errors.Is(err, want) || !reflect.DeepEqual(result, ledger.CreateManualTransactionResult{}) {
				t.Fatal(result, err)
			}
			if name != "commit" && (storage.saves != 0 || storage.inserts != 0 || len(storage.completed) != 0) {
				t.Fatal("failed command wrote data")
			}
		})
	}
}
