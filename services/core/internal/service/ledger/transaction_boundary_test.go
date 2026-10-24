package ledger_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
)

func balanceCommand(t *testing.T) ledger.BalanceCommand {
	t.Helper()
	base := transactionCommand(t)
	return ledger.BalanceCommand{Scope: base.Record.Scope, Date: base.Record.Date, Amount: base.Record.Amount, Source: domain.BalanceSource{Kind: domain.ManualEOD}, Note: "counted", ExpectedDataVersion: 1, ActorID: base.ActorID, IdempotencyKey: base.IdempotencyKey}
}

func TestCommandTransactionBoundary(t *testing.T) {
	t.Parallel()
	id := uuid.MustParse("019a0c65-9800-7000-8000-000000000003")
	for _, tc := range []struct {
		name, payload string
		invalid       []string
		want, zero    any
		run           func(*captureStore) (any, error)
	}{
		{name: "transaction", payload: `{"id":"019a0c65-9800-7000-8000-000000000003","version":1,"data_version":42}`,
			invalid: []string{
				`{"id":null,"version":1,"data_version":42}`,
				`{"id":"00000000-0000-0000-0000-000000000000","version":1,"data_version":42}`,
				`{"version":1,"data_version":42}`,
				`{"id":"019a0c65-9800-7000-8000-000000000003","version":null,"data_version":42}`,
				`{"id":"019a0c65-9800-7000-8000-000000000003","version":0,"data_version":42}`,
				`{"id":"019a0c65-9800-7000-8000-000000000003","version":-1,"data_version":42}`,
				`{"id":"019a0c65-9800-7000-8000-000000000003","data_version":42}`,
				`{"id":"019a0c65-9800-7000-8000-000000000003","version":1,"data_version":null}`,
				`{"id":"019a0c65-9800-7000-8000-000000000003","version":1,"data_version":0}`,
				`{"id":"019a0c65-9800-7000-8000-000000000003","version":1,"data_version":-1}`,
				`{"id":"019a0c65-9800-7000-8000-000000000003","version":1}`,
			},
			want: ledger.CreateTransactionResult{TransactionID: id, TransactionVersion: 1, DataVersion: 42}, zero: ledger.CreateTransactionResult{},
			run: func(s *captureStore) (any, error) {
				return ledger.New(s.withinTx).CreateTransaction(t.Context(), transactionCommand(t))
			}},
		{name: "balance", payload: `{"date":"2026-10-23","data_version":42}`,
			invalid: []string{
				`{"date":null,"data_version":42}`,
				`{"date":"","data_version":42}`,
				`{"date":"2026-02-30","data_version":42}`,
				`{"data_version":42}`,
				`{"date":"2026-10-23","data_version":null}`,
				`{"date":"2026-10-23","data_version":0}`,
				`{"date":"2026-10-23","data_version":-1}`,
				`{"date":"2026-10-23"}`,
			},
			want: ledger.RecordBalanceResult{Date: "2026-10-23", DataVersion: 42}, zero: ledger.RecordBalanceResult{},
			run: func(s *captureStore) (any, error) {
				return ledger.New(s.withinTx).RecordBalance(t.Context(), balanceCommand(t))
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("replay before resource reads", func(t *testing.T) {
				storage := &captureStore{replay: true, payload: []byte(tc.payload)}
				got, err := tc.run(storage)
				if err != nil || got != tc.want || storage.accountReads != 0 || storage.inserts != 0 || len(storage.completed) != 0 {
					t.Fatalf("replay executed scenario: result=%+v err=%v", got, err)
				}
			})
			for _, payload := range append([]string{"", "null", "{", "[]", "{}"}, tc.invalid...) {
				t.Run("invalid receipt "+payload, func(t *testing.T) {
					storage := &captureStore{replay: true, payload: []byte(payload)}
					got, err := tc.run(storage)
					if !errors.Is(err, ledger.ErrUnavailable) || got != tc.zero || storage.accountReads != 0 {
						t.Fatalf("invalid receipt executed scenario: %+v %v", got, err)
					}
				})
			}
			t.Run("reservation conflict before reads", func(t *testing.T) {
				storage := &captureStore{reserveErr: ledger.ErrIdempotencyConflict}
				got, err := tc.run(storage)
				if !errors.Is(err, ledger.ErrIdempotencyConflict) || got != tc.zero || storage.accountReads != 0 {
					t.Fatalf("reservation conflict executed scenario: %+v %v", got, err)
				}
			})
			for _, failure := range []string{"commit", "receipt write"} {
				t.Run(failure, func(t *testing.T) {
					storage := &captureStore{}

					want := errors.New("transaction failed")
					if failure == "commit" {
						storage.txErr = want
					} else {
						storage.completeErr = want
					}
					got, err := tc.run(storage)
					if !errors.Is(err, want) || got != tc.zero || len(storage.completed) == 0 {
						t.Fatalf("failed transaction exposed result: %+v %v", got, err)
					}
				})
			}
		})
	}
}
