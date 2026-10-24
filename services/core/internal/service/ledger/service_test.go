package ledger_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
)

type captureStore struct {
	calls      int
	actor, key string
}

func (s *captureStore) CreateTransaction(_ context.Context, cmd ledger.TransactionCommand, record domain.Transaction) (ledger.CreateTransactionResult, error) {
	s.calls++
	s.actor, s.key = cmd.ActorID, cmd.IdempotencyKey
	return ledger.CreateTransactionResult{TransactionID: record.Data().ID, TransactionVersion: 7, DataVersion: 42}, nil
}

func (s *captureStore) RecordBalance(_ context.Context, cmd ledger.BalanceCommand, record domain.BalanceObservation) (ledger.RecordBalanceResult, error) {
	s.calls++
	s.actor, s.key = cmd.ActorID, cmd.IdempotencyKey
	return ledger.RecordBalanceResult{Date: record.Date(), DataVersion: 42}, nil
}

func transactionCommand(t *testing.T) ledger.TransactionCommand {
	t.Helper()
	amount, err := money.New(100, money.RUB)
	if err != nil {
		t.Fatal(err)
	}
	return ledger.TransactionCommand{
		Record: domain.TransactionData{Scope: domain.Scope{
			CompanyID: uuid.MustParse("019a0c65-9800-7000-8000-000000000001"),
			AccountID: uuid.MustParse("019a0c65-9800-7000-8000-000000000002")},
			Date: "2026-10-23", Direction: domain.Inflow, Amount: amount, Source: domain.Source{Kind: domain.Manual}},
		ExpectedDataVersion: 1, ActorID: "auth-user-123", IdempotencyKey: "test-command",
	}
}

func TestCommandMetadata(t *testing.T) {
	t.Parallel()
	for _, kind := range []domain.SourceKind{domain.Manual, domain.Imported} {
		t.Run(string(kind), func(t *testing.T) {
			for _, tc := range []struct {
				name, actor, key string
				complete         bool
			}{
				{name: "empty"}, {name: "actor only", actor: "auth-user-123"}, {name: "key only", key: "test-command"},
				{name: "both", actor: "auth-user-123", key: "test-command", complete: true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					cmd := transactionCommand(t)
					cmd.ActorID, cmd.IdempotencyKey = tc.actor, tc.key
					if kind == domain.Imported {
						cmd.Record.Source = domain.Source{Kind: kind, Namespace: domain.Namespace{System: "dataset_v1", CompanyID: "C001"}, ExternalID: "OP1", ImportedAt: time.Now()}
					}
					storage := &captureStore{}
					_, err := ledger.New(storage).CreateTransaction(t.Context(), cmd)
					valid := tc.complete || (kind == domain.Imported && tc.actor == "" && tc.key == "")
					if valid {
						if err != nil || storage.calls != 1 {
							t.Fatalf("valid metadata: calls=%d, err=%v", storage.calls, err)
						}
					} else if !errors.Is(err, ledger.ErrInvalid) || storage.calls != 0 {
						t.Fatalf("invalid metadata reached storage: calls=%d, err=%v", storage.calls, err)
					}
				})
			}
		})
	}
}
