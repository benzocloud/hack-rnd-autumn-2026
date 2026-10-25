package ml

import (
	"math/big"
	"slices"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/ml/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
)

func validateFinancialEvidence(request *pb.PredictRequest, r *pb.PredictResponse) error {
	u := r.Uncertainty
	if u == nil || u.Method != "paired_moving_block_bootstrap_uncalibrated" || u.Calibrated || u.Seed != 20261024 || u.Scenarios != 512 || u.BlockDays != 7 || u.LowerQuantile != 0.1 || u.UpperQuantile != 0.9 || u.GapProbabilityUnavailableReason != "REQUIREMENT_OPEN_NO_CALIBRATED_GAP_PROBABILITY" || len(u.Bands) != 30 || len(u.Assumptions) == 0 {
		return forecasting.ErrModelResponse
	}
	for n, band := range u.Bands {
		if band == nil || band.Date != r.Days[n].Date || band.LowerMinor > band.UpperMinor {
			return forecasting.ErrModelResponse
		}
	}
	operations := make(map[string]*pb.FinancialOperation)
	for _, op := range request.Operations {
		operations[op.OperationId] = op
	}
	evidence := make(map[string]*pb.HistoryGroupEvidence)
	covered := make(map[string]bool)
	for _, e := range r.HistoryEvidence {
		if e == nil || e.EvidenceId == "" || evidence[e.EvidenceId] != nil || len(e.HistoricalOperationIds) == 0 {
			return forecasting.ErrModelResponse
		}
		for _, id := range e.HistoricalOperationIds {
			op := operations[id]
			if op == nil || covered[id] || e.CategorySource != op.CategorySource {
				return forecasting.ErrModelResponse
			}
			covered[id] = true
		}
		evidence[e.EvidenceId] = e
	}
	if len(covered) != len(operations) {
		return forecasting.ErrModelResponse
	}
	totals := make(map[string][2]*big.Int)
	seen := make(map[string]bool)
	for _, c := range r.Components {
		if c == nil || c.ComponentId == "" || seen[c.ComponentId] || c.Kind != "mean_90_uncovered" || c.InflowMinor < 0 || c.OutflowMinor < 0 || (c.InflowMinor == 0 && c.OutflowMinor == 0) || len(c.SourceIds) != 1 || evidence[c.SourceIds[0]] == nil {
			return forecasting.ErrModelResponse
		}
		seen[c.ComponentId] = true
		group := evidence[c.SourceIds[0]]
		for _, id := range group.HistoricalOperationIds {
			op := operations[id]
			if c.CategoryCode != op.CategoryCode || c.RegularityKey != op.RegularityKey || c.GetCategoryId() != op.GetCategoryId() || c.GetCategoryCompanyId() != op.GetCategoryCompanyId() || ((c.CategoryId == nil) != (op.CategoryId == nil)) || ((c.CategoryCompanyId == nil) != (op.CategoryCompanyId == nil)) {
				return forecasting.ErrModelResponse
			}
		}
		total, ok := totals[c.Date]
		if !ok {
			total = [2]*big.Int{new(big.Int), new(big.Int)}
		}
		total[0].Add(total[0], big.NewInt(c.InflowMinor))
		total[1].Add(total[1], big.NewInt(c.OutflowMinor))
		totals[c.Date] = total
	}
	for _, day := range r.Days {
		total, ok := totals[day.Date]
		if !ok {
			total = [2]*big.Int{new(big.Int), new(big.Int)}
		}
		if total[0].Cmp(big.NewInt(day.InflowMinor)) != 0 || total[1].Cmp(big.NewInt(day.OutflowMinor)) != 0 {
			return forecasting.ErrModelResponse
		}
		delete(totals, day.Date)
	}
	if len(totals) != 0 {
		return forecasting.ErrModelResponse
	}
	return nil
}

func decodeFinancial(input forecasting.FinancialModelInput, r *pb.PredictResponse) forecasting.FinancialModelResult {
	i := input.Snapshot
	projection := forecasting.ModelResult{SnapshotID: i.SnapshotID, Scope: i.Scope, DataVersion: i.DataVersion, Cutoff: i.Cutoff, Model: i.Model, HistoryOnlyIncludesRecurringFlows: r.HistoryOnlyIncludesRecurringFlows, Warnings: slices.Clone(r.Warnings)}
	for _, day := range r.Days {
		incoming, _ := money.New(day.InflowMinor, money.RUB)
		outgoing, _ := money.New(day.OutflowMinor, money.RUB)
		balance, _ := money.New(day.ReferenceBalanceMinor, money.RUB)
		projection.Days = append(projection.Days, forecasting.ModelForecastDay{Date: domain.Date(day.Date), Inflow: incoming, Outflow: outgoing, ReferenceBalance: balance})
	}
	if r.FirstNegativeBalanceDate != nil {
		date := domain.Date(*r.FirstNegativeBalanceDate)
		projection.FirstNegativeDate = &date
	}
	result := forecasting.FinancialModelResult{Projection: projection, ProbabilityUnavailableReason: "not_calibrated", Assumptions: append(slices.Clone(r.Assumptions), r.Uncertainty.Assumptions...), Limitations: slices.Clone(r.AvailabilityLimitations)}
	if i.Model.Version != forecasting.CSVModelVersion {
		result.Assumptions = append(result.Assumptions, "Empirical uncalibrated bands are not exposed as public confidence intervals; residual components are not scenario targets.")
	}
	for _, c := range r.Components {
		id, _ := uuid.Parse(c.GetCategoryId())
		result.Components = append(result.Components, forecasting.FinancialComponent{ID: c.ComponentId, Kind: c.Kind, CategoryCode: c.CategoryCode, RegularityKey: c.RegularityKey, Date: domain.Date(c.Date), InflowMinor: c.InflowMinor, OutflowMinor: c.OutflowMinor, SourceIDs: slices.Clone(c.SourceIds), CategoryID: id})
	}
	for _, e := range r.HistoryEvidence {
		entry := forecasting.HistoryEvidence{ID: e.EvidenceId, CategorySource: e.CategorySource}
		for _, id := range e.HistoricalOperationIds {
			parsed, _ := uuid.Parse(id)
			entry.OperationIDs = append(entry.OperationIDs, parsed)
		}
		result.Evidence = append(result.Evidence, entry)
	}
	return result
}
