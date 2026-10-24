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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	authClient "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/client/auth"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	forecastHandler "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/grpcserver"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/mtls"
)

type forecastModelFunc func(context.Context, forecasting.FinancialModelInput) (forecasting.FinancialModelResult, error)

func (f forecastModelFunc) PredictFinancial(ctx context.Context, input forecasting.FinancialModelInput) (forecasting.FinancialModelResult, error) {
	return f(ctx, input)
}

func deterministicForecast(_ context.Context, input forecasting.FinancialModelInput) (forecasting.FinancialModelResult, error) {
	i := input.Snapshot
	cutoff, _ := time.Parse(time.DateOnly, string(i.Cutoff))
	balance := i.OpeningBalance.Minor()
	projection := forecasting.ModelResult{SnapshotID: i.SnapshotID, Scope: i.Scope, DataVersion: i.DataVersion, Cutoff: i.Cutoff, Model: i.Model}
	for n := range 30 {
		balance -= 100
		date := domain.Date(cutoff.AddDate(0, 0, n+1).Format(time.DateOnly))
		projection.Days = append(projection.Days, forecasting.ModelForecastDay{Date: date, Inflow: rubModel(100), Outflow: rubModel(200), ReferenceBalance: rubModel(balance)})
		if balance < 0 && projection.FirstNegativeDate == nil {
			projection.FirstNegativeDate = &date
		}
	}
	return forecasting.FinancialModelResult{Projection: projection, ProbabilityUnavailableReason: "not_calibrated", Limitations: []string{"historical_calendar_unavailable"}}, nil
}

func rubModel(n int64) money.Money { amount, _ := money.New(n, money.RUB); return amount }

