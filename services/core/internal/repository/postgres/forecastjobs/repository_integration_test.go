//go:build integration

package forecastjobs_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	appPostgres "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/app/postgres"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres/forecastjobs"
)

func testTransaction(t *testing.T) (context.Context, *sqlx.Tx, ledger.Scope) {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Postgres.Host != "127.0.0.1" || cfg.Postgres.Database != "saldo_core_test" {
		t.Fatal("integration tests require dedicated loopback saldo_core_test")
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 10*time.Second)
	t.Cleanup(cancel)
	db, err := appPostgres.Open(ctx, cfg.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	var marker string
	if err := db.GetContext(ctx, &marker, "SELECT coalesce(current_setting('saldo.test_database',true),'')"); err != nil || marker != "saldo-core-integration" {
		t.Fatal("refusing writes without dedicated test database marker", err)
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			t.Error(err)
		}
	})
	scope := ledger.Scope{CompanyID: uuid.New(), AccountID: uuid.New()}
	if _, err := tx.ExecContext(ctx, "INSERT INTO core_companies(id) VALUES($1)", scope.CompanyID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO core_accounts(company_id,id,currency,data_version) VALUES($1,$2,'RUB',1)", scope.CompanyID, scope.AccountID); err != nil {
		t.Fatal(err)
	}
	return ctx, tx, scope
}

func testJob(scope ledger.Scope) domain.Job {
	return domain.Job{ID: uuid.New(), Scope: scope, DataVersion: 1, Status: "queued",
		// Private transaction-local rows precede other test packages' real-time
		// fixtures; global claims skip these uncommitted rows in other sessions.
		CreatedAt: time.Date(100, time.January, 1, 0, 0, 0, 0, time.UTC), Input: []byte(`{"schema_version":1}`)}
}

