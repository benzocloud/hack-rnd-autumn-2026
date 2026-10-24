package forecasting_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
)

type captureStore struct {
	opening              domain.OpeningBalance
	found                bool
	inserted             int
	commitErr, insertErr error
	snapshot             domain.FinancialSnapshot
}

func (s *captureStore) withinTx(ctx context.Context, work forecasting.Work) error {
	if err := work(ctx, s); err != nil {
		return err
	}
	return s.commitErr
}
func (s *captureStore) ReadDataVersion(context.Context, ledger.Scope) (int64, error) { return 3, nil }
func (s *captureStore) ReadOpeningBalance(context.Context, ledger.Scope, ledger.Date) (domain.OpeningBalance, bool, error) {
	return s.opening, s.found, nil
}
func (s *captureStore) ReadFacts(context.Context, ledger.Scope, ledger.Date) ([]domain.Fact, error) {
	return nil, nil
}
func (s *captureStore) ReadCalendar(context.Context, ledger.Scope) (domain.Calendar, error) {
	return domain.Calendar{Reason: domain.HistoricalCalendarUnavailable}, nil
}
func (s *captureStore) InsertSnapshot(_ context.Context, snapshot domain.FinancialSnapshot) error {
	s.inserted++
	s.snapshot = snapshot
	return s.insertErr
}
func (s *captureStore) ReadSnapshot(context.Context, ledger.Scope, uuid.UUID) (domain.FinancialSnapshot, error) {
	return s.snapshot, nil
}

func TestSnapshotReturnsOnlyAfterCommit(t *testing.T) {
	scope := ledger.Scope{CompanyID: uuid.New(), AccountID: uuid.New()}
	amount, err := money.New(-321, money.RUB)
	if err != nil {
		t.Fatal(err)
	}
	balance, err := ledger.NewBalanceObservation(scope, "2026-10-23", amount, ledger.BalanceSource{Kind: ledger.ManualEOD, RecordedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	failed := errors.New("transaction failed")
	for _, tc := range []struct {
		name                       string
		found                      bool
		insertErr, commitErr, want error
	}{
		{"missing EOD", false, nil, nil, forecasting.ErrMissingBalance},
		{"insert fails", true, failed, nil, failed},
		{"commit fails", true, nil, failed, failed},
		{"committed", true, nil, nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &captureStore{opening: domain.OpeningBalance{Observation: balance, DataVersion: 3}, found: tc.found, insertErr: tc.insertErr, commitErr: tc.commitErr}
			service := forecasting.New(store.withinTx)
			got, err := service.CaptureSnapshot(t.Context(), forecasting.CaptureCommand{Scope: scope, Cutoff: "2026-10-23"})
			if !errors.Is(err, tc.want) {
				t.Fatalf("error=%v want=%v", err, tc.want)
			}
			if tc.want != nil && got.Data().ID != uuid.Nil {
				t.Fatal("uncommitted result returned")
			}
			if !tc.found && store.inserted != 0 {
				t.Fatal("missing balance persisted")
			}
			if tc.want == nil && (got.Data().ID.Version() != 7 || got.Data().Opening.Observation.Amount().Minor() != -321) {
				t.Fatal("invalid committed result")
			}
		})
	}
}
