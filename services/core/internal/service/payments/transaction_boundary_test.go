package payments_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/payments"
)

func TestCommandTransactionBoundary(t *testing.T) {
	t.Parallel()
	id := uuid.MustParse("019a0c65-9800-7000-8000-000000000003")
	for _, tc := range []struct {
		name, payload string
		invalid       []string
		want, zero    any
		run           func(*captureStore) (any, error)
	}{
		{name: "payment", payload: `{"id":"019a0c65-9800-7000-8000-000000000003","version":1,"data_version":42}`,
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
			want: payments.CreatePaymentResult{PaymentID: id, PaymentVersion: 1, DataVersion: 42}, zero: payments.CreatePaymentResult{},
			run: func(s *captureStore) (any, error) {
				return payments.New(s.withinTx).CreatePayment(t.Context(), paymentCommand(t))
			}},
		{name: "match", payload: `{"id":"019a0c65-9800-7000-8000-000000000003","payment_version":2,"data_version":42}`,
			invalid: []string{
				`{"id":null,"payment_version":2,"data_version":42}`,
				`{"id":"00000000-0000-0000-0000-000000000000","payment_version":2,"data_version":42}`,
				`{"payment_version":2,"data_version":42}`,
				`{"id":"019a0c65-9800-7000-8000-000000000003","payment_version":null,"data_version":42}`,
				`{"id":"019a0c65-9800-7000-8000-000000000003","payment_version":0,"data_version":42}`,
				`{"id":"019a0c65-9800-7000-8000-000000000003","payment_version":-1,"data_version":42}`,
				`{"id":"019a0c65-9800-7000-8000-000000000003","data_version":42}`,
				`{"id":"019a0c65-9800-7000-8000-000000000003","payment_version":2,"data_version":null}`,
				`{"id":"019a0c65-9800-7000-8000-000000000003","payment_version":2,"data_version":0}`,
				`{"id":"019a0c65-9800-7000-8000-000000000003","payment_version":2,"data_version":-1}`,
				`{"id":"019a0c65-9800-7000-8000-000000000003","payment_version":2}`,
			},
			want: payments.MatchPaymentResult{MatchID: id, PaymentVersion: 2, DataVersion: 42}, zero: payments.MatchPaymentResult{},
			run: func(s *captureStore) (any, error) {
				return payments.New(s.withinTx).MatchPayment(t.Context(), matchCommand(t))
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
					if !errors.Is(err, payments.ErrUnavailable) || got != tc.zero || storage.accountReads != 0 {
						t.Fatalf("invalid receipt executed scenario: %+v %v", got, err)
					}
				})
			}
			t.Run("reservation conflict before reads", func(t *testing.T) {
				storage := &captureStore{reserveErr: payments.ErrIdempotencyConflict}
				got, err := tc.run(storage)
				if !errors.Is(err, payments.ErrIdempotencyConflict) || got != tc.zero || storage.accountReads != 0 {
					t.Fatalf("reservation conflict executed scenario: %+v %v", got, err)
				}
			})
			for _, failure := range []string{"commit", "receipt write"} {
				t.Run(failure, func(t *testing.T) {
					storage := &captureStore{}
					if tc.name == "match" {
						storage = matchStore(t, matchCommand(t))
					}
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
