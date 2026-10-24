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
	payload, completed, digest     []byte
	replay                         bool
	reserveErr, completeErr, txErr error
	operation, resource            string
	accountReads                   int
	calls                          int
	actor, key                     string
	account                        *ledger.BankAccount
	payment                        domain.ExpectedPayment
	paymentFound                   bool
	fact                           ledger.Transaction
	factFound                      bool
	allocated                      money.Money
	revision                       int64
	saves, inserts                 int
}

func (s *captureStore) prepare(scope ledger.Scope) error {
	if s.account != nil {
		return nil
	}
	var err error
	s.account, err = ledger.NewBankAccount(scope)
	return err
}
func (s *captureStore) withinTx(ctx context.Context, work payments.Work) error {
	s.calls++
	if err := work(ctx, s, s); err != nil {
		return err
	}
	return s.txErr
}
func (s *captureStore) Reserve(_ context.Context, _ ledger.Scope, actor, operation, resource, key string, digest []byte) ([]byte, bool, error) {
	s.actor, s.key = actor, key
	s.operation, s.resource = operation, resource
	s.digest = append([]byte(nil), digest...)
	return s.payload, s.replay, s.reserveErr
}
func (s *captureStore) Complete(_ context.Context, _ ledger.Scope, _, _, _, _ string, payload []byte) error {
	s.completed = append([]byte(nil), payload...)
	return s.completeErr
}
func (s *captureStore) LoadAccount(_ context.Context, scope ledger.Scope) (*ledger.BankAccount, error) {
	s.accountReads++
	if err := s.prepare(scope); err != nil {
		return nil, err
	}
	return s.account, nil
}
func (s *captureStore) SaveAccount(_ context.Context, account *ledger.BankAccount) error {
	s.saves++
	s.account = account
	return nil
}
func (s *captureStore) CarryHistoryCoverage(context.Context, ledger.Scope, int64, int64) error {
	return nil
}
func (s *captureStore) FindPayment(context.Context, ledger.Scope, uuid.UUID, ledger.Source) (domain.ExpectedPayment, int64, bool, error) {
	return s.payment, s.revision, s.paymentFound, nil
}
func (s *captureStore) InsertPayment(_ context.Context, record domain.ExpectedPayment, revision int64) error {
	s.inserts++
	s.payment, s.revision, s.paymentFound = record, revision, true
	return nil
}
func (s *captureStore) FindTransaction(context.Context, ledger.Scope, uuid.UUID, ledger.Source) (ledger.Transaction, int64, bool, error) {
	return s.fact, s.revision, s.factFound, nil
}
func (s *captureStore) FactAllocated(context.Context, ledger.Scope, uuid.UUID) (money.Money, error) {
	return s.allocated, nil
}
func (s *captureStore) SaveMatch(_ context.Context, payment domain.ExpectedPayment, _ domain.PaymentMatch, _ int64) error {
	s.inserts++
	s.payment = payment
	return nil
}

func matchStore(t *testing.T, cmd payments.MatchCommand) *captureStore {
	t.Helper()
	created, err := domain.NewExpectedPayment(domain.PaymentData{ID: cmd.PaymentID, Scope: cmd.Scope,
		ScheduledDate: "2026-10-23", Direction: ledger.Inflow, Amount: cmd.Amount, Source: ledger.Source{Kind: ledger.Manual}, CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	fact, err := ledger.NewTransaction(ledger.TransactionData{ID: cmd.TransactionID, Scope: cmd.Scope,
		Date: "2026-10-23", Direction: ledger.Inflow, Amount: cmd.Amount, Source: ledger.Source{Kind: ledger.Manual}})
	if err != nil {
		t.Fatal(err)
	}
	zero, err := money.New(0, money.RUB)
	if err != nil {
		t.Fatal(err)
	}
	return &captureStore{payment: created, paymentFound: true, fact: fact, factFound: true, allocated: zero}
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
					_, err := payments.New(storage.withinTx).CreatePayment(t.Context(), cmd)
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
