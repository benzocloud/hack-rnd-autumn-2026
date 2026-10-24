package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/jmoiron/sqlx"

	appPostgres "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/app/postgres"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
	forecastDomain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	ledgerDomain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	commandsSQL "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres/commands"
	forecastSQL "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres/forecasting"
	ledgerSQL "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres/ledger"
	paymentsSQL "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres/payments"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/payments"
)

type ledgerOperations interface {
	CreateTransaction(context.Context, ledger.TransactionCommand) (ledger.CreateTransactionResult, error)
	RecordBalance(context.Context, ledger.BalanceCommand) (ledger.RecordBalanceResult, error)
}

type paymentsOperations interface {
	CreatePayment(context.Context, payments.CreateCommand) (payments.CreatePaymentResult, error)
	MatchPayment(context.Context, payments.MatchCommand) (payments.MatchPaymentResult, error)
}

type forecastingOperations interface {
	CaptureSnapshot(context.Context, forecasting.CaptureCommand) (forecastDomain.FinancialSnapshot, error)
	GetSnapshot(context.Context, ledgerDomain.Scope, uuid.UUID) (forecastDomain.FinancialSnapshot, error)
}

type application struct {
	Ledger      ledgerOperations
	Payments    paymentsOperations
	Forecasting forecastingOperations
	db          *sqlx.DB
}

// New owns the opened pool. Call Close after operations have finished.
// Migrations and transports are intentionally separate from application construction.
func New(ctx context.Context, cfg config.Config) (*application, error) {
	db, err := appPostgres.Open(ctx, cfg.Postgres)
	if err != nil {
		return nil, err
	}
	return &application{
		db: db,
		Ledger: ledger.New(func(ctx context.Context, work ledger.Work) error {
			err := appPostgres.WithinTx(ctx, db, cfg.Postgres.QueryTimeout, func(ctx context.Context, tx *sqlx.Tx) error {
				return work(ctx, ledgerSQL.New(tx), commandsSQL.New(tx))
			})
			return ledgerError(err)
		}),
		Payments: payments.New(func(ctx context.Context, work payments.Work) error {
			err := appPostgres.WithinTx(ctx, db, cfg.Postgres.QueryTimeout, func(ctx context.Context, tx *sqlx.Tx) error {
				return work(ctx, paymentsSQL.New(tx), commandsSQL.New(tx))
			})
			return paymentsError(err)
		}),
		Forecasting: forecasting.New(func(ctx context.Context, work forecasting.Work) error {
			err := appPostgres.WithinRepeatableRead(ctx, db, cfg.Postgres.QueryTimeout,
				func(ctx context.Context, tx *sqlx.Tx) error {
					return work(ctx, forecastSQL.New(tx))
				})
			return forecastingError(err)
		}),
	}, nil
}

func (a *application) Close() error { return a.db.Close() }
