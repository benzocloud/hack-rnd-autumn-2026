package ml

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/ml/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
)

func financialFixture(t *testing.T) forecasting.FinancialModelInput {
	t.Helper()
	i := inputFixture()
	i.Model = forecasting.ModelContext{Name: "mean_90", Version: financialVersion, PreprocessingVersion: financialPreprocessing}
	result := forecasting.FinancialModelInput{Snapshot: i, Coverage: forecasting.Coverage{Kind: "certified", From: i.History[0].Date, Through: i.Cutoff, Reference: "test-only certified complete fixture", DataVersion: i.DataVersion}}
	for _, day := range i.History {
		for _, direction := range []domain.Direction{domain.Inflow, domain.Outflow} {
			amount := day.Inflow
			if direction == domain.Outflow {
				amount = day.Outflow
			}
			record, err := domain.NewTransaction(domain.TransactionData{ID: uuid.New(), Scope: i.Scope, Date: day.Date, Direction: direction, Amount: amount, Source: domain.Source{Kind: domain.Manual}})
			if err != nil {
				t.Fatal(err)
			}
			result.Operations = append(result.Operations, record)
		}
	}
	return result
}

func financialResponse(input forecasting.FinancialModelInput) *pb.PredictResponse {
	req, err := encodeFinancial(input)
	if err != nil {
		panic(err)
	}
	r := responseFixture(input.Snapshot)
	r.Model = req.Model
	r.CalendarSnapshotId = req.Calendar.SnapshotId
	r.CalendarDataVersion = req.Calendar.DataVersion
	r.AvailabilityLimitations = req.AvailabilityLimitations
	r.ClassifiedOperations = req.Operations
	r.Warnings = append(r.Warnings, "UNCALIBRATED_EMPIRICAL_BANDS")
	r.Uncertainty = &pb.ForecastUncertainty{Method: "paired_moving_block_bootstrap_uncalibrated", Scenarios: 512, BlockDays: 7, Seed: 20261024, LowerQuantile: 0.1, UpperQuantile: 0.9, GapProbabilityUnavailableReason: "REQUIREMENT_OPEN_NO_CALIBRATED_GAP_PROBABILITY", Assumptions: []string{"synthetic empirical test"}}
	evidence := &pb.HistoryGroupEvidence{EvidenceId: "history:test", CategorySource: "unknown"}
	for _, op := range req.Operations {
		evidence.HistoricalOperationIds = append(evidence.HistoricalOperationIds, op.OperationId)
	}
	r.HistoryEvidence = []*pb.HistoryGroupEvidence{evidence}
	r.GapProbability = &pb.FinancialProbability{Estimate: &pb.FinancialProbability_UnavailableReason{UnavailableReason: "not_calibrated"}}
	for _, day := range r.Days {
		day.NegativeBalanceProbability = proto.Clone(r.GapProbability).(*pb.FinancialProbability)
		r.Uncertainty.Bands = append(r.Uncertainty.Bands, &pb.EmpiricalBalanceBand{Date: day.Date, LowerMinor: day.ReferenceBalanceMinor - 1, UpperMinor: day.ReferenceBalanceMinor + 1})
		r.Components = append(r.Components, &pb.ForecastComponent{ComponentId: uuid.NewString(), Date: day.Date, CategoryCode: "unknown", InflowMinor: 100, OutflowMinor: 200, Kind: "mean_90_uncovered", SourceIds: []string{"history:test"}})
	}
	return r
}

func TestFinancialPredictUsesSinglePinnedRPC(t *testing.T) {
	i := financialFixture(t)
	calls := 0
	i.Snapshot.History[0].Source = forecasting.ReconstructedCovered
	c := testClient(t, func(ctx context.Context, r *pb.PredictRequest) (*pb.PredictResponse, error) {
		calls++
		if r.Model.ModelVersion != financialVersion || r.Model.ConfigurationId != financialVersion || r.Calendar.KnowledgeMode != pb.CalendarKnowledgeMode_CALENDAR_KNOWLEDGE_MODE_UNAVAILABLE || r.Snapshot.History[0].Source != pb.DaySource_DAY_SOURCE_RECONSTRUCTED_COVERED {
			t.Error("wrong financial profile/provenance", r.Model, r.Calendar)
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Error("missing deadline")
		}
		return financialResponse(i), nil
	})
	r, err := c.PredictFinancial(t.Context(), i)
	if err != nil || calls != 1 || len(r.Projection.Days) != 30 || r.Projection.Days[29].ReferenceBalance.Minor() != -2700 || r.ProbabilityUnavailableReason != "not_calibrated" || len(r.Components) != 30 || len(r.Evidence) != 1 {
		t.Fatal(r, err, calls)
	}
}

