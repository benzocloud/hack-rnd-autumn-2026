package ml

import (
	"context"
	"errors"
	"math"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/ml/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
)

type testServer struct {
	pb.UnimplementedForecastServiceServer
	predict func(context.Context, *pb.PredictRequest) (*pb.PredictResponse, error)
}

func (s *testServer) Predict(ctx context.Context, r *pb.PredictRequest) (*pb.PredictResponse, error) {
	return s.predict(ctx, r)
}

func testClient(t *testing.T, handler func(context.Context, *pb.PredictRequest) (*pb.PredictResponse, error)) *client {
	t.Helper()
	listener := bufconn.Listen(2 * maxMessageSize)
	server := grpc.NewServer()
	pb.RegisterForecastServiceServer(server, &testServer{predict: handler})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient("passthrough:///ml-test", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	c, err := New(conn, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func rub(value int64) money.Money { m, _ := money.New(value, money.RUB); return m }
func inputFixture() forecasting.ModelInput {
	cutoff := time.Date(2026, time.October, 23, 0, 0, 0, 0, time.UTC)
	input := forecasting.ModelInput{SnapshotID: uuid.MustParse("019a1bd8-0000-7000-8000-000000000001"), Scope: ledger.Scope{CompanyID: uuid.MustParse("019a1bd8-0000-7000-8000-000000000002"), AccountID: uuid.MustParse("019a1bd8-0000-7000-8000-000000000003")}, DataVersion: 4, Cutoff: ledger.Date(cutoff.Format(time.DateOnly)), OpeningBalance: rub(300), HorizonDays: 30, Model: forecasting.ModelContext{Name: "mean_90", Version: "mean_90_v1", PreprocessingVersion: "daily_snapshot_v1"}}
	for i := range 90 {
		input.History = append(input.History, forecasting.ModelDay{Date: ledger.Date(cutoff.AddDate(0, 0, i-89).Format(time.DateOnly)), Inflow: rub(100), Outflow: rub(200), ClosingBalance: rub(int64(300 + (89-i)*100)), Source: forecasting.Observed})
	}
	return input
}
func responseFixture(input forecasting.ModelInput) *pb.PredictResponse {
	req := encodeInput(input)
	r := &pb.PredictResponse{SnapshotId: req.Snapshot.SnapshotId, CompanyId: req.Snapshot.CompanyId, AccountId: req.Snapshot.AccountId, DataVersion: req.Snapshot.DataVersion, Currency: money.RUB, CutoffDate: string(input.Cutoff), Model: req.Model, HistoryOnlyIncludesRecurringFlows: true, Warnings: []string{"HISTORY_ONLY_NO_CALENDAR", "UNCALIBRATED_NO_PROBABILITY"}}
	cutoff, _ := time.Parse(time.DateOnly, string(input.Cutoff))
	balance := input.OpeningBalance.Minor()
	for i := range input.HorizonDays {
		date := cutoff.AddDate(0, 0, i+1).Format(time.DateOnly)
		balance -= 100
		r.Days = append(r.Days, &pb.ForecastDay{Date: date, InflowMinor: 100, OutflowMinor: 200, ReferenceBalanceMinor: balance})
		if balance < 0 && r.FirstNegativeBalanceDate == nil {
			r.FirstNegativeBalanceDate = &date
		}
	}
	r.StartDate = r.Days[0].Date
	return r
}
func TestPredict(t *testing.T) {
	input := inputFixture()
	c := testClient(t, func(ctx context.Context, r *pb.PredictRequest) (*pb.PredictResponse, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > time.Second {
			t.Error("missing bounded deadline")
		}
		if !proto.Equal(r, encodeInput(input)) {
			t.Error("request differs from verified snapshot")
		}
		return responseFixture(input), nil
	})
	result, err := c.Predict(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Days) != 30 || result.Days[0].ReferenceBalance.Minor() != 200 || result.FirstNegativeDate == nil || *result.FirstNegativeDate != "2026-10-27" || result.DataVersion != 4 {
		t.Fatalf("unexpected result: %+v", result)
	}
}
func TestRejectInputBeforeRPC(t *testing.T) {
	tests := map[string]func(*forecasting.ModelInput){
		"missing id":       func(i *forecasting.ModelInput) { i.SnapshotID = uuid.Nil },
		"scope":            func(i *forecasting.ModelInput) { i.Scope.AccountID = uuid.Nil },
		"version":          func(i *forecasting.ModelInput) { i.DataVersion = 0 },
		"horizon":          func(i *forecasting.ModelInput) { i.HorizonDays = 31 },
		"history length":   func(i *forecasting.ModelInput) { i.History = i.History[1:] },
		"gap":              func(i *forecasting.ModelInput) { i.History[2].Date = i.History[1].Date },
		"unknown coverage": func(i *forecasting.ModelInput) { i.History[0].Source = "unknown" },
		"negative flow":    func(i *forecasting.ModelInput) { i.History[1].Inflow = rub(-1) },
		"invalid money":    func(i *forecasting.ModelInput) { i.History[1].Inflow = money.Money{} },
		"unverified zero":  func(i *forecasting.ModelInput) { i.History[0].Source = forecasting.VerifiedNoOperations },
		"balance mismatch": func(i *forecasting.ModelInput) { i.History[1].ClosingBalance = rub(100) },
		"opening mismatch": func(i *forecasting.ModelInput) { i.OpeningBalance = rub(200) },
		"model":            func(i *forecasting.ModelInput) { i.Model.Name = "" },
		"calendar":         func(i *forecasting.ModelInput) { i.KnownPayments = []payments.ExpectedPayment{{}} },
		"oversized":        func(i *forecasting.ModelInput) { i.Model.Version = strings.Repeat("x", maxMessageSize) },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			i := inputFixture()
			change(&i)
			c := testClient(t, func(context.Context, *pb.PredictRequest) (*pb.PredictResponse, error) {
				t.Error("invalid input reached RPC")
				return nil, nil
			})
			result, err := c.Predict(t.Context(), i)
			want := forecasting.ErrModelInput
			if name == "calendar" {
				want = forecasting.ErrModelContext
			}
			if !errors.Is(err, want) || !reflect.DeepEqual(result, forecasting.ModelResult{}) {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}
func TestRejectResponse(t *testing.T) {
	tests := map[string]func(*pb.PredictResponse){
		"snapshot":       func(r *pb.PredictResponse) { r.SnapshotId = uuid.NewString() },
		"company":        func(r *pb.PredictResponse) { r.CompanyId = uuid.NewString() },
		"account":        func(r *pb.PredictResponse) { r.AccountId = uuid.NewString() },
		"version":        func(r *pb.PredictResponse) { r.DataVersion++ },
		"currency":       func(r *pb.PredictResponse) { r.Currency = "USD" },
		"cutoff":         func(r *pb.PredictResponse) { r.CutoffDate = "2026-10-22" },
		"model":          func(r *pb.PredictResponse) { r.Model.ModelVersion = "other" },
		"missing model":  func(r *pb.PredictResponse) { r.Model = nil },
		"length":         func(r *pb.PredictResponse) { r.Days = r.Days[:29] },
		"nil day":        func(r *pb.PredictResponse) { r.Days[0] = nil },
		"start":          func(r *pb.PredictResponse) { r.StartDate = "2026-10-23" },
		"gap":            func(r *pb.PredictResponse) { r.Days[1].Date = r.Days[0].Date },
		"negative flow":  func(r *pb.PredictResponse) { r.Days[0].InflowMinor = -1 },
		"arithmetic":     func(r *pb.PredictResponse) { r.Days[0].ReferenceBalanceMinor++ },
		"overflow":       func(r *pb.PredictResponse) { r.Days[0].InflowMinor = math.MaxInt64 },
		"first negative": func(r *pb.PredictResponse) { r.FirstNegativeBalanceDate = nil },
		"history mode":   func(r *pb.PredictResponse) { r.HistoryOnlyIncludesRecurringFlows = false },
		"warnings":       func(r *pb.PredictResponse) { r.Warnings = nil },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			input := inputFixture()
			response := responseFixture(input)
			change(response)
			c := testClient(t, func(context.Context, *pb.PredictRequest) (*pb.PredictResponse, error) { return response, nil })
			result, err := c.Predict(t.Context(), input)
			if !errors.Is(err, forecasting.ErrModelResponse) || !reflect.DeepEqual(result, forecasting.ModelResult{}) {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
	if err := validateResponse(inputFixture(), nil); !errors.Is(err, forecasting.ErrModelResponse) {
		t.Fatal(err)
	}
}
func TestStatusMapping(t *testing.T) {
	tests := []struct {
		code codes.Code
		want error
	}{{codes.InvalidArgument, forecasting.ErrModelInput}, {codes.FailedPrecondition, forecasting.ErrModelContext}, {codes.ResourceExhausted, forecasting.ErrModelBusy}, {codes.Unavailable, forecasting.ErrModelUnavailable}, {codes.Internal, forecasting.ErrModelUnavailable}, {codes.Canceled, context.Canceled}, {codes.DeadlineExceeded, context.DeadlineExceeded}}
	for _, tt := range tests {
		t.Run(tt.code.String(), func(t *testing.T) {
			c := testClient(t, func(context.Context, *pb.PredictRequest) (*pb.PredictResponse, error) {
				return nil, status.Error(tt.code, "private financial diagnostics")
			})
			_, err := c.Predict(t.Context(), inputFixture())
			if !errors.Is(err, tt.want) || strings.Contains(err.Error(), "private") {
				t.Fatal(err)
			}
			var wrapped *modelError
			if !errors.As(err, &wrapped) || status.Code(wrapped.cause) != tt.code {
				t.Fatal("lost original status")
			}
		})
	}
}
func TestCancellationAndParentDeadline(t *testing.T) {
	c := testClient(t, func(ctx context.Context, _ *pb.PredictRequest) (*pb.PredictResponse, error) {
		<-ctx.Done()
		return nil, status.FromContextError(ctx.Err()).Err()
	})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := c.Predict(ctx, inputFixture()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	ctx, cancel = context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if _, err := c.Predict(ctx, inputFixture()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
func TestHistoryIntegerBoundaries(t *testing.T) {
	for _, tt := range []struct {
		previous, inflow, outflow, actual int64
		source                            forecasting.HistorySource
		want                              bool
	}{
		{math.MaxInt64, 1, 0, math.MaxInt64, forecasting.Observed, true},
		{math.MinInt64, 0, 1, math.MinInt64, forecasting.Observed, true},
		{math.MaxInt64, 2, 0, math.MaxInt64, forecasting.Observed, false},
		{math.MaxInt64, 0, 0, math.MaxInt64, forecasting.VerifiedNoOperations, true},
		{math.MaxInt64, 0, 0, math.MaxInt64 - 1, forecasting.VerifiedNoOperations, false},
	} {
		day := forecasting.ModelDay{Inflow: rub(tt.inflow), Outflow: rub(tt.outflow), ClosingBalance: rub(tt.actual), Source: tt.source}
		if got := historyBalanceMatches(tt.previous, day); got != tt.want {
			t.Fatalf("%+v: %v", tt, got)
		}
	}
	result, err := balanceAfter(rub(math.MaxInt64), 1, 1)
	if err != nil || result.Minor() != math.MaxInt64 {
		t.Fatal(result, err)
	}
}
func TestNew(t *testing.T) {
	if _, err := New(nil, time.Second); !errors.Is(err, forecasting.ErrModelInput) {
		t.Fatal(err)
	}
	conn, err := grpc.NewClient("passthrough:///unused", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for _, timeout := range []time.Duration{0, -1, 31 * time.Second} {
		if _, err := New(conn, timeout); !errors.Is(err, forecasting.ErrModelInput) {
			t.Fatal(err)
		}
	}
}

func TestReconciliationWarning(t *testing.T) {
	input := inputFixture()
	input.History[0].ClosingBalance = rub(input.History[0].ClosingBalance.Minor() + 1)
	if err := validateInput(input); err != nil {
		t.Fatal(err)
	}
	response := responseFixture(input)
	if err := validateResponse(input, response); !errors.Is(err, forecasting.ErrModelResponse) {
		t.Fatal("missing reconciliation warning accepted", err)
	}
	response.Warnings = append(response.Warnings, "OBSERVED_RECONCILIATION_WITHIN_ONE_MINOR_UNIT")
	if err := validateResponse(input, response); err != nil {
		t.Fatal(err)
	}
}

func TestOptionalZeroAndVerifiedCoverage(t *testing.T) {
	input := inputFixture()
	for i := range input.History {
		input.History[i].Inflow = rub(0)
		input.History[i].Outflow = rub(0)
		input.History[i].ClosingBalance = input.OpeningBalance
		input.History[i].Source = forecasting.VerifiedNoOperations
	}
	input.History[89].Source = forecasting.Observed
	if err := validateInput(input); err != nil {
		t.Fatal(err)
	}
	request := encodeInput(input)
	for _, day := range request.Snapshot.History {
		if day.InflowMinor == nil || day.OutflowMinor == nil || day.ClosingBalanceMinor == nil || *day.InflowMinor != 0 || *day.OutflowMinor != 0 {
			t.Fatal("explicit zero lost presence")
		}
	}
	input.History[89].Source = forecasting.VerifiedNoOperations
	if err := validateInput(input); !errors.Is(err, forecasting.ErrModelInput) {
		t.Fatal("cutoff without observed EOD accepted")
	}
}
