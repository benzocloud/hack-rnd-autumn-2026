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
	payload, completed, digest     []byte
	replay                         bool
	reserveErr, completeErr, txErr error
	operation, resource            string
	accountReads                   int
	calls                          int
	actor, key                     string
	account                        *domain.BankAccount
	transaction                    domain.Transaction
	transactionFound               bool
	balance                        domain.BalanceObservation
	latestOverride                 *domain.BalanceObservation
	balanceFound                   bool
	revision                       int64
	saves, inserts                 int
	categoryExists                 bool
	createdAt                      time.Time
	importedEquivalent             bool
}

func (s *captureStore) HasImportedEquivalent(context.Context, domain.TransactionData) (bool, error) {
	return s.importedEquivalent, nil
}

func (s *captureStore) CategoryExists(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return s.categoryExists, nil
}

func (s *captureStore) TransactionCreatedAt(context.Context, domain.Scope, uuid.UUID) (time.Time, error) {
	if s.createdAt.IsZero() {
		s.createdAt = time.Now().UTC().Truncate(time.Microsecond)
	}
	return s.createdAt, nil
}

func (s *captureStore) prepare(scope domain.Scope) error {
	if s.account != nil {
		return nil
	}
	var err error
	s.account, err = domain.NewBankAccount(scope)
	return err
}

func (s *captureStore) withinTx(ctx context.Context, work ledger.Work) error {
	s.calls++
	if err := work(ctx, s, s); err != nil {
		return err
	}
	return s.txErr
}
func (s *captureStore) Reserve(_ context.Context, _ domain.Scope, actor, operation, resource, key string, digest []byte) ([]byte, bool, error) {
	s.actor, s.key = actor, key
	s.operation, s.resource = operation, resource
	s.digest = append([]byte(nil), digest...)
	return s.payload, s.replay, s.reserveErr
}
func (s *captureStore) Complete(_ context.Context, _ domain.Scope, _, _, _, _ string, payload []byte) error {
	s.completed = append([]byte(nil), payload...)
	return s.completeErr
}
func (s *captureStore) LoadAccount(_ context.Context, scope domain.Scope) (*domain.BankAccount, error) {
	s.accountReads++
	if err := s.prepare(scope); err != nil {
		return nil, err
	}
	return s.account, nil
}
func (s *captureStore) SaveAccount(_ context.Context, account *domain.BankAccount) error {
	s.saves++
	s.account = account
	return nil
}
func (s *captureStore) FindTransaction(context.Context, domain.Scope, uuid.UUID, domain.Source) (domain.Transaction, int64, bool, error) {
	return s.transaction, s.revision, s.transactionFound, nil
}
func (s *captureStore) InsertTransaction(_ context.Context, record domain.Transaction, revision int64) error {
	s.inserts++
	s.transaction, s.revision, s.transactionFound = record, revision, true
	return nil
}
func (s *captureStore) FindBalance(context.Context, domain.Scope, domain.Date) (domain.BalanceObservation, int64, bool, error) {
	return s.balance, s.revision, s.balanceFound, nil
}

func (s *captureStore) FindLatestBalance(context.Context, domain.Scope, domain.Date) (domain.BalanceObservation, int64, bool, error) {
	if s.latestOverride != nil {
		return *s.latestOverride, s.revision, true, nil
	}
	return s.balance, s.revision, s.balanceFound, nil
}
func (s *captureStore) InsertBalance(_ context.Context, record domain.BalanceObservation, _ string, revision int64) error {
	s.inserts++
	s.balance, s.revision, s.balanceFound = record, revision, true
	return nil
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
					_, err := ledger.New(storage.withinTx).CreateTransaction(t.Context(), cmd)
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
