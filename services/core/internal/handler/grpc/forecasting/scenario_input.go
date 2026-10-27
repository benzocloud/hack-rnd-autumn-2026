package forecasting

import (
	"google.golang.org/grpc/codes"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/rpcerror"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
)

func scenarioCommand(r *pb.CreateScenarioRequest, scope ledger.Scope, actor string) (forecasting.ScenarioCommand, error) {
	cmd := forecasting.ScenarioCommand{Scope: scope, ActorID: actor, IdempotencyKey: r.IdempotencyKey, Name: r.Name}
	if response := r.GetResponse(); response != nil {
		parent, err := parseID(response.ComparisonParentScenarioId)
		if err != nil {
			return cmd, rpcerror.From(err)
		}
		action := response.ResponseAction
		if action == nil || action.Amount == nil || action.Amount.Currency != "RUB" {
			return cmd, rpcerror.From(forecasting.ErrInvalid)
		}
		cmd.ParentScenarioID = parent
		cmd.Response = &forecasting.ResponseAction{EffectiveOccurrenceID: action.EffectiveOccurrenceId, AmountMinor: action.Amount.AmountMinor, NewDate: ledger.Date(action.NewDate), FeasibilityCondition: action.FeasibilityCondition}
		return cmd, nil
	}
	input := r.GetIndependent()
	if input == nil {
		return cmd, rpcerror.From(forecasting.ErrInvalid)
	}
	base, err := parseID(input.BaseForecastId)
	if err != nil {
		return cmd, rpcerror.From(err)
	}
	cmd.BaseForecastID = base
	for _, c := range input.Changes {
		if delay := c.GetDelayReceipt(); delay != nil {
			id, e := parseID(delay.Id)
			if e != nil || delay.Target == nil || delay.Amount == nil || delay.Amount.Currency != "RUB" {
				return cmd, rpcerror.From(forecasting.ErrInvalid)
			}
			cmd.DelayChanges = append(cmd.DelayChanges, forecasting.DelayReceiptChange{ID: id, Target: forecasting.ReceiptTarget{ComponentID: delay.Target.ComponentId, OccurrenceID: delay.Target.OccurrenceId}, AmountMinor: delay.Amount.AmountMinor, NewDate: ledger.Date(delay.NewDate)})
			continue
		}
		if reduction := c.GetReduceSales(); reduction != nil {
			id, e := parseID(reduction.Id)
			if e != nil {
				return cmd, rpcerror.From(e)
			}
			change := forecasting.SalesReductionChange{ID: id, PeriodStart: ledger.Date(reduction.PeriodStart), PeriodEnd: ledger.Date(reduction.PeriodEnd), ReductionBasisPoints: reduction.ReductionBasisPoints}
			for _, t := range reduction.Targets {
				change.Targets = append(change.Targets, forecasting.SalesTarget{ComponentID: t.GetComponentId(), OccurrenceID: t.GetOccurrenceId()})
			}
			cmd.SalesChanges = append(cmd.SalesChanges, change)
			continue
		}
		expense := c.GetOneOffExpense()
		if expense == nil {
			return cmd, rpcerror.New(codes.FailedPrecondition, pb.CoreErrorReason_CORE_ERROR_REASON_SCENARIO_NOT_SUPPORTED)
		}
		id, e := parseID(expense.Id)
		if e != nil || expense.Amount == nil || expense.Amount.Currency != "RUB" || expense.CategoryId != nil {
			return cmd, rpcerror.From(forecasting.ErrInvalid)
		}
		cmd.Changes = append(cmd.Changes, forecasting.ExpenseChange{ID: id, Date: ledger.Date(expense.Date), AmountMinor: expense.Amount.AmountMinor, Description: expense.Description})
	}
	return cmd, nil
}
