package ml

import (
	"math/big"
	"slices"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/ml/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
)

func encodeCurrentCalendar(input forecasting.FinancialModelInput) (*pb.FinancialCalendar, error) {
	c, i := input.Calendar, input.Snapshot
	cutoff, _ := time.Parse(time.DateOnly, string(i.Cutoff))
	start, end := cutoff.AddDate(0, 0, 1).Format(time.DateOnly), cutoff.AddDate(0, 0, 30).Format(time.DateOnly)
	if c.AsOf.Validate() != nil || string(c.AsOf) < string(i.Cutoff) || string(c.AsOf) > start || string(c.AsOf) > string(i.Cutoff) && !c.NoPostCutoffMovements || len(c.Payments) > 1000 || len(c.Groups) > 1000 {
		return nil, forecasting.ErrModelInput
	}
	out := &pb.FinancialCalendar{SnapshotId: i.SnapshotID.String(), DataVersion: uint64(i.DataVersion), KnowledgeMode: pb.CalendarKnowledgeMode_CALENDAR_KNOWLEDGE_MODE_CURRENT, AsOfBusinessDate: string(c.AsOf), NoPostCutoffMovements: c.NoPostCutoffMovements}
	operations := map[uuid.UUID]int64{}
	for _, op := range input.Operations {
		operations[op.Data().ID] = op.Version()
	}
	groups := map[uuid.UUID]bool{}
	covered := map[uuid.UUID]bool{}
	for _, g := range c.Groups {
		if g.Scope != i.Scope || g.ID == uuid.Nil || groups[g.ID] || g.Version < 1 || g.Status != "active" || g.Cutoff != i.Cutoff || string(g.HorizonStart) != start || string(g.HorizonEnd) != end || g.ConfirmedBy == "" || g.ConfirmedAt.IsZero() || g.Additional != (len(g.Operations) == 0) {
			return nil, forecasting.ErrModelInput
		}
		group := &pb.CalendarGroup{GroupId: g.ID.String(), CompleteForHorizon: true, AdditionalObligationConfirmed: g.Additional}
		for _, ref := range g.Operations {
			if covered[ref.ID] || operations[ref.ID] != ref.Version {
				return nil, forecasting.ErrModelInput
			}
			covered[ref.ID] = true
			group.HistoricalOperationIds = append(group.HistoricalOperationIds, ref.ID.String())
		}
		groups[g.ID] = true
		out.Groups = append(out.Groups, group)
	}
	seen := map[uuid.UUID]bool{}
	for _, p := range c.Payments {
		d := p.Data()
		if d.Scope != i.Scope || seen[d.ID] {
			return nil, forecasting.ErrModelInput
		}
		seen[d.ID] = true
		if p.Status() != payments.Planned && p.Status() != payments.PartiallyMatched || string(d.ScheduledDate) > end {
			continue
		}
		if string(d.ScheduledDate) < start || !groups[d.ReconciliationGroupID] || p.Remaining().Positive() != nil {
			return nil, forecasting.ErrModelInput
		}
		scheduled := &pb.ScheduledPayment{PaymentId: d.ID.String(), Version: uint64(p.Version()), EffectiveDate: string(d.ScheduledDate), OriginalDueDate: string(d.ScheduledDate), CategoryCode: "unknown", GroupId: d.ReconciliationGroupID.String(), Source: string(d.Source.Kind)}
		if d.Direction == ledger.Inflow {
			scheduled.RemainingInflowMinor = p.Remaining().Minor()
		} else {
			scheduled.RemainingOutflowMinor = p.Remaining().Minor()
		}
		if d.Category.ID != uuid.Nil {
			id, company := d.Category.ID.String(), d.Category.CompanyID.String()
			scheduled.CategoryId, scheduled.CategoryCompanyId = &id, &company
		}
		out.Payments = append(out.Payments, scheduled)
	}
	return out, nil
}
func validateCurrentFinancialResponse(input forecasting.FinancialModelInput, request *pb.PredictRequest, r *pb.PredictResponse) error {
	s := request.Snapshot
	if r == nil || r.SnapshotId != s.SnapshotId || r.CompanyId != s.CompanyId || r.AccountId != s.AccountId || r.DataVersion != s.DataVersion || r.Currency != money.RUB || r.CutoffDate != s.CutoffDate || !proto.Equal(r.Model, request.Model) || r.CalendarSnapshotId != s.SnapshotId || r.CalendarDataVersion != s.DataVersion || r.HistoryOnlyIncludesRecurringFlows || len(r.Days) != 30 || !unavailableFinancialProbability(r.GapProbability) || !slices.Equal(r.AvailabilityLimitations, request.AvailabilityLimitations) || len(r.ClassifiedOperations) != len(request.Operations) || !slices.Contains(r.Warnings, "EXPLICIT_COMPLETE_CALENDAR_REPLACEMENT") || !slices.Contains(r.Warnings, "UNCALIBRATED_NO_PROBABILITY") || r.ScenarioContext != nil || r.AnalysisContext != nil || len(r.RecurringHypotheses) > 0 {
		return forecasting.ErrModelResponse
	}
	for i, op := range request.Operations {
		if !proto.Equal(op, r.ClassifiedOperations[i]) {
			return forecasting.ErrModelResponse
		}
	}
	replaced := map[string]bool{}
	for _, g := range request.Calendar.Groups {
		for _, id := range g.HistoricalOperationIds {
			if replaced[id] {
				return forecasting.ErrModelResponse
			}
			replaced[id] = true
		}
	}
	if len(replaced) != len(r.ReplacedHistoricalOperationIds) {
		return forecasting.ErrModelResponse
	}
	received := map[string]bool{}
	for _, id := range r.ReplacedHistoricalOperationIds {
		if !replaced[id] || received[id] {
			return forecasting.ErrModelResponse
		}
		received[id] = true
	}
	totals := [2]*big.Int{new(big.Int), new(big.Int)}
	for _, op := range request.Operations {
		if !replaced[op.OperationId] {
			totals[0].Add(totals[0], big.NewInt(op.InflowMinor))
			totals[1].Add(totals[1], big.NewInt(op.OutflowMinor))
		}
	}
	mean := [2]*big.Int{}
	for i := range mean {
		mean[i] = new(big.Int).Quo(new(big.Int).Add(totals[i], big.NewInt(45)), big.NewInt(90))
	}
	known := map[string][2]*big.Int{}
	for _, p := range request.Calendar.Payments {
		sum := known[p.EffectiveDate]
		if sum[0] == nil {
			sum = [2]*big.Int{new(big.Int), new(big.Int)}
		}
		sum[0].Add(sum[0], big.NewInt(p.RemainingInflowMinor))
		sum[1].Add(sum[1], big.NewInt(p.RemainingOutflowMinor))
		known[p.EffectiveDate] = sum
	}
	cutoff, _ := time.Parse(time.DateOnly, s.CutoffDate)
	if r.StartDate != cutoff.AddDate(0, 0, 1).Format(time.DateOnly) {
		return forecasting.ErrModelResponse
	}
	balance := input.Snapshot.OpeningBalance
	first := ""
	for i, day := range r.Days {
		date := cutoff.AddDate(0, 0, i+1).Format(time.DateOnly)
		if day == nil || day.Date != date || day.InflowMinor < 0 || day.OutflowMinor < 0 || day.BalanceLowerMinor != nil || day.BalanceUpperMinor != nil || !unavailableFinancialProbability(day.NegativeBalanceProbability) {
			return forecasting.ErrModelResponse
		}
		for d, actual := range []int64{day.InflowMinor, day.OutflowMinor} {
			expected := new(big.Int).Set(mean[d])
			if sum := known[date]; sum[d] != nil {
				expected.Add(expected, sum[d])
			}
			if !expected.IsInt64() || expected.Int64() != actual {
				return forecasting.ErrModelResponse
			}
		}
		next, err := balanceAfter(balance, day.InflowMinor, day.OutflowMinor)
		if err != nil || next.Minor() != day.ReferenceBalanceMinor {
			return forecasting.ErrModelResponse
		}
		balance = next
		if first == "" && balance.Minor() < 0 {
			first = date
		}
	}
	if r.GetFirstNegativeBalanceDate() != first || first == "" && r.FirstNegativeBalanceDate != nil {
		return forecasting.ErrModelResponse
	}
	return validateCurrentEvidence(request, r, replaced)
}
func validateCurrentEvidence(request *pb.PredictRequest, r *pb.PredictResponse, replaced map[string]bool) error {
	// Reuse the checked residual evidence validator after separating exact known
	// occurrences; no payment may appear twice, and replacement facts stay excluded.
	residualRequest, ok := proto.Clone(request).(*pb.PredictRequest)
	if !ok {
		return forecasting.ErrModelResponse
	}
	residualRequest.Operations = nil
	for _, op := range request.Operations {
		if !replaced[op.OperationId] {
			residualRequest.Operations = append(residualRequest.Operations, op)
		}
	}
	residual, ok := proto.Clone(r).(*pb.PredictResponse)
	if !ok {
		return forecasting.ErrModelResponse
	}
	residual.Components = nil
	paymentsByID := map[string]*pb.ScheduledPayment{}
	for _, p := range request.Calendar.Payments {
		paymentsByID[p.PaymentId] = p
	}
	seen := map[string]bool{}
	componentIDs := map[string]bool{}
	known := map[string][2]int64{}
	for _, c := range r.Components {
		if c == nil || c.ComponentId == "" || componentIDs[c.ComponentId] {
			return forecasting.ErrModelResponse
		}
		componentIDs[c.ComponentId] = true
		if c.Kind != "known_remaining_payment" {
			residual.Components = append(residual.Components, c)
			continue
		}
		if len(c.SourceIds) != 1 {
			return forecasting.ErrModelResponse
		}
		p := paymentsByID[c.SourceIds[0]]
		if p == nil || seen[p.PaymentId] || c.ComponentId != "known:"+p.PaymentId || c.Date != p.EffectiveDate || c.InflowMinor != p.RemainingInflowMinor || c.OutflowMinor != p.RemainingOutflowMinor || c.CategoryCode != p.CategoryCode || c.GetCategoryId() != p.GetCategoryId() || c.GetCategoryCompanyId() != p.GetCategoryCompanyId() {
			return forecasting.ErrModelResponse
		}
		seen[p.PaymentId] = true
		sum := known[c.Date]
		incoming := new(big.Int).Add(big.NewInt(sum[0]), big.NewInt(c.InflowMinor))
		outgoing := new(big.Int).Add(big.NewInt(sum[1]), big.NewInt(c.OutflowMinor))
		if !incoming.IsInt64() || !outgoing.IsInt64() {
			return forecasting.ErrModelResponse
		}
		known[c.Date] = [2]int64{incoming.Int64(), outgoing.Int64()}
	}
	if len(seen) != len(paymentsByID) {
		return forecasting.ErrModelResponse
	}
	for _, day := range residual.Days {
		sum := known[day.Date]
		if day.InflowMinor < sum[0] || day.OutflowMinor < sum[1] {
			return forecasting.ErrModelResponse
		}
		day.InflowMinor -= sum[0]
		day.OutflowMinor -= sum[1]
	}
	return validateFinancialEvidence(residualRequest, residual)
}