func TestForecastJobCoverageAndTenantScope(t *testing.T) {
	ctx, tx, scope := testTransaction(t)
	r := forecastjobs.New(tx)
	revision, err := r.LockDataVersion(ctx, scope)
	if err != nil || revision != 1 {
		t.Fatal(revision, err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO core_history_coverage(company_id,account_id,data_version,from_date,through_date,reference,kind) VALUES($1,$2,1,'2026-01-01','2026-12-31','verified test source','certified')", scope.CompanyID, scope.AccountID); err != nil {
		t.Fatal(err)
	}
	coverage, found, err := r.ReadCoverage(ctx, scope, 1, "2026-07-26", "2026-10-23")
	if err != nil || !found || coverage.From != "2026-01-01" || coverage.Through != "2026-12-31" || coverage.Reference != "verified test source" {
		t.Fatal(coverage, found, err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO core_history_coverage(company_id,account_id,data_version,from_date,through_date,reference) VALUES($1,$2,3,'2026-01-01','2026-12-31','legacy source')", scope.CompanyID, scope.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, found, err := r.ReadCoverage(ctx, scope, 3, "2026-07-26", "2026-10-23"); err != nil || found {
		t.Fatal("legacy coverage without kind must stay unread, not fail the read", found, err)
	}
	for _, test := range []struct {
		scope    ledger.Scope
		revision int64
		from     ledger.Date
		through  ledger.Date
	}{
		{scope, 2, "2026-07-26", "2026-10-23"},
		{scope, 1, "2025-12-31", "2026-10-23"},
		{scope, 1, "2026-07-26", "2027-01-01"},
		{ledger.Scope{CompanyID: uuid.New(), AccountID: scope.AccountID}, 1, "2026-07-26", "2026-10-23"},
	} {
		if _, found, err := r.ReadCoverage(ctx, test.scope, test.revision, test.from, test.through); err != nil || found {
			t.Fatal("uncertified scope/revision/range accepted", found, err)
		}
	}
	job := testJob(scope)
	if err := r.InsertJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	foreign := scope
	foreign.CompanyID = uuid.New()
	if _, err := r.ReadJob(ctx, foreign, job.ID); !errors.Is(err, storage.ErrNotFound) {
		t.Fatal("foreign tenant read a job", err)
	}
	if _, err := r.LockDataVersion(ctx, foreign); !errors.Is(err, storage.ErrNotFound) {
		t.Fatal("foreign tenant locked an account", err)
	}
	if jobs, err := r.ListJobs(ctx, foreign, time.Time{}, uuid.Nil, 50); err != nil || len(jobs) != 0 {
		t.Fatal("foreign tenant listed jobs", jobs, err)
	}
}

func TestForecastJobClaimLeaseAndStaleCompletion(t *testing.T) {
	ctx, tx, scope := testTransaction(t)
	r := forecastjobs.New(tx)
	job := testJob(scope)
	if err := r.InsertJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	firstToken := uuid.New()
	claimed, found, err := r.ClaimJob(ctx, firstToken)
	if err != nil || !found || claimed.ID != job.ID || claimed.Status != "running" || claimed.LeaseToken != firstToken {
		t.Fatal(claimed, found, err)
	}
	// A live running job is skipped even though it precedes the next queued job.
	var live bool
	if err := tx.GetContext(ctx, &live, "SELECT lease_expires_at > clock_timestamp() FROM core_forecast_jobs WHERE company_id=$1 AND account_id=$2 AND id=$3", scope.CompanyID, scope.AccountID, job.ID); err != nil || !live {
		t.Fatal(live, err)
	}
	other := testJob(scope)
	other.CreatedAt = job.CreatedAt.Add(time.Minute)
	if err := r.InsertJob(ctx, other); err != nil {
		t.Fatal(err)
	}
	otherToken := uuid.New()
	next, found, err := r.ClaimJob(ctx, otherToken)
	if err != nil || !found || next.ID != other.ID {
		t.Fatal("live lease was claimed again", next, found, err)
	}
	next.Status, next.ErrorCode, next.RequestID = "failed", "DEPENDENCY_UNAVAILABLE", "test-worker-request"
	if err := r.FinishJob(ctx, next, otherToken); err != nil {
		t.Fatal(err)
	}
	failed, err := r.ReadJob(ctx, scope, other.ID)
	if err != nil || failed.Status != "failed" || failed.ErrorCode != next.ErrorCode || failed.RequestID != next.RequestID || failed.CompletedAt == nil || len(failed.Result) != 0 {
		t.Fatal("failure did not persist safely", failed, err)
	}
	finished := claimed
	finished.Status, finished.Result = "succeeded", []byte(`{"answer":42}`)
	if err := r.FinishJob(ctx, finished, uuid.New()); !errors.Is(err, storage.ErrConflict) {
		t.Fatal("wrong lease token finished a job", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE core_forecast_jobs SET lease_expires_at=clock_timestamp()-interval '1 second' WHERE company_id=$1 AND account_id=$2 AND id=$3", scope.CompanyID, scope.AccountID, job.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.FinishJob(ctx, finished, firstToken); !errors.Is(err, storage.ErrConflict) {
		t.Fatal("expired lease finished a job", err)
	}
	secondToken := uuid.New()
	reclaimed, found, err := r.ClaimJob(ctx, secondToken)
	if err != nil || !found || reclaimed.ID != job.ID || reclaimed.LeaseToken != secondToken {
		t.Fatal(reclaimed, found, err)
	}
	if err := r.FinishJob(ctx, finished, firstToken); !errors.Is(err, storage.ErrConflict) {
		t.Fatal("old worker overwrote reclaimed job", err)
	}
	if err := r.FinishJob(ctx, finished, secondToken); err != nil {
		t.Fatal(err)
	}
	saved, err := r.ReadJob(ctx, scope, job.ID)
	if err != nil || saved.Status != "succeeded" || saved.CompletedAt == nil || saved.LeaseToken != uuid.Nil || len(saved.Result) == 0 {
		t.Fatal(saved, err)
	}
	var input map[string]int
	if err := json.Unmarshal(saved.Input, &input); err != nil || input["schema_version"] != 1 {
		t.Fatal("immutable input changed", input, err)
	}
	if err := r.FinishJob(ctx, finished, secondToken); !errors.Is(err, storage.ErrConflict) {
		t.Fatal("terminal job was overwritten", err)
	}
	listed, err := r.ListJobs(ctx, scope, time.Time{}, uuid.Nil, 50)
	if err != nil || len(listed) != 2 || listed[0].Status != "failed" || listed[1].Status != "succeeded" || len(listed[0].Result) != 0 || len(listed[1].Result) != 0 {
		t.Fatal("summary loaded full result", listed, err)
	}
}

func TestForecastJobListStableKeyset(t *testing.T) {
	ctx, tx, scope := testTransaction(t)
	r := forecastjobs.New(tx)
	ids := []uuid.UUID{
		uuid.MustParse("019a0c65-9800-7000-8000-000000000001"),
		uuid.MustParse("019a0c65-9800-7000-8000-000000000002"),
		uuid.MustParse("019a0c65-9800-7000-8000-000000000003"),
	}
	for _, id := range ids {
		job := testJob(scope)
		job.ID = id
		if err := r.InsertJob(ctx, job); err != nil {
			t.Fatal(err)
		}
	}
	first, err := r.ListJobs(ctx, scope, time.Time{}, uuid.Nil, 2)
	if err != nil || len(first) != 2 || first[0].ID != ids[2] || first[1].ID != ids[1] {
		t.Fatal(first, err)
	}
	second, err := r.ListJobs(ctx, scope, first[1].CreatedAt, first[1].ID, 2)
	if err != nil || len(second) != 1 || second[0].ID != ids[0] {
		t.Fatal("cursor repeated/skipped job", second, err)
	}
}
