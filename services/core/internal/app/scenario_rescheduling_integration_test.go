//go:build integration

package app

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	authClient "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/client/auth"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	forecastHandler "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/imports"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/grpcserver"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/mtls"
)

func movementModel(_ context.Context, input forecasting.FinancialModelInput) (forecasting.FinancialModelResult, error) {
	analysis := fixtureUnknownAnalysis(input)
	for i, op := range input.Operations {
		if op.Data().Direction == ledger.Inflow {
			analysis.Operations[i] = forecasting.OperationAnalysis{ID: op.Data().ID, Version: op.Version(), CategoryCode: "customer_payment", CategorySource: "rules", RuleID: "generic_customer_payment", CategoryID: forecasting.AnalysisCategory("customer_payment", ledger.Inflow, input.Categories)}
		}
	}
	projection := forecasting.ModelResult{SnapshotID: input.Snapshot.SnapshotID, Scope: input.Snapshot.Scope, DataVersion: input.Snapshot.DataVersion, Cutoff: input.Snapshot.Cutoff, Model: input.Snapshot.Model}
	cutoff, _ := time.Parse(time.DateOnly, string(input.Snapshot.Cutoff))
	for n := range 30 {
		projection.Days = append(projection.Days, forecasting.ModelForecastDay{Date: ledger.Date(cutoff.AddDate(0, 0, n+1).Format(time.DateOnly)), Inflow: rubModel(100), Outflow: rubModel(50), ReferenceBalance: rubModel(95500 + int64(n+1)*50)})
	}
	return forecasting.FinancialModelResult{Analysis: analysis, Projection: projection, ProbabilityUnavailableReason: "not_calibrated", Limitations: input.Assessment.Limitations}, nil
}

