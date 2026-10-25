//go:build integration

package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	importDomain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/imports"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/imports"
	ledgerService "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
)

func TestManualSupplementAfterImportKeepsSourceReplayAndForecast(t *testing.T) {
	application, cfg := testApplication(t)
	scope := newScope(t, application)
	body := "company_id;operation_id;operation_date;amount;direction;counterparty_inn;counterparty_name;payment_purpose;balance_eod\nC001;FIRST;2026-07-02;90.00;Поступление;0012345678;client;first;1000.00\nC001;LAST;2026-09-29;45.00;Списание;0012345678;supplier;last;955.00\n"
	service, accepted, preview := stageImport(t, application, cfg, scope, body)
	if _, err := service.Commit(t.Context(), imports.CommitCommand{Scope: scope, ActorID: "actor", Key: uuid.NewString(), ID: accepted.ID, ExpectedVersion: preview.Version, ExpectedDataVersion: 1, AcknowledgeWarnings: true, ViewDate: "2026-10-25"}); err != nil {
		t.Fatal(err)
	}
	manual := ledgerService.ManualTransactionCommand{Record: ledger.TransactionData{Scope: scope, Date: "2026-09-29", Direction: ledger.Outflow, Amount: rub(t, 4500), Source: ledger.Source{Kind: ledger.Manual}, CounterpartyINN: "0012345678", CounterpartyName: "supplier", Purpose: "last"}, ExpectedDataVersion: 2, ActorID: "actor", IdempotencyKey: uuid.NewString(), ViewDate: "2026-10-25"}
	if _, err := application.Ledger.CreateManualTransaction(t.Context(), manual); !errors.Is(err, ledgerService.ErrSuspectedDuplicate) {
		t.Fatal("imported duplicate accepted", err)
	}
	assertRevision(t, application, scope, 2)
	manual.Record.Date = "2026-09-30"
	manual.Record.Direction = ledger.Inflow
	manual.Record.Amount = rub(t, 180)
	manual.Record.CounterpartyName = "client"
	manual.Record.Purpose = "manual supplement"
	created, err := application.Ledger.CreateManualTransaction(t.Context(), manual)
	if err != nil || created.DataVersion != 3 || created.Transaction.Data().Source.Kind != ledger.Manual {
		t.Fatal(created, err)
	}
	replay, err := application.Ledger.CreateManualTransaction(t.Context(), manual)
	if err != nil || replay.Transaction.Data().ID != created.Transaction.Data().ID || replay.DataVersion != 3 {
		t.Fatal(replay, err)
	}
	assertCount(t, application, "SELECT count(*) FROM core_accounts WHERE company_id=$1 AND factual_kind='imported' AND source_system='dataset_v1' AND source_company_id='C001'", scope, 1)
	assertCount(t, application, "SELECT count(*) FROM core_transactions WHERE company_id=$1 AND source_kind='manual'", scope, 1)
	if _, err := application.Ledger.RecordManualBalance(t.Context(), ledgerService.ManualBalanceCommand{Scope: scope, Date: "2026-09-30", Amount: rub(t, 95680), Note: "Подтверждённый остаток", ExpectedDataVersion: 3, ActorID: "actor", IdempotencyKey: uuid.NewString(), ViewDate: "2026-10-25"}); err != nil {
		t.Fatal(err)
	}
	model := forecastModelFunc(func(_ context.Context, input forecasting.FinancialModelInput) (forecasting.FinancialModelResult, error) {
		if input.Assessment == nil || len(input.Operations) != 2 {
			t.Fatal(input)
		}
		manualFacts := 0
		for _, op := range input.Operations {
			if op.Data().Source.Kind == ledger.Manual {
				manualFacts++
			}
		}
		if manualFacts != 1 {
			t.Fatal("manual supplement missing from forecast")
		}
		projection := forecasting.ModelResult{SnapshotID: input.Snapshot.SnapshotID, Scope: scope, DataVersion: input.Snapshot.DataVersion, Cutoff: input.Snapshot.Cutoff, Model: input.Snapshot.Model}
		cutoff, _ := time.Parse(time.DateOnly, string(input.Snapshot.Cutoff))
		for n := 0; n < 30; n++ {
			projection.Days = append(projection.Days, forecasting.ModelForecastDay{Date: ledger.Date(cutoff.AddDate(0, 0, n+1).Format(time.DateOnly)), Inflow: rubModel(2), Outflow: rubModel(50), ReferenceBalance: rubModel(95680 - int64(n+1)*48)})
		}
		return forecasting.FinancialModelResult{Projection: projection, ProbabilityUnavailableReason: "not_calibrated", Limitations: input.Assessment.Limitations}, nil
	})
	jobs := newForecastJobs(application.db, cfg.Postgres, model)
	job, err := jobs.Create(t.Context(), forecasting.CreateForecastCommand{Scope: scope, ActorID: "actor", IdempotencyKey: uuid.NewString(), Cutoff: "2026-09-30", Today: "2026-10-25", HorizonDays: 30})
	if err != nil {
		t.Fatal(err)
	}
	if worked, err := jobs.ProcessOne(t.Context()); err != nil || !worked {
		t.Fatal(worked, err)
	}
	view, err := jobs.Read(t.Context(), scope, job.ID)
	if err != nil || view.Status != "succeeded" || view.Result.Days[29].Balance != 94240 {
		t.Fatal(view, err)
	}
	// A later bank file can contain the already manually entered movement.
	later := body + "C001;BANK-MANUAL;2026-09-30;1.80;Поступление;0012345678;client;manual supplement;956.80\n"
	service, accepted, preview = stageImport(t, application, cfg, scope, later, uuid.NewString())
	found := false
	for _, issue := range preview.Preview.Issues {
		if issue.Code == "SUSPECTED_CONTENT_DUPLICATE" && issue.Field == "manual_transaction" {
			found = true
		}
	}
	if !found || preview.Preview.Blockers != 0 {
		t.Fatal("manual/import duplicate missing from preview", preview)
	}
	cmd := imports.CommitCommand{Scope: scope, ActorID: "actor", Key: uuid.NewString(), ID: accepted.ID, ExpectedVersion: preview.Version, ExpectedDataVersion: 4, ViewDate: "2026-10-25"}
	if _, err := service.Commit(t.Context(), cmd); !errors.Is(err, importDomain.ErrWarnings) {
		t.Fatal("duplicate imported without acknowledgement", err)
	}
	cmd.AcknowledgeWarnings = true
	if _, err := service.Commit(t.Context(), cmd); err != nil {
		t.Fatal("explicit duplicate acknowledgement rejected", err)
	}
	assertCount(t, application, "SELECT count(*) FROM core_transactions WHERE company_id=$1", scope, 4)
	assertCount(t, application, "SELECT count(*) FROM core_balances WHERE company_id=$1 AND source_kind='manual'", scope, 1)
	assertCount(t, application, "SELECT count(*) FROM core_history_coverage WHERE company_id=$1", scope, 0)
	old, err := jobs.Read(t.Context(), scope, job.ID)
	if err != nil || old.CurrentDataVersion != 5 || old.Result.Days[29].Balance != 94240 {
		t.Fatal("old forecast changed", old, err)
	}
}
