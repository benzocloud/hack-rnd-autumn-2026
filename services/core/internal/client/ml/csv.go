package ml

import (
	"math/big"
	"slices"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/ml/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
)

func encodeCSV(input forecasting.FinancialModelInput) (*pb.PredictRequest, error) {
	i, a := input.Snapshot, input.Assessment
	if a == nil || a.RuleVersion != forecasting.CSVQualityVersion || a.Scope != i.Scope || a.DataVersion != i.DataVersion || a.Through != i.Cutoff || i.Model != (forecasting.ModelContext{Name: "mean_90", Version: forecasting.CSVModelVersion, PreprocessingVersion: forecasting.CSVPreprocessing}) || i.HorizonDays != 30 || len(i.History) != 0 || len(i.KnownPayments) != 0 || len(input.Operations) == 0 || len(input.Operations) > 20000 || input.Coverage.Kind != "" {
		return nil, forecasting.ErrModelInput
	}
	cutoff, err := time.Parse(time.DateOnly, string(i.Cutoff))
	if err != nil || a.From != ledger.Date(cutoff.AddDate(0, 0, -89).Format(time.DateOnly)) {
		return nil, forecasting.ErrModelInput
	}
	request := &pb.PredictRequest{Snapshot: &pb.ForecastSnapshot{SnapshotId: i.SnapshotID.String(), CompanyId: i.Scope.CompanyID.String(), AccountId: i.Scope.AccountID.String(), DataVersion: uint64(i.DataVersion), Currency: "RUB", CutoffDate: string(i.Cutoff), InitialBalanceMinor: new(i.OpeningBalance.Minor())}, HorizonDays: 30, Model: &pb.ModelContext{ModelName: i.Model.Name, ModelVersion: i.Model.Version, PreprocessingVersion: i.Model.PreprocessingVersion, ConfigurationId: i.Model.Version}, Calendar: &pb.FinancialCalendar{SnapshotId: i.SnapshotID.String(), DataVersion: uint64(i.DataVersion), KnowledgeMode: pb.CalendarKnowledgeMode_CALENDAR_KNOWLEDGE_MODE_UNAVAILABLE}, CsvHistory: &pb.CsvHistoryContext{RuleVersion: a.RuleVersion, FromDate: string(a.From), ThroughDate: string(a.Through), DataVersion: uint64(a.DataVersion), FirstOperationDate: string(a.FirstOperationDate)}, AvailabilityLimitations: slices.Clone(a.Limitations)}
	flows := make(map[ledger.Date][2]*big.Int)
	seen := make(map[uuid.UUID]bool)
	for _, record := range input.Operations {
		d := record.Data()
		if _, err := ledger.RestoreTransaction(d, record.Version()); err != nil || seen[d.ID] || d.Scope != i.Scope || d.Date < a.From || d.Date > i.Cutoff || (d.Source.Kind != ledger.Imported && d.Source.Kind != ledger.Manual) {
			return nil, forecasting.ErrModelInput
		}
		seen[d.ID] = true
		v, ok := flows[d.Date]
		if !ok {
			v = [2]*big.Int{new(big.Int), new(big.Int)}
		}
		op := &pb.FinancialOperation{OperationId: d.ID.String(), Date: string(d.Date), Version: uint64(record.Version()), CategoryCode: "unknown", CategorySource: "unknown", CounterpartyInn: d.CounterpartyINN, CounterpartyName: d.CounterpartyName, PaymentPurpose: d.Purpose, FactualSourceKind: string(d.Source.Kind), SourceNamespace: d.Source.Namespace.System, ExternalOperationId: d.Source.ExternalID}
		if d.Direction == ledger.Inflow {
			op.InflowMinor = d.Amount.Minor()
			v[0].Add(v[0], big.NewInt(d.Amount.Minor()))
		} else {
			op.OutflowMinor = d.Amount.Minor()
			v[1].Add(v[1], big.NewInt(d.Amount.Minor()))
		}
		if d.Category.ID != uuid.Nil {
			op.CategoryId = new(d.Category.ID.String())
			op.CategoryCompanyId = new(d.Category.CompanyID.String())
			op.CategorySource = "user"
		}
		flows[d.Date] = v
		request.Operations = append(request.Operations, op)
	}
	balances := make(map[ledger.Date]int64)
	for _, b := range a.Balances {
		if b.Date >= a.From && b.Date <= a.Through {
			balances[b.Date] = b.AmountMinor
		}
	}
	for n := 0; n < 90; n++ {
		date := ledger.Date(cutoff.AddDate(0, 0, n-89).Format(time.DateOnly))
		day := &pb.HistoricalDay{Date: string(date), Source: pb.DaySource_DAY_SOURCE_UNAVAILABLE}
		if v, ok := flows[date]; ok {
			if !v[0].IsInt64() || !v[1].IsInt64() {
				return nil, forecasting.ErrModelInput
			}
			day.InflowMinor = new(v[0].Int64())
			day.OutflowMinor = new(v[1].Int64())
			day.Source = pb.DaySource_DAY_SOURCE_PARTIAL
		}
		if balance, ok := balances[date]; ok {
			day.ClosingBalanceMinor = new(balance)
			day.Source = pb.DaySource_DAY_SOURCE_PARTIAL
		}
		request.Snapshot.History = append(request.Snapshot.History, day)
	}
	return request, nil
}

