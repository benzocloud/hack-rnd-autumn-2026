package payments_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/payments"
)

type captureStore struct {
	calls      int
	actor, key string
}

func (s *captureStore) CreatePayment(_ context.Context, cmd payments.CreateCommand, record domain.ExpectedPayment) (payments.CreatePaymentResult, error) {
	s.calls++
	s.actor, s.key = cmd.ActorID, cmd.IdempotencyKey
	return payments.CreatePaymentResult{PaymentID: record.Data().ID, PaymentVersion: 7, DataVersion: 42}, nil
}

func (s *captureStore) MatchPayment(_ context.Context, cmd payments.MatchCommand) (payments.MatchPaymentResult, error) {
	s.calls++
	s.actor, s.key = cmd.ActorID, cmd.IdempotencyKey
	return payments.MatchPaymentResult{MatchID: cmd.ID, PaymentVersion: 9, DataVersion: 43}, nil
}

func paymentCommand(t *testing.T) payments.CreateCommand {
	t.Helper()
	amount, err := money.New(100, money.RUB)
	if err != nil {
		t.Fatal(err)
	}
	return payments.CreateCommand{Record: domain.PaymentData{Scope: ledger.Scope{
		CompanyID: uuid.MustParse("019a0c65-9800-7000-8000-000000000001"),
		AccountID: uuid.MustParse("019a0c65-9800-7000-8000-000000000002")},
		ScheduledDate: "2026-10-23", Direction: ledger.Inflow, Amount: amount, Source: ledger.Source{Kind: ledger.Manual}},
		ExpectedDataVersion: 1, ActorID: "auth-user-123", IdempotencyKey: "test-command"}
}

func matchCommand(t *testing.T) payments.MatchCommand {
	t.Helper()
	base := paymentCommand(t)
	return payments.MatchCommand{Scope: base.Record.Scope,
		PaymentID:     uuid.MustParse("019a0c65-9800-7000-8000-000000000003"),
		TransactionID: uuid.MustParse("019a0c65-9800-7000-8000-000000000004"), Amount: base.Record.Amount,
		ExpectedVersion: 1, ExpectedDataVersion: 1, ActorID: base.ActorID, IdempotencyKey: base.IdempotencyKey}
}

func TestCommandMetadata(t *testing.T) {
	t.Parallel()
	for _, kind := range []ledger.SourceKind{ledger.Manual, ledger.Imported} {
		t.Run(string(kind), func(t *testing.T) {
			for _, tc := range []struct {
				name, actor, key string
				complete         bool
			}{
				{name: "empty"}, {name: "actor only", actor: "auth-user-123"}, {name: "key only", key: "test-command"},
				{name: "both", actor: "auth-user-123", key: "test-command", complete: true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					cmd := paymentCommand(t)
					cmd.ActorID, cmd.IdempotencyKey = tc.actor, tc.key
					if kind == ledger.Imported {
						cmd.Record.Source = ledger.Source{Kind: kind, Namespace: ledger.Namespace{System: "dataset_v1", CompanyID: "C001"}, ExternalID: "PAY1", ImportedAt: time.Now()}
					}
					storage := &captureStore{}
					_, err := payments.New(storage).CreatePayment(t.Context(), cmd)
					valid := tc.complete || (kind == ledger.Imported && tc.actor == "" && tc.key == "")
					if valid {
						if err != nil || storage.calls != 1 {
							t.Fatalf("valid metadata: calls=%d, err=%v", storage.calls, err)
						}
					} else if !errors.Is(err, payments.ErrInvalid) || storage.calls != 0 {
						t.Fatalf("invalid metadata reached storage: calls=%d, err=%v", storage.calls, err)
					}
				})
			}
		})
	}
}