func TestScenarioDelayResponseRPCPersistenceAndParentComparison(t *testing.T) {
	f := newManualBalanceFixture(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	body := "company_id;operation_id;operation_date;amount;direction;counterparty_inn;counterparty_name;payment_purpose;balance_eod\nC001;FIRST;2026-07-02;90.00;Поступление;0012345678;client;Оплата по договору;1000.00\nC001;LAST;2026-09-29;45.00;Списание;0012345678;supplier;expense;955.00\n"
	importer, imported, preview := stageImport(t, f.app, cfg, f.scope, body)
	if _, err := importer.Commit(t.Context(), imports.CommitCommand{Scope: f.scope, ActorID: "actor", Key: uuid.NewString(), ID: imported.ID, ExpectedVersion: preview.Version, ExpectedDataVersion: 1, AcknowledgeWarnings: true, ViewDate: "2026-10-25"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
		defer cancel()
		for _, query := range []string{"DELETE FROM core_scenarios WHERE company_id=$1", "DELETE FROM core_transactions WHERE company_id=$1", "DELETE FROM core_imports WHERE company_id=$1"} {
			if _, err := f.app.db.ExecContext(ctx, query, f.scope.CompanyID); err != nil {
				t.Error(err)
			}
		}
	})
	jobs := newForecastJobs(f.app.db, cfg.Postgres, forecastModelFunc(movementModel))
	root, err := jobs.Create(t.Context(), forecasting.CreateForecastCommand{Scope: f.scope, ActorID: "actor", IdempotencyKey: uuid.NewString(), Cutoff: "2026-09-29", Today: "2026-10-25", HorizonDays: 30})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jobs.ProcessOne(t.Context()); err != nil {
		t.Fatal(err)
	}
	scenarios := newScenarios(f.app.db, cfg.Postgres, jobs)
	auth, err := authClient.New(f.authConnection, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	tls, err := mtls.Server(f.ca.Files(t, mtls.Core))
	if err != nil {
		t.Fatal(err)
	}
	server, err := grpcserver.New(credentials.NewTLS(tls))
	if err != nil {
		t.Fatal(err)
	}
	if err := forecastHandler.Register(server, jobs, f.app.Companies, auth, scenarios); err != nil {
		t.Fatal(err)
	}
	conn := testRPCConnection(t, f.ca, server, mtls.API, mtls.Core, "saldo-core")
	client := pb.NewScenariosServiceClient(conn)
	forecasts := pb.NewForecastingServiceClient(conn)
	base, err := forecasts.GetForecast(t.Context(), &pb.GetForecastRequest{Context: f.request.Context.Browser, Scope: f.request.Scope, ForecastId: root.ID.String()})
	if err != nil || !base.GetForecast().GetSucceeded().GetResult().GetScenarioCapabilities().GetDelayReceipt() {
		t.Fatal("delay unavailable", err)
	}
	sale := base.Forecast.GetSucceeded().Result.Components[0]
	expenseID := uuid.NewString()
	request := &pb.CreateScenarioRequest{Context: f.request.Context, Scope: f.request.Scope, IdempotencyKey: uuid.NewString(), Name: "Stress", Scenario: &pb.CreateScenarioRequest_Independent{Independent: &pb.IndependentScenario{BaseForecastId: root.ID.String(), Changes: []*pb.ScenarioChange{{Change: &pb.ScenarioChange_OneOffExpense{OneOffExpense: &pb.OneOffExpenseChange{Id: expenseID, Date: "2026-10-02", Amount: &pb.Money{AmountMinor: 100000, Currency: "RUB"}, Description: "Equipment"}}}, {Change: &pb.ScenarioChange_DelayReceipt{DelayReceipt: &pb.DelayReceiptChange{Id: uuid.NewString(), Target: &pb.OccurrenceTarget{ComponentId: sale.Id, OccurrenceId: sale.OccurrenceId}, Amount: &pb.Money{AmountMinor: 50, Currency: "RUB"}, NewDate: "2026-10-10"}}}}}}}
	parent, err := client.CreateScenario(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	parentForecast, err := forecasts.GetForecast(t.Context(), &pb.GetForecastRequest{Context: f.request.Context.Browser, Scope: f.request.Scope, ForecastId: parent.Accepted.ForecastId})
	if err != nil {
		t.Fatal(err)
	}
	parentResult := parentForecast.Forecast.GetSucceeded().Result
	if parentResult.Metrics.FirstGap == nil || parentResult.Metrics.ClosingBalance.AmountMinor != -3000 {
		t.Fatal("stress lost")
	}
	var target *pb.EffectiveOccurrence
	for _, c := range parentResult.EffectiveOccurrences {
		if c.ComponentId == expenseID {
			target = c
		}
	}
	if target == nil || !target.ResponseActionEligible || target.RemainingAmount.AmountMinor != 100000 {
		t.Fatal("expense response unavailable")
	}
	responseRequest := &pb.CreateScenarioRequest{Context: f.request.Context, Scope: f.request.Scope, IdempotencyKey: uuid.NewString(), Name: "Supplier deferral", Scenario: &pb.CreateScenarioRequest_Response{Response: &pb.ResponseScenario{ComparisonParentScenarioId: parent.Accepted.Id, ResponseAction: &pb.ReschedulePaymentPart{EffectiveOccurrenceId: target.EffectiveOccurrenceId, Amount: &pb.Money{AmountMinor: 100000, Currency: "RUB"}, NewDate: "2026-10-30", FeasibilityCondition: "Supplier agrees"}}}}
	response, err := client.CreateScenario(t.Context(), responseRequest)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := client.CreateScenario(t.Context(), responseRequest)
	if err != nil || !proto.Equal(replay, response) {
		t.Fatal("response replay", err)
	}
	saved, err := client.GetScenario(t.Context(), &pb.GetScenarioRequest{Context: f.request.Context.Browser, Scope: f.request.Scope, ScenarioId: response.Accepted.Id})
	if err != nil || saved.Scenario.GetComparisonParentScenarioId() != parent.Accepted.Id || len(saved.Scenario.Changes) != 2 || len(saved.Scenario.OutOfHorizonOccurrenceIds) != 1 {
		t.Fatal("stress inheritance or future liability", err)
	}
	compare := &pb.CompareForecastsRequest{Context: f.request.Context.Browser, Scope: f.request.Scope, CandidateForecastId: response.Accepted.ForecastId, ReferenceForecastId: parent.Accepted.ForecastId}
	comparison, err := forecasts.CompareForecasts(t.Context(), compare)
	if err != nil || comparison.Comparison.DeltaClosingBalance.AmountMinor != 100000 || comparison.Comparison.CandidateMetrics.FirstGap != nil {
		t.Fatal("parent comparison", err)
	}
	compare.ReferenceForecastId = root.ID.String()
	if _, err := forecasts.CompareForecasts(t.Context(), compare); status.Code(err) != codes.FailedPrecondition {
		t.Fatal("response compared with root", err)
	}
	changed := proto.Clone(responseRequest).(*pb.CreateScenarioRequest)
	changed.GetResponse().ResponseAction.FeasibilityCondition = "changed"
	if _, err := client.CreateScenario(t.Context(), changed); status.Code(err) != codes.AlreadyExists {
		t.Fatal("conflicting replay", err)
	}
	changed = proto.Clone(responseRequest).(*pb.CreateScenarioRequest)
	changed.IdempotencyKey = uuid.NewString()
	changed.GetResponse().ComparisonParentScenarioId = response.Accepted.Id
	if _, err := client.CreateScenario(t.Context(), changed); status.Code(err) != codes.InvalidArgument {
		t.Fatal("spent expense moved twice", err)
	}
	restarted := newScenarios(f.app.db, cfg.Postgres, jobs)
	fid, _ := uuid.Parse(response.Accepted.ForecastId)
	restored, err := restarted.Forecast(t.Context(), f.scope, fid)
	if err != nil || restored.Result.Days[29].Balance != 97000 || len(restored.Occurrences) != 33 {
		t.Fatal("restart split ledger", err)
	}
	again, err := forecasts.GetForecast(t.Context(), &pb.GetForecastRequest{Context: f.request.Context.Browser, Scope: f.request.Scope, ForecastId: parent.Accepted.ForecastId})
	if err != nil || !proto.Equal(again, parentForecast) {
		t.Fatal("parent mutated", err)
	}
	assertRevision(t, f.app, f.scope, 2)
}