func TestFinancialInputRejectsUnverifiedCoverageAndDrift(t *testing.T) {
	for name, edit := range map[string]func(*forecasting.FinancialModelInput){
		"no coverage":     func(i *forecasting.FinancialModelInput) { i.Coverage.Reference = "" },
		"wrong span":      func(i *forecasting.FinancialModelInput) { i.Coverage.From = i.Snapshot.Cutoff },
		"missing flows":   func(i *forecasting.FinancialModelInput) { i.Operations = nil },
		"duplicate facts": func(i *forecasting.FinancialModelInput) { i.Operations = append(i.Operations, i.Operations[0]) },
		"horizon":         func(i *forecasting.FinancialModelInput) { i.Snapshot.HorizonDays = 90 },
		"baseline model":  func(i *forecasting.FinancialModelInput) { i.Snapshot.Model.Version = "mean_90_v1" },
		"unobserved cutoff": func(i *forecasting.FinancialModelInput) {
			i.Snapshot.History[89].Source = forecasting.ReconstructedCovered
		},
		"unknown day":        func(i *forecasting.FinancialModelInput) { i.Snapshot.History[0].Source = "unknown" },
		"bad reconciliation": func(i *forecasting.FinancialModelInput) { i.Snapshot.History[0].ClosingBalance = rub(10) },
		"reconstructed rounding": func(i *forecasting.FinancialModelInput) {
			i.Snapshot.History[1].Source = forecasting.ReconstructedCovered
			i.Snapshot.History[1].ClosingBalance = rub(i.Snapshot.History[1].ClosingBalance.Minor() + 1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			i := financialFixture(t)
			edit(&i)
			called := false
			c := testClient(t, func(context.Context, *pb.PredictRequest) (*pb.PredictResponse, error) { called = true; return nil, nil })
			if _, err := c.PredictFinancial(t.Context(), i); err == nil || called {
				t.Fatal("invalid input reached ML", err, called)
			}
		})
	}
}

func TestFinancialResponseRejectsCorruption(t *testing.T) {
	for name, edit := range map[string]func(*pb.PredictResponse){
		"scope":        func(r *pb.PredictResponse) { r.CompanyId = uuid.NewString() },
		"version":      func(r *pb.PredictResponse) { r.DataVersion++ },
		"model":        func(r *pb.PredictResponse) { r.Model.ModelVersion = "mean_90_v1" },
		"calendar":     func(r *pb.PredictResponse) { r.CalendarSnapshotId = uuid.NewString() },
		"replaced ids": func(r *pb.PredictResponse) { r.ReplacedHistoricalOperationIds = []string{uuid.NewString()} },
		"probability": func(r *pb.PredictResponse) {
			r.GapProbability = &pb.FinancialProbability{Estimate: &pb.FinancialProbability_Calibrated{Calibrated: &pb.CalibratedEstimate{Value: math.NaN()}}}
		},
		"balance":        func(r *pb.PredictResponse) { r.Days[0].ReferenceBalanceMinor++ },
		"mean":           func(r *pb.PredictResponse) { r.Days[0].InflowMinor++ },
		"date":           func(r *pb.PredictResponse) { r.Days[0].Date = "2026-10-25" },
		"first gap":      func(r *pb.PredictResponse) { r.FirstNegativeBalanceDate = nil },
		"classification": func(r *pb.PredictResponse) { r.ClassifiedOperations[0].PaymentPurpose = "changed" },
		"evidence": func(r *pb.PredictResponse) {
			r.HistoryEvidence[0].HistoricalOperationIds = append(r.HistoryEvidence[0].HistoricalOperationIds, uuid.NewString())
		},
		"duplicate evidence":  func(r *pb.PredictResponse) { r.HistoryEvidence = append(r.HistoryEvidence, r.HistoryEvidence[0]) },
		"component totals":    func(r *pb.PredictResponse) { r.Components[0].InflowMinor++ },
		"component kind":      func(r *pb.PredictResponse) { r.Components[0].Kind = "known_payment" },
		"component source":    func(r *pb.PredictResponse) { r.Components[0].SourceIds = []string{"foreign"} },
		"duplicate component": func(r *pb.PredictResponse) { r.Components = append(r.Components, r.Components[0]) },
		"band":                func(r *pb.PredictResponse) { r.Uncertainty.Bands[0].UpperMinor = r.Uncertainty.Bands[0].LowerMinor - 1 },
		"calibrated claim":    func(r *pb.PredictResponse) { r.Uncertainty.Calibrated = true },
		"seed":                func(r *pb.PredictResponse) { r.Uncertainty.Seed++ },
		"limitation stripped": func(r *pb.PredictResponse) { r.AvailabilityLimitations = nil },
	} {
		t.Run(name, func(t *testing.T) {
			i := financialFixture(t)
			c := testClient(t, func(context.Context, *pb.PredictRequest) (*pb.PredictResponse, error) {
				r := financialResponse(i)
				edit(r)
				return r, nil
			})
			if _, err := c.PredictFinancial(t.Context(), i); !errors.Is(err, forecasting.ErrModelResponse) {
				t.Fatal(err)
			}
		})
	}
}

func TestFinancialFailureNeverFallsBack(t *testing.T) {
	i := financialFixture(t)
	calls := 0
	c := testClient(t, func(context.Context, *pb.PredictRequest) (*pb.PredictResponse, error) {
		calls++
		return nil, status.Error(codes.FailedPrecondition, "private unsupported model")
	})
	if _, err := c.PredictFinancial(t.Context(), i); !errors.Is(err, forecasting.ErrModelContext) || calls != 1 {
		t.Fatal(err, calls)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := c.PredictFinancial(ctx, i); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	c.timeout = time.Nanosecond
	if _, err := c.PredictFinancial(t.Context(), i); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
