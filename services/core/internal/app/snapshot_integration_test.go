//go:build integration

package app

import (
	"context"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	appPostgres "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/app/postgres"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
	forecastSQL "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
	ledgerService "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
)

// Test-local wrapper commits a real writer after the first MVCC read. There is
// no production test hook, mock SQL, lock-based serialization or timing race.
type snapshotStore interface {
	ReadDataVersion(context.Context, ledger.Scope) (int64, error)
	ReadOpeningBalance(context.Context, ledger.Scope, ledger.Date) (domain.OpeningBalance, bool, error)
	ReadFacts(context.Context, ledger.Scope, ledger.Date) ([]domain.Fact, error)
	ReadCalendar(context.Context, ledger.Scope) (domain.Calendar, error)
	InsertSnapshot(context.Context, domain.FinancialSnapshot) error
	ReadSnapshot(context.Context, ledger.Scope, uuid.UUID) (domain.FinancialSnapshot, error)
}

type interleavedSnapshotStore struct {
	snapshotStore
	write   func() error
	observe func(domain.SnapshotData)
}

func (s interleavedSnapshotStore) InsertSnapshot(ctx context.Context, snapshot domain.FinancialSnapshot) error {
	s.observe(snapshot.Data())
	return s.snapshotStore.InsertSnapshot(ctx, snapshot)
}

func (s interleavedSnapshotStore) ReadDataVersion(ctx context.Context, scope ledger.Scope) (int64, error) {
	revision, err := s.snapshotStore.ReadDataVersion(ctx, scope)
	if err != nil {
		return 0, err
	}
	if err := s.write(); err != nil {
		return 0, err
	}
	return revision, nil
}

