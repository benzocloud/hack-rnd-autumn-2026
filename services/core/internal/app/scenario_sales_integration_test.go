//go:build integration

package app

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/imports"
)

func TestSalesScenarioFrozenBasisConcurrentReplayAndLegacyRead(t *testing.T) {
	application, cfg := testApplication(t)
	scope := newScope(t, application)
	body := "company_id;operation_id;operation_date;amount;direction;counterparty_inn;counterparty_name;payment_purpose;balance_eod\nC001;FIRST;2026-07-02;90.00;Поступление;0012345678;client;Оплата по договору;1000.00\nC001;LAST;2026-09-29;45.00;Списание;0012345678;supplier;expense;955.00\n"
	importer, accepted, preview := stageImport(t, application, cfg, scope, body)
	if _, err := importer.Commit(t.Context(), imports.CommitCommand{Scope: scope, ActorID: "actor", Key: uuid.NewString(), ID: accepted.ID, ExpectedVersion: preview.Version, ExpectedDataVersion: 1, AcknowledgeWarnings: true, ViewDate: "2026-10-25"}); err != nil {
		t.Fatal(err)
	}
	model := forecastModelFunc(func(_ context.Context, input forecasting.FinancialModelInput) (forecasting.FinancialModelResult, error) {
		analysis := fixtureUnknownAnalysis(input)
		for i, operation := range input.Operations {
			if operation.Data().Direction == ledger.Inflow {
				analysis.Operations[i] = forecasting.OperationAnalysis{ID: operation.Data().ID, Version: operation.Version(), CategoryCode: "customer_payment", CategorySource: "rules", RuleID: "generic_customer_payment", CategoryID: forecasting.AnalysisCategory("customer_payment", ledger.Inflow, input.Categories)}
			}
		}
		projection := forecasting.ModelResult{SnapshotID: input.Snapshot.SnapshotID, Scope: input.Snapshot.Scope, DataVersion: input.Snapshot.DataVersion, Cutoff: input.Snapshot.Cutoff, Model: input.Snapshot.Model}
		cutoff, _ := time.Parse(time.DateOnly, string(input.Snapshot.Cutoff))
		for n := range 30 {
			projection.Days = append(projection.Days, forecasting.ModelForecastDay{Date: ledger.Date(cutoff.AddDate(0, 0, n+1).Format(time.DateOnly)), Inflow: rubModel(100), Outflow: rubModel(50), ReferenceBalance: rubModel(95500 + int64(n+1)*50)})
		}
		return forecasting.FinancialModelResult{Analysis: analysis, Projection: projection, ProbabilityUnavailableReason: "not_calibrated", Limitations: input.Assessment.Limitations}, nil
	})
	jobs := newForecastJobs(application.db, cfg.Postgres, model)
	baseID, err := jobs.Create(t.Context(), forecasting.CreateForecastCommand{Scope: scope, ActorID: "actor", IdempotencyKey: uuid.NewString(), Cutoff: "2026-09-29", Today: "2026-10-25", HorizonDays: 30})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jobs.ProcessOne(t.Context()); err != nil {
		t.Fatal(err)
	}
	base, err := jobs.Read(t.Context(), scope, baseID.ID)
	if err != nil || len(base.Sales) != 30 || base.Sales[0].AmountMinor != 100 {
		t.Fatal("missing certified sales basis", err)
	}
	before, _ := json.Marshal(base.Result)
	scenarios := newScenarios(application.db, cfg.Postgres, jobs)
	targets := []forecasting.SalesTarget{}
	for _, c := range base.Sales {
		targets = append(targets, forecasting.SalesTarget{ComponentID: c.ID, OccurrenceID: c.ID})
	}
	command := forecasting.ScenarioCommand{Scope: scope, ActorID: "actor", IdempotencyKey: uuid.NewString(), Name: "Sales -20%", BaseForecastID: base.ID, SalesChanges: []forecasting.SalesReductionChange{{ID: uuid.New(), Targets: targets, PeriodStart: base.Result.Days[0].Date, PeriodEnd: base.Result.Days[29].Date, ReductionBasisPoints: 2000}}}
	results := make([]forecasting.ScenarioView, 4)
	errs := make([]error, 4)
	var wg sync.WaitGroup
	for i := range results {
		wg.Go(func() { results[i], errs[i] = scenarios.Create(t.Context(), command) })
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil || results[i].Record.ID != results[0].Record.ID {
			t.Fatal("concurrent replay", i, err)
		}
	}
	record := results[0]
	restarted := newScenarios(application.db, cfg.Postgres, jobs)
	derived, err := restarted.Forecast(t.Context(), scope, record.Record.ForecastID)
	if err != nil || derived.Result.Days[29].Balance != 96400 || derived.Sales[0].RemainingMinor != 80 || len(derived.Result.Analysis.Operations) != 2 {
		t.Fatal("durable reduction", err)
	}
	command.Name = "changed"
	if _, err := scenarios.Create(t.Context(), command); !errors.Is(err, forecasting.ErrJobConflict) {
		t.Fatal("conflicting replay", err)
	}
	listed, err := restarted.List(t.Context(), forecasting.ScenarioListCommand{Scope: scope, ActorID: "actor", Limit: 100})
	if err != nil || len(listed.Items) != 1 || len(listed.Items[0].SalesChanges) != 1 {
		t.Fatal("schema2 list", err)
	}
	legacy, err := scenarios.Create(t.Context(), forecasting.ScenarioCommand{Scope: scope, ActorID: "actor", IdempotencyKey: uuid.NewString(), Name: "Legacy expense", BaseForecastID: base.ID, Changes: []forecasting.ExpenseChange{{ID: uuid.New(), Date: base.Result.Days[0].Date, AmountMinor: 10, Description: "expense"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.db.ExecContext(t.Context(), "UPDATE core_scenarios SET payload=payload-'SalesBasis'-'SalesChanges' WHERE company_id=$1 AND id=$2", scope.CompanyID, legacy.Record.ID); err != nil {
		t.Fatal(err)
	}
	if view, err := restarted.Forecast(t.Context(), scope, legacy.Record.ForecastID); err != nil || view.Result.Days[29].Balance != 96990 {
		t.Fatal("old schema1 unreadable", err)
	}
	after, err := jobs.Read(t.Context(), scope, base.ID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(after.Result)
	if string(encoded) != string(before) {
		t.Fatal("base forecast changed")
	}
	assertRevision(t, application, scope, 2)
	if _, err := application.db.ExecContext(t.Context(), "DELETE FROM core_scenarios WHERE company_id=$1", scope.CompanyID); err != nil {
		t.Fatal(err)
	}
}
