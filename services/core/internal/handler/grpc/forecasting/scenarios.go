package forecasting

import (
	"context"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/rpcerror"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
)

type scenarioOperations interface {
	Create(context.Context, forecasting.ScenarioCommand) (forecasting.ScenarioView, error)
	Read(context.Context, ledger.Scope, uuid.UUID) (forecasting.ScenarioView, error)
	Forecast(context.Context, ledger.Scope, uuid.UUID) (forecasting.ForecastView, error)
	List(context.Context, forecasting.ScenarioListCommand) (forecasting.ScenarioList, error)
}

func encodeScenario(v forecasting.ScenarioView) *pb.Scenario {
	r := v.Record
	s := &pb.Scenario{Id: r.ID.String(), ForecastId: r.ForecastID.String(), BaseForecastId: r.BaseForecastID.String(), Name: r.Name, CreatedAt: timestamppb.New(r.CreatedAt), Changes: []*pb.ScenarioChange{}, OutOfHorizonOccurrenceIds: []string{}}
	for _, c := range v.Changes {
		s.Changes = append(s.Changes, &pb.ScenarioChange{Change: &pb.ScenarioChange_OneOffExpense{OneOffExpense: &pb.OneOffExpenseChange{Id: c.ID.String(), Date: string(c.Date), Amount: kopecks(c.AmountMinor), Description: c.Description}}})
	}
	for _, c := range v.SalesChanges {
		targets := []*pb.OccurrenceTarget{}
		for _, t := range c.Targets {
			targets = append(targets, &pb.OccurrenceTarget{ComponentId: t.ComponentID, OccurrenceId: t.OccurrenceID})
		}
		s.Changes = append(s.Changes, &pb.ScenarioChange{Change: &pb.ScenarioChange_ReduceSales{ReduceSales: &pb.ReduceSalesChange{Id: c.ID.String(), Targets: targets, PeriodStart: string(c.PeriodStart), PeriodEnd: string(c.PeriodEnd), ReductionBasisPoints: c.ReductionBasisPoints}}})
	}

	return s
}
func (h *handler) CreateScenario(ctx context.Context, r *pb.CreateScenarioRequest) (*pb.CreateScenarioResponse, error) {
	if r == nil || r.Context == nil || h.scenarios == nil {
		return nil, rpcerror.From(forecasting.ErrInvalid)
	}
	scope, err := parseScope(r.Scope)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	actor, err := h.authorize(ctx, scope, nil, r.Context)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	input := r.GetIndependent()
	if input == nil {
		return nil, rpcerror.New(codes.FailedPrecondition, pb.CoreErrorReason_CORE_ERROR_REASON_SCENARIO_NOT_SUPPORTED)
	}
	base, err := parseID(input.BaseForecastId)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	cmd := forecasting.ScenarioCommand{Scope: scope, ActorID: actor, IdempotencyKey: r.IdempotencyKey, Name: r.Name, BaseForecastID: base}
	for _, c := range input.Changes {
		if reduction := c.GetReduceSales(); reduction != nil {
			id, e := parseID(reduction.Id)
			if e != nil {
				return nil, rpcerror.From(e)
			}
			change := forecasting.SalesReductionChange{ID: id, PeriodStart: ledger.Date(reduction.PeriodStart), PeriodEnd: ledger.Date(reduction.PeriodEnd), ReductionBasisPoints: reduction.ReductionBasisPoints}
			for _, t := range reduction.Targets {
				change.Targets = append(change.Targets, forecasting.SalesTarget{ComponentID: t.GetComponentId(), OccurrenceID: t.GetOccurrenceId()})
			}
			cmd.SalesChanges = append(cmd.SalesChanges, change)
			continue
		}

		v := c.GetOneOffExpense()
		if v == nil {
			return nil, rpcerror.New(codes.FailedPrecondition, pb.CoreErrorReason_CORE_ERROR_REASON_SCENARIO_NOT_SUPPORTED)
		}
		id, e := parseID(v.Id)
		if e != nil || v.Amount == nil || v.Amount.Currency != "RUB" || v.CategoryId != nil {
			return nil, rpcerror.From(forecasting.ErrInvalid)
		}
		cmd.Changes = append(cmd.Changes, forecasting.ExpenseChange{ID: id, Date: ledger.Date(v.Date), AmountMinor: v.Amount.AmountMinor, Description: v.Description})
	}
	v, err := h.scenarios.Create(ctx, cmd)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	return &pb.CreateScenarioResponse{Accepted: &pb.ScenarioAccepted{Id: v.Record.ID.String(), ForecastId: v.Record.ForecastID.String(), Status: pb.ForecastStatus_FORECAST_STATUS_QUEUED, PollAfterMs: 1000}}, nil
}
func (h *handler) GetScenario(ctx context.Context, r *pb.GetScenarioRequest) (*pb.GetScenarioResponse, error) {
	if r == nil || h.scenarios == nil {
		return nil, rpcerror.From(forecasting.ErrInvalid)
	}
	scope, err := parseScope(r.Scope)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	if _, err = h.authorize(ctx, scope, r.Context, nil); err != nil {
		return nil, rpcerror.From(err)
	}
	id, err := parseID(r.ScenarioId)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	v, err := h.scenarios.Read(ctx, scope, id)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	return &pb.GetScenarioResponse{Scenario: encodeScenario(v)}, nil
}
func (h *handler) ListScenarios(ctx context.Context, r *pb.ListScenariosRequest) (*pb.ListScenariosResponse, error) {
	if r == nil || h.scenarios == nil {
		return nil, rpcerror.From(forecasting.ErrInvalid)
	}
	scope, err := parseScope(r.Scope)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	actor, err := h.authorize(ctx, scope, r.Context, nil)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	cmd := forecasting.ScenarioListCommand{Scope: scope, ActorID: actor, Limit: 50}
	if r.Page != nil {
		cmd.Cursor = r.Page.Cursor
		if r.Page.Limit != nil {
			if *r.Page.Limit == 0 || *r.Page.Limit > 100 {
				return nil, rpcerror.From(forecasting.ErrInvalid)
			}
			cmd.Limit = int(*r.Page.Limit)
		}
	}
	if r.BaseForecastId != nil {
		cmd.BaseForecastID, err = parseID(*r.BaseForecastId)
		if err != nil {
			return nil, rpcerror.From(err)
		}
	}
	list, err := h.scenarios.List(ctx, cmd)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	out := &pb.ListScenariosResponse{Items: []*pb.Scenario{}, Pagination: &pb.PageResponse{NextCursor: list.NextCursor}}
	for _, v := range list.Items {
		out.Items = append(out.Items, encodeScenario(v))
	}
	return out, nil
}
func (h *handler) CompareForecasts(ctx context.Context, r *pb.CompareForecastsRequest) (*pb.CompareForecastsResponse, error) {
	if r == nil {
		return nil, rpcerror.From(forecasting.ErrInvalid)
	}
	scope, err := parseScope(r.Scope)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	if _, err = h.authorize(ctx, scope, r.Context, nil); err != nil {
		return nil, rpcerror.From(err)
	}
	cid, err := parseID(r.CandidateForecastId)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	rid, err := parseID(r.ReferenceForecastId)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	candidate, err := h.readForecast(ctx, scope, cid)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	reference, err := h.readForecast(ctx, scope, rid)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	incompatible := rpcerror.New(codes.FailedPrecondition, pb.CoreErrorReason_CORE_ERROR_REASON_FORECAST_NOT_COMPARABLE)
	if candidate.Status != "succeeded" || reference.Status != "succeeded" || candidate.Result == nil || reference.Result == nil || candidate.Snapshot.Data().ID != reference.Snapshot.Data().ID || candidate.ModelContextID != reference.ModelContextID || candidate.ScenarioID == nil || reference.ScenarioID != nil || h.scenarios == nil {
		return nil, incompatible
	}
	scenario, err := h.scenarios.Read(ctx, scope, *candidate.ScenarioID)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	if scenario.Record.BaseForecastID != rid {
		return nil, incompatible
	}
	cm, rm := encodeResult(candidate.Result).Metrics, encodeResult(reference.Result).Metrics
	delta := func(a, b int64) (*pb.Money, error) {
		x, _ := money.New(a, money.RUB)
		y, _ := money.New(b, money.RUB)
		z, e := x.Sub(y)
		return kopecks(z.Minor()), e
	}
	shortfall, e := delta(cm.MaxShortfall.AmountMinor, rm.MaxShortfall.AmountMinor)
	if e != nil {
		return nil, incompatible
	}
	minimum, e := delta(cm.MinimumBalance.AmountMinor, rm.MinimumBalance.AmountMinor)
	if e != nil {
		return nil, incompatible
	}
	closing, e := delta(cm.ClosingBalance.AmountMinor, rm.ClosingBalance.AmountMinor)
	if e != nil {
		return nil, incompatible
	}
	out := &pb.Comparison{ReferenceForecastId: rid.String(), CandidateForecastId: cid.String(), SnapshotId: candidate.Snapshot.Data().ID.String(), ModelContextId: candidate.ModelContextID.String(), IsStale: candidate.CurrentDataVersion > candidate.DataVersion || reference.CurrentDataVersion > reference.DataVersion, ReferenceMetrics: rm, CandidateMetrics: cm, DeltaMaxShortfall: shortfall, DeltaMinimumBalance: minimum, DeltaClosingBalance: closing}
	if cm.FirstGap != nil && rm.FirstGap != nil {
		c, _ := time.Parse(time.DateOnly, cm.FirstGap.Date)
		v, _ := time.Parse(time.DateOnly, rm.FirstGap.Date)
		d := int32(c.Sub(v).Hours() / 24)
		out.DeltaFirstGapDays = &d
	}
	return &pb.CompareForecastsResponse{Comparison: out}, nil
}
