//go:build integration

package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/imports"
)

func TestCSVImportForecastWithoutManifestOrDuplicateFacts(t *testing.T) {
	application, cfg := testApplication(t)
	scope := newScope(t, application)
	body := "company_id;operation_id;operation_date;amount;direction;counterparty_inn;counterparty_name;payment_purpose;balance_eod\nC001;FIRST;2026-07-02;90.00;Поступление;0012345678;client;first;1000.00\nC001;LAST;2026-09-29;45.00;Списание;0012345678;supplier;last;955.00\n"
	importService, accepted, preview := stageImport(t, application, cfg, scope, body)
	if _, err := importService.Commit(t.Context(), imports.CommitCommand{Scope: scope, ActorID: "actor", Key: uuid.NewString(), ID: accepted.ID, ExpectedVersion: preview.Version, ExpectedDataVersion: 1, AcknowledgeWarnings: true, ViewDate: "2026-10-25"}); err != nil {
		t.Fatal(err)
	}
	assertCount(t, application, "SELECT count(*) FROM core_history_coverage WHERE company_id=$1", scope, 0)
	model := forecastModelFunc(func(_ context.Context, input forecasting.FinancialModelInput) (forecasting.FinancialModelResult, error) {
		if input.Assessment == nil || input.Assessment.OperationDays != 2 || input.Coverage.Kind != "" || len(input.Snapshot.History) != 0 || len(input.Operations) != 2 || input.Snapshot.Model.Version != forecasting.CSVAnalysisVersion {
			t.Fatal("CSV input lost provenance", input)
		}
		cutoff, _ := time.Parse(time.DateOnly, string(input.Snapshot.Cutoff))
		projection := forecasting.ModelResult{SnapshotID: input.Snapshot.SnapshotID, Scope: input.Snapshot.Scope, DataVersion: input.Snapshot.DataVersion, Cutoff: input.Snapshot.Cutoff, Model: input.Snapshot.Model}
		for n := 0; n < 30; n++ {
			projection.Days = append(projection.Days, forecasting.ModelForecastDay{Date: ledger.Date(cutoff.AddDate(0, 0, n+1).Format(time.DateOnly)), Inflow: rubModel(100), Outflow: rubModel(50), ReferenceBalance: rubModel(95500 + int64(n+1)*50)})
		}
		return forecasting.FinancialModelResult{Analysis: fixtureUnknownAnalysis(input), Projection: projection, ProbabilityUnavailableReason: "not_calibrated", Limitations: input.Assessment.Limitations}, nil
	})
	jobs := newForecastJobs(application.db, cfg.Postgres, model)
	cmd := forecasting.CreateForecastCommand{Scope: scope, ActorID: "actor", IdempotencyKey: uuid.NewString(), Cutoff: "2026-09-29", Today: "2026-10-25", HorizonDays: 30}
	job, err := jobs.Create(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if worked, err := jobs.ProcessOne(t.Context()); err != nil || !worked {
		t.Fatal(worked, err)
	}
	restarted := newForecastJobs(application.db, cfg.Postgres, model)
	view, err := restarted.Read(t.Context(), scope, job.ID)
	if err != nil || view.Status != "succeeded" || view.Assessment == nil || view.Coverage.Kind != "" || view.Result.Days[29].Balance != 97000 {
		t.Fatal(view, err)
	}
	replayed, err := restarted.Create(t.Context(), cmd)
	if err != nil || replayed.ID != job.ID {
		t.Fatal(replayed, err)
	}
	assertCount(t, application, "SELECT count(*) FROM core_transactions WHERE company_id=$1", scope, 2)
	assertCount(t, application, "SELECT count(*) FROM core_history_coverage WHERE company_id=$1", scope, 0)
	assertRevision(t, application, scope, 2)
	foreign := scope
	foreign.CompanyID = uuid.New()
	if _, err := restarted.Read(t.Context(), foreign, job.ID); err == nil {
		t.Fatal("foreign forecast exposed")
	}
	// New evaluation sees a new factual revision; the old immutable result survives.
	if _, err := application.db.ExecContext(t.Context(), "UPDATE core_accounts SET data_version=3 WHERE company_id=$1 AND id=$2", scope.CompanyID, scope.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := application.db.ExecContext(t.Context(), "UPDATE core_balances SET amount_minor=amount_minor+1,created_data_version=3 WHERE company_id=$1 AND account_id=$2 AND balance_date='2026-09-29'", scope.CompanyID, scope.AccountID); err != nil {
		t.Fatal(err)
	}
	cmd.IdempotencyKey = uuid.NewString()
	if _, err := restarted.Create(t.Context(), cmd); !errors.Is(err, forecasting.ErrHistoryReconciliation) {
		t.Fatal("gap not rejected", err)
	}
	old, err := restarted.Read(t.Context(), scope, job.ID)
	if err != nil || old.Result.Days[29].Balance != 97000 || old.CurrentDataVersion != 3 {
		t.Fatal("old result changed", old, err)
	}
}
