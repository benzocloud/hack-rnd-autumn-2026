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
	forecastSQL "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres/forecasting"
	jobsSQL "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres/forecastjobs"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
)

type financialPredictor interface {
	PredictFinancial(context.Context, forecasting.FinancialModelInput) (forecasting.FinancialModelResult, error)
}

type forecastJobs interface {
	Create(context.Context, forecasting.CreateForecastCommand) (forecasting.ForecastAccepted, error)
	Read(context.Context, ledger.Scope, uuid.UUID) (forecasting.ForecastView, error)
	List(context.Context, forecasting.ListForecastCommand) (forecasting.ListForecastResult, error)
	Run(context.Context) error
	ProcessOne(context.Context) (bool, error)
}

func newForecastJobs(db *sqlx.DB, cfg config.Postgres, model financialPredictor) forecastJobs {
	return forecasting.NewJobs(func(ctx context.Context, work forecasting.ForecastWork) error {
		err := appPostgres.WithinTx(ctx, db, cfg.QueryTimeout, func(ctx context.Context, tx *sqlx.Tx) error {
			return work(ctx, jobsSQL.New(tx), forecastSQL.New(tx), commandsSQL.New(tx))
		})
		if errors.Is(err, storage.ErrIdempotencyConflict) {
			return storage.WithCause(forecasting.ErrJobConflict, err)
		}
		return forecastingError(err)
	}, model)
}