func validateCSVResponse(input forecasting.FinancialModelInput, request *pb.PredictRequest, r *pb.PredictResponse) error {
	i := input.Snapshot
	if r == nil || r.SnapshotId != i.SnapshotID.String() || r.CompanyId != i.Scope.CompanyID.String() || r.AccountId != i.Scope.AccountID.String() || r.DataVersion != uint64(i.DataVersion) || r.Currency != "RUB" || r.CutoffDate != string(i.Cutoff) || !proto.Equal(r.Model, request.Model) || !proto.Equal(r.CsvHistory, request.CsvHistory) || r.CalendarSnapshotId != request.Calendar.SnapshotId || r.CalendarDataVersion != request.Calendar.DataVersion || !slices.Equal(r.AvailabilityLimitations, request.AvailabilityLimitations) || !r.HistoryOnlyIncludesRecurringFlows || len(r.Days) != 30 || len(r.Components) != 0 || len(r.HistoryEvidence) != 0 || len(r.ReplacedHistoricalOperationIds) != 0 || r.AnalysisContext != nil || len(r.RecurringHypotheses) != 0 || r.ScenarioContext != nil || len(r.ClassifiedOperations) != len(request.Operations) || !unavailableFinancialProbability(r.GapProbability) || r.Uncertainty == nil || r.Uncertainty.Method != "unavailable_incomplete_history" || r.Uncertainty.Calibrated || len(r.Uncertainty.Bands) != 0 || r.Uncertainty.GapProbabilityUnavailableReason != "not_calibrated" || len(r.Assumptions) == 0 {
		return forecasting.ErrModelResponse
	}
	for _, warning := range []string{"CSV_HISTORY_COMPLETENESS_UNVERIFIED", "HISTORY_ONLY_NO_CALENDAR", "UNCALIBRATED_NO_PROBABILITY"} {
		if !slices.Contains(r.Warnings, warning) {
			return forecasting.ErrModelResponse
		}
	}
	for n, op := range request.Operations {
		if !proto.Equal(op, r.ClassifiedOperations[n]) {
			return forecasting.ErrModelResponse
		}
	}
	incoming, outgoing := new(big.Int), new(big.Int)
	for _, op := range request.Operations {
		incoming.Add(incoming, big.NewInt(op.InflowMinor))
		outgoing.Add(outgoing, big.NewInt(op.OutflowMinor))
	}
	incoming.Quo(incoming.Add(incoming, big.NewInt(45)), big.NewInt(90))
	outgoing.Quo(outgoing.Add(outgoing, big.NewInt(45)), big.NewInt(90))
	cutoff, _ := time.Parse(time.DateOnly, string(i.Cutoff))
	if r.StartDate != cutoff.AddDate(0, 0, 1).Format(time.DateOnly) {
		return forecasting.ErrModelResponse
	}
	balance := i.OpeningBalance
	first := ""
	for n, day := range r.Days {
		if day == nil || day.Date != cutoff.AddDate(0, 0, n+1).Format(time.DateOnly) || big.NewInt(day.InflowMinor).Cmp(incoming) != 0 || big.NewInt(day.OutflowMinor).Cmp(outgoing) != 0 || day.BalanceLowerMinor != nil || day.BalanceUpperMinor != nil || !unavailableFinancialProbability(day.NegativeBalanceProbability) {
			return forecasting.ErrModelResponse
		}
		next, err := balanceAfter(balance, day.InflowMinor, day.OutflowMinor)
		if err != nil || next.Minor() != day.ReferenceBalanceMinor {
			return forecasting.ErrModelResponse
		}
		balance = next
		if first == "" && balance.Minor() < 0 {
			first = day.Date
		}
	}
	if r.GetFirstNegativeBalanceDate() != first || (first == "" && r.FirstNegativeBalanceDate != nil) {
		return forecasting.ErrModelResponse
	}
	return nil
}
