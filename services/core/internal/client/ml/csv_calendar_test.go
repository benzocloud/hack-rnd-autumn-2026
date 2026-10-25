package ml

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/ml/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
)

func csvCalendarFixture(t *testing.T) forecasting.FinancialModelInput {
	t.Helper()
	i := csvFixture(t)
	i.Snapshot.Model = forecasting.ModelContext{Name: "mean_90", Version: forecasting.CSVCalendarModelVersion, PreprocessingVersion: forecasting.CSVCalendarPreprocessing}
	i.Assessment.Limitations = []string{"csv_history_completeness_unverified", "unobserved_days_unknown", "observed_operations_mean_per_calendar_day", "historical_availability_unverified", "not_calibrated", "current_calendar_only"}
	cutoff, _ := time.Parse(time.DateOnly, string(i.Snapshot.Cutoff))
	group := domain.ReconciliationGroup{ID: uuid.New(), Scope: i.Snapshot.Scope, Version: 1, Status: "active", Cutoff: i.Snapshot.Cutoff, HorizonStart: ledger.Date(cutoff.AddDate(0, 0, 1).Format(time.DateOnly)), HorizonEnd: ledger.Date(cutoff.AddDate(0, 0, 30).Format(time.DateOnly)), ConfirmedBy: "actor", ConfirmedAt: time.Now().UTC(), Additional: true}
	p, err := payments.NewExpectedPayment(payments.PaymentData{ID: uuid.New(), Scope: i.Snapshot.Scope, ReconciliationGroupID: group.ID, Direction: ledger.Inflow, Amount: rub(900), ScheduledDate: ledger.Date(cutoff.AddDate(0, 0, 4).Format(time.DateOnly)), Source: ledger.Source{Kind: ledger.Manual}, CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	i.Calendar = domain.Calendar{Captured: true, KnowledgeMode: "current", AsOf: i.Snapshot.Cutoff, NoPostCutoffMovements: true, Groups: []domain.ReconciliationGroup{group}, Payments: []payments.ExpectedPayment{p}}
	return i
}
func csvCalendarResponse(t *testing.T, request *pb.PredictRequest) *pb.PredictResponse {
	t.Helper()
	s := request.Snapshot
	r := &pb.PredictResponse{SnapshotId: s.SnapshotId, CompanyId: s.CompanyId, AccountId: s.AccountId, DataVersion: s.DataVersion, Model: request.Model, Currency: "RUB", CutoffDate: s.CutoffDate, CalendarSnapshotId: s.SnapshotId, CalendarDataVersion: s.DataVersion, CsvHistory: request.CsvHistory, ClassifiedOperations: request.Operations, AvailabilityLimitations: request.AvailabilityLimitations, Warnings: []string{"CSV_HISTORY_COMPLETENESS_UNVERIFIED", "CURRENT_CALENDAR_INCLUDED", "UNCALIBRATED_NO_PROBABILITY"}, Assumptions: []string{"Supplied operations only; unknown days are not certified zeros."}, Uncertainty: &pb.ForecastUncertainty{Method: "unavailable_incomplete_history", GapProbabilityUnavailableReason: "not_calibrated"}, GapProbability: unavailableProbability()}
	cutoff, _ := time.Parse(time.DateOnly, s.CutoffDate)
	evidence := &pb.HistoryGroupEvidence{EvidenceId: "history", CategorySource: "unknown"}
	for _, op := range request.Operations {
		evidence.HistoricalOperationIds = append(evidence.HistoricalOperationIds, op.OperationId)
	}
	r.HistoryEvidence = []*pb.HistoryGroupEvidence{evidence}
	balance := s.GetInitialBalanceMinor()
	p := request.Calendar.Payments[0]
	for n := 0; n < 30; n++ {
		date := cutoff.AddDate(0, 0, n+1).Format(time.DateOnly)
		incoming := int64(100)
		r.Components = append(r.Components, &pb.ForecastComponent{ComponentId: fmt.Sprintf("residual:%d", n), Date: date, CategoryCode: "unknown", Kind: "mean_90_uncovered", SourceIds: []string{"history"}, InflowMinor: 100, OutflowMinor: 50})
		if date == p.EffectiveDate {
			incoming += 900
			r.Components = append(r.Components, &pb.ForecastComponent{ComponentId: "known:" + p.PaymentId, Date: date, CategoryCode: "unknown", Kind: "known_remaining_payment", SourceIds: []string{p.PaymentId}, InflowMinor: 900})
		}
		balance += incoming - 50
		r.Days = append(r.Days, &pb.ForecastDay{Date: date, InflowMinor: incoming, OutflowMinor: 50, ReferenceBalanceMinor: balance, NegativeBalanceProbability: unavailableProbability()})
	}
	r.StartDate = r.Days[0].Date
	return r
}
func unavailableProbability() *pb.FinancialProbability {
	return &pb.FinancialProbability{Estimate: &pb.FinancialProbability_UnavailableReason{UnavailableReason: "not_calibrated"}}
}
func TestCSVCalendarValidatesArithmeticEvidenceAndContext(t *testing.T) {
	i := csvCalendarFixture(t)
	request, err := encodeFinancial(i)
	if err != nil {
		t.Fatal(err)
	}
	if request.Snapshot.History[1].Source != pb.DaySource_DAY_SOURCE_UNAVAILABLE || request.Snapshot.History[1].InflowMinor != nil {
		t.Fatal("unknown CSV day was certified")
	}
	base := csvCalendarResponse(t, request)
	if err := validateFinancialResponse(i, request, base); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*pb.PredictResponse)
	}{
		{"wrong amount", func(r *pb.PredictResponse) { r.Days[3].InflowMinor++ }},
		{"wrong balance", func(r *pb.PredictResponse) { r.Days[4].ReferenceBalanceMinor++ }},
		{"double occurrence", func(r *pb.PredictResponse) { r.Components = append(r.Components, r.Components[4]) }},
		{"foreign evidence", func(r *pb.PredictResponse) { r.HistoryEvidence[0].HistoricalOperationIds[0] = uuid.NewString() }},
		{"invented replacement", func(r *pb.PredictResponse) {
			r.ReplacedHistoricalOperationIds = []string{request.Operations[0].OperationId}
		}},
		{"wrong model", func(r *pb.PredictResponse) { r.Model.ModelVersion = forecasting.CSVModelVersion }},
		{"fabricated bands", func(r *pb.PredictResponse) { r.Uncertainty.Bands = []*pb.EmpiricalBalanceBand{{Date: r.StartDate}} }},
		{"probability zero", func(r *pb.PredictResponse) {
			r.GapProbability = &pb.FinancialProbability{Estimate: &pb.FinancialProbability_Calibrated{Calibrated: &pb.CalibratedEstimate{Value: 0, CalibrationVersion: "invented", EvaluationReference: "not-supported"}}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, ok := proto.Clone(base).(*pb.PredictResponse)
			if !ok {
				t.Fatal("unexpected clone type")
			}
			tc.mutate(r)
			if validateFinancialResponse(i, request, r) == nil {
				t.Fatal("corrupt model response accepted")
			}
		})
	}
}