func seedForecastHistory(t *testing.T, f *manualBalanceFixture, coverage bool) (domain.Date, int64) {
	t.Helper()
	today, err := time.Parse(time.DateOnly, string(f.today))
	if err != nil {
		t.Fatal(err)
	}
	cutoff := today.AddDate(0, 0, -1)
	revision := int64(1)
	for day := range 90 {
		date := domain.Date(cutoff.AddDate(0, 0, day-89).Format(time.DateOnly))
		for _, direction := range []domain.Direction{domain.Inflow, domain.Outflow} {
			amount := int64(100)
			if direction == domain.Outflow {
				amount = 200
			}
			result, err := f.app.Ledger.CreateTransaction(t.Context(), ledger.TransactionCommand{Record: domain.TransactionData{Scope: f.scope, Date: date, Direction: direction, Amount: rub(t, amount), Source: domain.Source{Kind: domain.Manual}}, ExpectedDataVersion: revision, ActorID: "auth-owned-user", IdempotencyKey: uuid.NewString()})
			if err != nil {
				t.Fatal(err)
			}
			revision = result.DataVersion
		}
	}
	result, err := f.app.Ledger.RecordBalance(t.Context(), ledger.BalanceCommand{Scope: f.scope, Date: domain.Date(cutoff.Format(time.DateOnly)), Amount: rub(t, 300), Source: domain.BalanceSource{Kind: domain.ManualEOD}, Note: "observed cutoff EOD", ExpectedDataVersion: revision, ActorID: "auth-owned-user", IdempotencyKey: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	revision = result.DataVersion
	if coverage {
		if _, err := f.app.db.ExecContext(t.Context(), "INSERT INTO core_history_coverage(company_id,account_id,data_version,from_date,through_date,reference,kind) VALUES($1,$2,$3,$4,$5,$6,'certified')", f.scope.CompanyID, f.scope.AccountID, revision, cutoff.AddDate(0, 0, -89).Format(time.DateOnly), cutoff.Format(time.DateOnly), "complete synthetic integration fixture, 180 known operations"); err != nil {
			t.Fatal(err)
		}
	}
	return domain.Date(cutoff.Format(time.DateOnly)), revision
}

func forecastRPC(t *testing.T, f *manualBalanceFixture, jobs forecastJobs, identity string) pb.ForecastingServiceClient {
	t.Helper()
	auth, err := authClient.New(f.authConnection, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	tlsConfig, err := mtls.Server(f.ca.Files(t, mtls.Core))
	if err != nil {
		t.Fatal(err)
	}
	server, err := grpcserver.New(credentials.NewTLS(tlsConfig))
	if err != nil {
		t.Fatal(err)
	}
	if err := forecastHandler.Register(server, jobs, f.app.Companies, auth); err != nil {
		t.Fatal(err)
	}
	return pb.NewForecastingServiceClient(testRPCConnection(t, f.ca, server, identity, mtls.Core, "saldo-core"))
}

func createForecastRequest(f *manualBalanceFixture, cutoff domain.Date) *pb.CreateForecastRequest {
	return &pb.CreateForecastRequest{Context: f.request.Context, Scope: f.request.Scope, IdempotencyKey: uuid.NewString(), CutoffDate: string(cutoff), HorizonDays: 30}
}

func TestForecastRPCDurableLifecycleReplayAndStale(t *testing.T) {
	f := newManualBalanceFixture(t)
	cutoff, revision := seedForecastHistory(t, f, true)
	// With just one connection, this independent SQL query would time out if
	// Predict were called inside the claim transaction.
	f.app.db.SetMaxOpenConns(1)
	model := forecastModelFunc(func(ctx context.Context, input forecasting.FinancialModelInput) (forecasting.FinancialModelResult, error) {
		var one int
		if err := f.app.db.GetContext(ctx, &one, "SELECT 1"); err != nil {
			return forecasting.FinancialModelResult{}, err
		}
		return deterministicForecast(ctx, input)
	})
	jobs := newForecastJobs(f.app.db, config.Postgres{QueryTimeout: 5 * time.Second}, model)
	client := forecastRPC(t, f, jobs, mtls.API)
	request := createForecastRequest(f, cutoff)
	results := make([]*pb.CreateForecastResponse, 8)
	errs := make([]error, 8)
	var wg sync.WaitGroup
	for n := range results {
		wg.Go(func() { results[n], errs[n] = client.CreateForecast(t.Context(), request) })
	}
	wg.Wait()
	for n, err := range errs {
		if err != nil || !proto.Equal(results[0], results[n]) {
			t.Fatal(n, err)
		}
	}
	id := results[0].Accepted.Id
	if results[0].Accepted.Status != pb.ForecastStatus_FORECAST_STATUS_QUEUED || results[0].Accepted.PollAfterMs < 1000 {
		t.Fatal(results[0])
	}
	assertCount(t, f.app, "SELECT count(*) FROM core_forecast_jobs WHERE company_id=$1", f.scope, 1)
	get := &pb.GetForecastRequest{Context: request.Context.Browser, Scope: request.Scope, ForecastId: id}
	pending, err := client.GetForecast(t.Context(), get)
	if err != nil || pending.Forecast.GetPending().GetStatus() != pb.ForecastStatus_FORECAST_STATUS_QUEUED || pending.Forecast.Input.DataVersion != revision {
		t.Fatal(pending, err)
	}
	// A different application instance starts the worker from persisted input.
	second, cfg := testApplication(t)
	restarted := newForecastJobs(second.db, cfg.Postgres, model)
	worked, err := restarted.ProcessOne(t.Context())
	if err != nil || !worked {
		t.Fatal(worked, err)
	}
	ready, err := client.GetForecast(t.Context(), get)
	if err != nil || ready.Forecast.GetSucceeded() == nil || len(ready.Forecast.GetSucceeded().Result.DailyPoints) != 30 || ready.Forecast.GetSucceeded().Result.Metrics.ClosingBalance.AmountMinor != -2700 || ready.Forecast.GetSucceeded().Result.Metrics.GapProbability.GetUnavailableReason() != "not_calibrated" {
		t.Fatal(ready, err)
	}
	assertForecastCorruptionRejected(t, f, client, get)
	replayed, err := client.CreateForecast(t.Context(), request)
	if err != nil || !proto.Equal(results[0], replayed) {
		t.Fatal("completed job changed frozen acceptance", replayed, err)
	}
	if _, err := f.app.Ledger.RecordBalance(t.Context(), ledger.BalanceCommand{Scope: f.scope, Date: f.today, Amount: rub(t, 9000), Source: domain.BalanceSource{Kind: domain.ManualEOD}, Note: "new EOD", ExpectedDataVersion: revision, ActorID: "auth-owned-user", IdempotencyKey: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	stale, err := client.GetForecast(t.Context(), get)
	if err != nil || !stale.Forecast.IsStale || !proto.Equal(ready.Forecast.GetSucceeded(), stale.Forecast.GetSucceeded()) || stale.Forecast.CurrentDataVersion != revision+1 {
		t.Fatal(stale, err)
	}
	for _, mode := range []string{"body", "CSRF", "revoked", "account"} {
		r := proto.Clone(request).(*pb.CreateForecastRequest)
		want := codes.PermissionDenied
		switch mode {
		case "body":
			r.CutoffDate = string(f.today)
			want = codes.AlreadyExists
		case "CSRF":
			r.Context.CsrfToken = "invalid"
		case "revoked":
			r.Context.Browser.CookieToken = "revoked-session"
		case "account":
			r.Scope.AccountId = uuid.NewString()
			want = codes.NotFound
		}
		if _, err := client.CreateForecast(t.Context(), r); status.Code(err) != want {
			t.Fatal(mode, err)
		}
	}
	if _, err := forecastRPC(t, f, jobs, mtls.Auth).GetForecast(t.Context(), get); status.Code(err) != codes.PermissionDenied {
		t.Fatal("Auth peer read forecast", err)
	}
	list, err := client.ListForecasts(t.Context(), &pb.ListForecastsRequest{Context: request.Context.Browser, Scope: request.Scope})
	if err != nil || list.Pagination == nil || len(list.Items) != 1 || list.Items[0].Id != id || !list.Items[0].IsStale {
		t.Fatal(list, err)
	}
	bad := proto.Clone(get).(*pb.GetForecastRequest)
	bad.Scope.AccountId = uuid.NewString()
	if _, err := client.GetForecast(t.Context(), bad); status.Code(err) != codes.NotFound {
		t.Fatal(err)
	}
}

func assertForecastCorruptionRejected(t *testing.T, f *manualBalanceFixture, client pb.ForecastingServiceClient, get *pb.GetForecastRequest) {
	t.Helper()
	var original []byte
	if err := f.app.db.GetContext(t.Context(), &original, "SELECT result FROM core_forecast_jobs WHERE company_id=$1 AND id=$2", f.scope.CompanyID, get.ForecastId); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"schema", "arithmetic", "probability"} {
		var result forecasting.SavedForecast
		if err := json.Unmarshal(original, &result); err != nil {
			t.Fatal(err)
		}
		switch mode {
		case "schema":
			result.SchemaVersion++
		case "arithmetic":
			result.Days[0].Balance++
		case "probability":
			result.ProbabilityReason = "fabricated_zero"
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.app.db.ExecContext(t.Context(), "UPDATE core_forecast_jobs SET result=$3::jsonb WHERE company_id=$1 AND id=$2", f.scope.CompanyID, get.ForecastId, encoded); err != nil {
			t.Fatal(err)
		}
		if _, err := client.GetForecast(t.Context(), get); status.Code(err) != codes.Unavailable {
			t.Fatal("corrupted forecast accepted", mode, err)
		}
	}
	if _, err := f.app.db.ExecContext(t.Context(), "UPDATE core_forecast_jobs SET result=$3::jsonb WHERE company_id=$1 AND id=$2", f.scope.CompanyID, get.ForecastId, original); err != nil {
		t.Fatal(err)
	}
}

func TestForecastCoverageAndFailureStates(t *testing.T) {
	f := newManualBalanceFixture(t)
	cutoff, _ := seedForecastHistory(t, f, false)
	model := forecastModelFunc(func(context.Context, forecasting.FinancialModelInput) (forecasting.FinancialModelResult, error) {
		return forecasting.FinancialModelResult{}, errors.New("private upstream diagnostic")
	})
	jobs := newForecastJobs(f.app.db, config.Postgres{QueryTimeout: 5 * time.Second}, model)
	client := forecastRPC(t, f, jobs, mtls.API)
	request := createForecastRequest(f, cutoff)
	if _, err := client.CreateForecast(t.Context(), request); status.Code(err) != codes.FailedPrecondition {
		t.Fatal("uncertified facts accepted", err)
	}
	assertCount(t, f.app, "SELECT count(*) FROM core_forecast_jobs WHERE company_id=$1", f.scope, 0)
	date, _ := time.Parse(time.DateOnly, string(cutoff))
	var revision int64
	if err := f.app.db.GetContext(t.Context(), &revision, "SELECT data_version FROM core_accounts WHERE company_id=$1", f.scope.CompanyID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.db.ExecContext(t.Context(), "INSERT INTO core_history_coverage(company_id,account_id,data_version,from_date,through_date,reference,kind) VALUES($1,$2,$3,$4,$5,'test complete fixture','certified')", f.scope.CompanyID, f.scope.AccountID, revision, date.AddDate(0, 0, -89).Format(time.DateOnly), cutoff); err != nil {
		t.Fatal(err)
	}
	request.HorizonDays = 90
	if _, err := client.CreateForecast(t.Context(), request); status.Code(err) != codes.FailedPrecondition {
		t.Fatal("horizon truncated", err)
	}
	request.HorizonDays = 30
	accepted, err := client.CreateForecast(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if worked, err := jobs.ProcessOne(t.Context()); err != nil || !worked {
		t.Fatal(worked, err)
	}
	result, err := client.GetForecast(t.Context(), &pb.GetForecastRequest{Context: request.Context.Browser, Scope: request.Scope, ForecastId: accepted.Accepted.Id})
	if err != nil || result.Forecast.GetFailed() == nil || result.Forecast.GetFailed().Error.Code != "DEPENDENCY_UNAVAILABLE" || result.Forecast.GetFailed().Error.RequestId == "" {
		t.Fatal(result, err)
	}
	if _, err := jobs.ProcessOne(t.Context()); err != nil {
		t.Fatal(err)
	}
	limit := uint32(1)
	second := proto.Clone(request).(*pb.CreateForecastRequest)
	second.IdempotencyKey = uuid.NewString()
	if _, err := client.CreateForecast(t.Context(), second); err != nil {
		t.Fatal(err)
	}
	page, err := client.ListForecasts(t.Context(), &pb.ListForecastsRequest{Context: request.Context.Browser, Scope: request.Scope, Page: &pb.PageRequest{Limit: &limit}})
	if err != nil || len(page.Items) != 1 || page.Pagination.NextCursor == "" {
		t.Fatal(page, err)
	}
	next, err := client.ListForecasts(t.Context(), &pb.ListForecastsRequest{Context: request.Context.Browser, Scope: request.Scope, Page: &pb.PageRequest{Cursor: page.Pagination.NextCursor, Limit: &limit}})
	if err != nil || len(next.Items) != 1 || next.Items[0].Id == page.Items[0].Id {
		t.Fatal(next, err)
	}
	if _, err := client.ListForecasts(t.Context(), &pb.ListForecastsRequest{Context: request.Context.Browser, Scope: request.Scope, IncludeScenarios: true, Page: &pb.PageRequest{Cursor: page.Pagination.NextCursor, Limit: &limit}}); status.Code(err) != codes.InvalidArgument {
		t.Fatal("cursor filter binding lost", err)
	}
}
