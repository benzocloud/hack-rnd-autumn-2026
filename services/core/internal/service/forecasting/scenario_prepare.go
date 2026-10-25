package forecasting

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/google/uuid"
)

func (s *scenarios) prepareScenario(ctx context.Context, cmd ScenarioCommand) (scenarioPayload, uuid.UUID, error) {
	if cmd.ParentScenarioID != uuid.Nil {
		return s.prepareResponse(ctx, cmd)
	}
	base, err := s.base.Read(ctx, cmd.Scope, cmd.BaseForecastID)
	if err != nil {
		return scenarioPayload{}, uuid.Nil, err
	}
	if base.Status != "succeeded" || base.Result == nil {
		return scenarioPayload{}, uuid.Nil, ErrInvalid
	}
	result, err := ApplyScenario(*base.Result, cmd.Changes)
	if err != nil {
		return scenarioPayload{}, uuid.Nil, err
	}
	result, sales, err := applySalesReductions(result, base.Sales, cmd.SalesChanges)
	if err != nil {
		return scenarioPayload{}, uuid.Nil, err
	}
	prepared := base
	prepared.Result = &result
	prepared.Sales = sales
	prepared.ScenarioExpenses = cmd.Changes
	occurrences, err := ScenarioOccurrences(prepared)
	if err != nil {
		return scenarioPayload{}, uuid.Nil, err
	}
	result, occurrences, err = applyReceiptDelays(result, occurrences, cmd.DelayChanges)
	if err != nil {
		return scenarioPayload{}, uuid.Nil, err
	}
	schema := 1
	if len(cmd.SalesChanges) > 0 {
		schema = 2
	}
	if len(cmd.DelayChanges) > 0 {
		schema = 3
	}
	return scenarioPayload{SchemaVersion: schema, Changes: cmd.Changes, SalesChanges: cmd.SalesChanges, SalesBasis: sales, DelayChanges: cmd.DelayChanges, Occurrences: occurrences, OutOfHorizonIDs: outsideHorizon(occurrences, result), Result: result}, cmd.BaseForecastID, nil
}

func (s *scenarios) prepareResponse(ctx context.Context, cmd ScenarioCommand) (scenarioPayload, uuid.UUID, error) {
	parent, err := s.Read(ctx, cmd.Scope, cmd.ParentScenarioID)
	if err != nil {
		return scenarioPayload{}, uuid.Nil, err
	}
	view, err := s.Forecast(ctx, cmd.Scope, parent.Record.ForecastID)
	if err != nil {
		return scenarioPayload{}, uuid.Nil, err
	}
	if view.Status != "succeeded" || view.Result == nil {
		return scenarioPayload{}, uuid.Nil, ErrInvalid
	}
	result, err := ApplyScenario(*view.Result, nil)
	if err != nil {
		return scenarioPayload{}, uuid.Nil, err
	}
	partID := fmt.Sprintf("response:%x", sha256.Sum256([]byte(cmd.ParentScenarioID.String()+"\x00"+cmd.ActorID+"\x00"+cmd.IdempotencyKey)))
	result, occurrences, err := applyResponse(result, view.Occurrences, cmd.Response, partID)
	if err != nil {
		return scenarioPayload{}, uuid.Nil, err
	}
	parentID := cmd.ParentScenarioID
	return scenarioPayload{SchemaVersion: 3, Changes: parent.Changes, SalesChanges: parent.SalesChanges, SalesBasis: view.Sales, DelayChanges: parent.DelayChanges, ParentScenarioID: &parentID, ResponseAction: cmd.Response, Occurrences: occurrences, OutOfHorizonIDs: outsideHorizon(occurrences, result), Result: result}, parent.Record.BaseForecastID, nil
}
