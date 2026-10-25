package app

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	appPostgres "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/app/postgres"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
	commandsSQL "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres/commands"
	scenariosSQL "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres/scenarios"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
)

type scenarioOperations interface {
	Create(context.Context, forecasting.ScenarioCommand) (forecasting.ScenarioView, error)
	Read(context.Context, ledger.Scope, uuid.UUID) (forecasting.ScenarioView, error)
	Forecast(context.Context, ledger.Scope, uuid.UUID) (forecasting.ForecastView, error)
	List(context.Context, forecasting.ScenarioListCommand) (forecasting.ScenarioList, error)
}

func newScenarios(db *sqlx.DB, cfg config.Postgres, jobs forecastJobs) scenarioOperations {
	return forecasting.NewScenarios(func(ctx context.Context, work forecasting.ScenarioWork) error {
		err := appPostgres.WithinTx(ctx, db, cfg.QueryTimeout, func(ctx context.Context, tx *sqlx.Tx) error {
			return work(ctx, scenariosSQL.New(tx), commandsSQL.New(tx))
		})
		if errors.Is(err, storage.ErrIdempotencyConflict) {
			return storage.WithCause(forecasting.ErrJobConflict, err)
		}
		return forecastingError(err)
	}, jobs)
}