func TestSnapshotConsistentAcrossCommittedMatchAndNewFacts(t *testing.T) {
	a, cfg := testApplication(t)
	scope := newScope(t, a)
	cmd := transactionCommand(t, scope)
	cmd.Record.Date = "2026-10-24"
	future, err := a.Ledger.CreateTransaction(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Ledger.RecordBalance(t.Context(), ledgerService.BalanceCommand{Scope: scope, Date: "2026-10-23", Amount: rub(t, math.MinInt64),
		Source: ledger.BalanceSource{Kind: ledger.ManualEOD}, Note: "opening fixture", ExpectedDataVersion: 2, ActorID: "test-owner", IdempotencyKey: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	payment, err := a.Payments.CreatePayment(t.Context(), paymentCommand(t, scope, 3))
	if err != nil {
		t.Fatal(err)
	}
	old, err := a.Forecasting.CaptureSnapshot(t.Context(), forecasting.CaptureCommand{Scope: scope, Cutoff: "2026-10-23"})
	if err != nil {
		t.Fatal(err)
	}
	var observed []domain.SnapshotData
	wrote := false
	service := forecasting.New(func(ctx context.Context, work forecasting.Work) error {
		return appPostgres.WithinRepeatableRead(ctx, a.db, cfg.Postgres.QueryTimeout, func(ctx context.Context, tx *sqlx.Tx) error {
			return work(ctx, interleavedSnapshotStore{snapshotStore: forecastSQL.New(tx), observe: func(d domain.SnapshotData) { observed = append(observed, d) }, write: func() error {
				if wrote {
					return nil
				}
				wrote = true
				fact := transactionCommand(t, scope)
				fact.ExpectedDataVersion = 4
				fact.Record.Amount = rub(t, math.MaxInt64)
				if _, err := a.Ledger.CreateTransaction(ctx, fact); err != nil {
					return err
				}
				if _, err := a.Payments.CreatePayment(ctx, paymentCommand(t, scope, 5)); err != nil {
					return err
				}
				_, err := a.Payments.MatchPayment(ctx, matchCommand(t, scope, future.TransactionID, payment.PaymentID, 6))
				return err
			}})
		})
	})
	current, err := service.CaptureSnapshot(t.Context(), forecasting.CaptureCommand{Scope: scope, Cutoff: "2026-10-23"})
	if err != nil {
		t.Fatal(err)
	}
	if len(observed) != 2 || observed[0].DataVersion != 4 || len(observed[0].Facts) != 0 || observed[0].Calendar.ExcludedPayments != 1 ||
		observed[1].DataVersion != 7 || len(observed[1].Facts) != 1 || observed[1].Calendar.ExcludedPayments != 2 || current.Data().DataVersion != 7 {
		t.Fatalf("retry mixed MVCC views: %+v", observed)
	}
	assertCount(t, a, "SELECT count(*) FROM core_financial_snapshots WHERE company_id=$1", scope, 2)
	d := old.Data()
	if d.DataVersion != 4 || len(d.Facts) != 0 || d.Calendar.ExcludedPayments != 1 || d.Calendar.Reason != domain.HistoricalCalendarUnavailable {
		t.Fatalf("mixed snapshot: %+v", d)
	}
	assertRevision(t, a, scope, 7)
	// The old payload remains unchanged after reconnect; fresh captures see v7.
	other, err := New(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	loaded, err := other.Forecasting.GetSnapshot(t.Context(), scope, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Data(), d) {
		t.Fatal("persisted snapshot changed or lost precision")
	}
	changed := d
	changed.CreatedAt = changed.CreatedAt.Add(time.Microsecond)
	replacement, err := domain.NewFinancialSnapshot(changed)
	if err != nil {
		t.Fatal(err)
	}
	err = appPostgres.WithinRepeatableRead(t.Context(), a.db, cfg.Postgres.QueryTimeout, func(ctx context.Context, tx *sqlx.Tx) error {
		return forecastSQL.New(tx).InsertSnapshot(ctx, replacement)
	})
	requireError(t, err, storage.ErrConflict)
	loaded, err = other.Forecasting.GetSnapshot(t.Context(), scope, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Data(), d) {
		t.Fatal("duplicate ID overwrote immutable snapshot")
	}
	fresh, err := other.Forecasting.CaptureSnapshot(t.Context(), forecasting.CaptureCommand{Scope: scope, Cutoff: "2026-10-23"})
	if err != nil {
		t.Fatal(err)
	}
	newData := fresh.Data()
	if newData.DataVersion != 7 || len(newData.Facts) != 1 || newData.Facts[0].Transaction.Data().Amount.Minor() != math.MaxInt64 || newData.Calendar.ExcludedPayments != 2 || newData.Opening.Observation.Amount().Minor() != math.MinInt64 {
		t.Fatalf("fresh snapshot lost financial input: %+v", newData)
	}
	assertRevision(t, a, scope, 7)
	// Neither a different tenant nor a different account can retrieve the ID.
	foreign := newScope(t, a)
	_, err = other.Forecasting.GetSnapshot(t.Context(), foreign, d.ID)
	requireError(t, err, forecasting.ErrNotFound)
	_, err = other.Forecasting.GetSnapshot(t.Context(), ledger.Scope{CompanyID: foreign.CompanyID, AccountID: scope.AccountID}, d.ID)
	requireError(t, err, forecasting.ErrNotFound)
}

func TestSnapshotRequiresExactEODAndKnownScope(t *testing.T) {
	a, _ := testApplication(t)
	scope := newScope(t, a)
	_, err := a.Ledger.RecordBalance(t.Context(), ledgerService.BalanceCommand{Scope: scope, Date: "2026-10-22", Amount: rub(t, 100),
		Source: ledger.BalanceSource{Kind: ledger.ManualEOD}, Note: "previous EOD", ExpectedDataVersion: 1, ActorID: "test-owner", IdempotencyKey: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Forecasting.CaptureSnapshot(t.Context(), forecasting.CaptureCommand{Scope: scope, Cutoff: "2026-10-23"})
	requireError(t, err, forecasting.ErrMissingBalance)
	assertCount(t, a, "SELECT count(*) FROM core_financial_snapshots WHERE company_id=$1", scope, 0)
	assertRevision(t, a, scope, 2)
	unknown := ledger.Scope{CompanyID: uuid.New(), AccountID: scope.AccountID}
	_, err = a.Forecasting.CaptureSnapshot(t.Context(), forecasting.CaptureCommand{Scope: unknown, Cutoff: "2026-10-22"})
	requireError(t, err, forecasting.ErrNotFound)
}
