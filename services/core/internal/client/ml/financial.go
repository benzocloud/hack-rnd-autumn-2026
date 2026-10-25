package ml

import (
	"context"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/ml/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
)

const financialVersion = "mean_90_components_v1"
const financialPreprocessing = "calendar_replacement_v1"

func (c *client) PredictFinancial(ctx context.Context, input forecasting.FinancialModelInput) (forecasting.FinancialModelResult, error) {
	request, err := encodeFinancial(input)
	if err != nil {
		return forecasting.FinancialModelResult{}, err
	}
	if proto.Size(request) > maxMessageSize {
		return forecasting.FinancialModelResult{}, forecasting.ErrModelInput
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	response, err := c.rpc.Predict(ctx, request, grpc.MaxCallRecvMsgSize(maxMessageSize), grpc.MaxCallSendMsgSize(maxMessageSize))
	if err != nil {
		return forecasting.FinancialModelResult{}, mapError(err)
	}
	if err := validateFinancialResponse(input, request, response); err != nil {
		return forecasting.FinancialModelResult{}, err
	}
	return decodeFinancial(input, response), nil
}

func encodeFinancial(input forecasting.FinancialModelInput) (*pb.PredictRequest, error) {
	if input.Snapshot.Model.Version == forecasting.CSVCalendarModelVersion {
		return encodeCSV(input)
	}
	if input.Snapshot.Model.Version == forecasting.CSVModelVersion {
		if input.Calendar.Captured && input.Calendar.KnowledgeMode == "current" {
			return nil, forecasting.ErrModelContext
		}
		return encodeCSV(input)
	}
	i := input.Snapshot
	if input.Calendar.Captured && input.Calendar.KnowledgeMode != "current" && input.Calendar.KnowledgeMode != "unavailable" {
		return nil, forecasting.ErrModelInput
	}
	if input.Calendar.Captured && input.Calendar.KnowledgeMode != "current" && (len(input.Calendar.Payments) > 0 || len(input.Calendar.Groups) > 0) {
		return nil, forecasting.ErrModelContext
	}
	if i.Model != (forecasting.ModelContext{Name: "mean_90", Version: financialVersion, PreprocessingVersion: financialPreprocessing}) {
		return nil, forecasting.ErrModelContext
	}
	if i.HorizonDays != 30 || len(i.KnownPayments) > 0 || len(input.Operations) > 20000 || len(i.History) != 90 || strings.TrimSpace(input.Coverage.Reference) == "" || len(input.Coverage.Reference) > 2048 || input.Coverage.DataVersion != i.DataVersion ||
		(input.Coverage.Kind != "certified" && input.Coverage.Kind != "dataset_fixture_assumption") || input.Coverage.From != i.History[0].Date || input.Coverage.Through != i.Cutoff || (input.Coverage.Kind == "dataset_fixture_assumption" && !slices.Contains(input.Coverage.Limitations, "dataset_coverage_assumed_not_bank_certified")) {
		return nil, forecasting.ErrModelInput
	}
	// Reuse the existing exact money/date/reconciliation checks. Only the copied
	// validation view labels a covered reconstructed day as observed; the actual
	// financial request preserves its reconstructed provenance.
	validation := i
	validation.History = slices.Clone(i.History)
	for n, day := range i.History {
		if day.Source == forecasting.ReconstructedCovered {
			validation.History[n].Source = forecasting.Observed
			if n > 0 {
				expected := big.NewInt(i.History[n-1].ClosingBalance.Minor())
				expected.Add(expected, big.NewInt(day.Inflow.Minor()))
				expected.Sub(expected, big.NewInt(day.Outflow.Minor()))
				if expected.Cmp(big.NewInt(day.ClosingBalance.Minor())) != 0 {
					return nil, forecasting.ErrModelInput
				}
			}
		}
	}
	if i.History[89].Source != forecasting.Observed || validateInput(validation) != nil {
		return nil, forecasting.ErrModelInput
	}
	request := encodeInput(i)
	request.Model.ConfigurationId = financialVersion
	request.Calendar = &pb.FinancialCalendar{SnapshotId: i.SnapshotID.String(), DataVersion: uint64(i.DataVersion), KnowledgeMode: pb.CalendarKnowledgeMode_CALENDAR_KNOWLEDGE_MODE_UNAVAILABLE}
	request.AvailabilityLimitations = []string{"historical_calendar_unavailable", "historical_availability_unverified"}
	request.AvailabilityLimitations = append(request.AvailabilityLimitations, "history_coverage_kind_"+input.Coverage.Kind)
	request.AvailabilityLimitations = append(request.AvailabilityLimitations, input.Coverage.Limitations...)
	if input.Calendar.Captured && input.Calendar.KnowledgeMode == "current" {
		current, calendarErr := encodeCurrentCalendar(input)
		request.Calendar = current
		if calendarErr != nil {
			return nil, calendarErr
		}
		request.AvailabilityLimitations = []string{"historical_availability_unverified", "history_coverage_kind_" + input.Coverage.Kind}
		request.AvailabilityLimitations = append(request.AvailabilityLimitations, input.Coverage.Limitations...)
	}
	for n, day := range i.History {
		if day.Source == forecasting.ReconstructedCovered {
			request.Snapshot.History[n].Source = pb.DaySource_DAY_SOURCE_RECONSTRUCTED_COVERED
		}
	}
	totals := make(map[domain.Date][2]*big.Int)
	seen := make(map[uuid.UUID]bool)
	for _, record := range input.Operations {
		d := record.Data()
		if _, err := domain.RestoreTransaction(d, record.Version()); err != nil || seen[d.ID] || d.Scope != i.Scope || d.Date < input.Coverage.From || d.Date > i.Cutoff {
			return nil, forecasting.ErrModelInput
		}
		seen[d.ID] = true
		total, ok := totals[d.Date]
		if !ok {
			total = [2]*big.Int{new(big.Int), new(big.Int)}
		}
		op := &pb.FinancialOperation{OperationId: d.ID.String(), Date: string(d.Date), Version: uint64(record.Version()), CategoryCode: "unknown", CategorySource: "unknown",
			CounterpartyInn: d.CounterpartyINN, CounterpartyName: d.CounterpartyName, PaymentPurpose: d.Purpose, FactualSourceKind: string(d.Source.Kind), SourceNamespace: d.Source.Namespace.System, ExternalOperationId: d.Source.ExternalID}
		if d.Direction == domain.Inflow {
			op.InflowMinor = d.Amount.Minor()
			total[0].Add(total[0], big.NewInt(d.Amount.Minor()))
		} else {
			op.OutflowMinor = d.Amount.Minor()
			total[1].Add(total[1], big.NewInt(d.Amount.Minor()))
		}
		if d.Category.ID != uuid.Nil {
			id, company := d.Category.ID.String(), d.Category.CompanyID.String()
			op.CategoryId, op.CategoryCompanyId = &id, &company
			op.CategorySource = "user"
		}
		totals[d.Date] = total
		request.Operations = append(request.Operations, op)
	}
	for _, day := range i.History {
		incoming, outgoing := new(big.Int), new(big.Int)
		if total, ok := totals[day.Date]; ok {
			incoming, outgoing = total[0], total[1]
		}
		if incoming.Cmp(big.NewInt(day.Inflow.Minor())) != 0 || outgoing.Cmp(big.NewInt(day.Outflow.Minor())) != 0 {
			return nil, forecasting.ErrModelInput
		}
	}
	return request, nil
}

func unavailableFinancialProbability(p *pb.FinancialProbability) bool {
	if p == nil {
		return false
	}
	v, ok := p.Estimate.(*pb.FinancialProbability_UnavailableReason)
	return ok && v != nil && v.UnavailableReason == "not_calibrated"
}

func validateFinancialResponse(input forecasting.FinancialModelInput, request *pb.PredictRequest, r *pb.PredictResponse) error {
	if input.Snapshot.Model.Version == forecasting.CSVCalendarModelVersion {
		return validateCSVCalendarResponse(input, request, r)
	}
	if request.Calendar.KnowledgeMode == pb.CalendarKnowledgeMode_CALENDAR_KNOWLEDGE_MODE_CURRENT {
		return validateCurrentFinancialResponse(input, request, r)
	}
	if input.Snapshot.Model.Version == forecasting.CSVModelVersion {
		return validateCSVResponse(input, request, r)
	}
	if r == nil || r.SnapshotId != request.Snapshot.SnapshotId || r.CompanyId != request.Snapshot.CompanyId || r.AccountId != request.Snapshot.AccountId || r.DataVersion != request.Snapshot.DataVersion || r.Currency != money.RUB || r.CutoffDate != request.Snapshot.CutoffDate ||
		!proto.Equal(r.Model, request.Model) || r.CalendarSnapshotId != request.Calendar.SnapshotId || r.CalendarDataVersion != request.Calendar.DataVersion || len(r.ReplacedHistoricalOperationIds) != 0 || !r.HistoryOnlyIncludesRecurringFlows ||
		len(r.Days) != 30 || !unavailableFinancialProbability(r.GapProbability) || !slices.Equal(r.AvailabilityLimitations, request.AvailabilityLimitations) || len(r.ClassifiedOperations) != len(request.Operations) ||
		!slices.Contains(r.Warnings, "HISTORY_ONLY_NO_CALENDAR") || !slices.Contains(r.Warnings, "UNCALIBRATED_NO_PROBABILITY") || !slices.Contains(r.Warnings, "UNCALIBRATED_EMPIRICAL_BANDS") {
		return forecasting.ErrModelResponse
	}
	for n, op := range request.Operations {
		if !proto.Equal(op, r.ClassifiedOperations[n]) {
			return forecasting.ErrModelResponse
		}
	}
	cutoff, _ := time.Parse(time.DateOnly, string(input.Snapshot.Cutoff))
	if r.StartDate != cutoff.AddDate(0, 0, 1).Format(time.DateOnly) {
		return forecasting.ErrModelResponse
	}
	if hasReconciliation(input.Snapshot.History) && !slices.Contains(r.Warnings, "OBSERVED_RECONCILIATION_WITHIN_ONE_MINOR_UNIT") {
		return forecasting.ErrModelResponse
	}
	balance := input.Snapshot.OpeningBalance
	first := ""
	totalIn, totalOut := new(big.Int), new(big.Int)
	for _, day := range input.Snapshot.History {
		totalIn.Add(totalIn, big.NewInt(day.Inflow.Minor()))
		totalOut.Add(totalOut, big.NewInt(day.Outflow.Minor()))
	}
	expectedIn := new(big.Int).Quo(new(big.Int).Add(totalIn, big.NewInt(45)), big.NewInt(90))
	expectedOut := new(big.Int).Quo(new(big.Int).Add(totalOut, big.NewInt(45)), big.NewInt(90))
	for n, day := range r.Days {
		if day == nil || day.Date != cutoff.AddDate(0, 0, n+1).Format(time.DateOnly) || day.InflowMinor < 0 || day.OutflowMinor < 0 || day.BalanceLowerMinor != nil || day.BalanceUpperMinor != nil || !unavailableFinancialProbability(day.NegativeBalanceProbability) || big.NewInt(day.InflowMinor).Cmp(expectedIn) != 0 || big.NewInt(day.OutflowMinor).Cmp(expectedOut) != 0 {
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
	return validateFinancialEvidence(request, r)
}
