package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/jmoiron/sqlx"

	appPostgres "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/app/postgres"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
	forecastDomain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	ledgerDomain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	paymentsDomain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
	commandsSQL "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres/commands"
	forecastSQL "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres/forecasting"
	ledgerSQL "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres/ledger"
	paymentsSQL "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres/payments"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/company"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/payments"
)

type ledgerOperations interface {
	CreateTransaction(context.Context, ledger.TransactionCommand) (ledger.CreateTransactionResult, error)
	RecordBalance(context.Context, ledger.BalanceCommand) (ledger.RecordBalanceResult, error)
	ReadAccount(context.Context, ledgerDomain.Scope, ledgerDomain.Date) (ledger.ReadAccountResult, error)
	RecordManualBalance(context.Context, ledger.ManualBalanceCommand) (ledger.RecordManualBalanceResult, error)
	CreateManualTransaction(context.Context, ledger.ManualTransactionCommand) (ledger.CreateManualTransactionResult, error)
}

type paymentsOperations interface {
	ReadPayment(context.Context, ledgerDomain.Scope, uuid.UUID) (paymentsDomain.ExpectedPayment, error)
	ListPayments(context.Context, payments.ListPaymentsCommand) (payments.ListPaymentsResult, error)
	ListPaymentMatches(context.Context, payments.ListMatchesCommand) (payments.ListMatchesResult, error)
	UpdatePayment(context.Context, payments.UpdatePaymentCommand) (payments.ChangePaymentResult, error)
	CancelPayment(context.Context, payments.CancelPaymentCommand) (payments.ChangePaymentResult, error)
	RemovePaymentMatch(context.Context, payments.RemoveMatchCommand) (payments.ChangePaymentResult, error)
	CreatePayment(context.Context, payments.CreateCommand) (payments.CreatePaymentResult, error)
	MatchPayment(context.Context, payments.MatchCommand) (payments.MatchPaymentResult, error)
}

type forecastingOperations interface {
	CaptureSnapshot(context.Context, forecasting.CaptureCommand) (forecastDomain.FinancialSnapshot, error)
	GetSnapshot(context.Context, ledgerDomain.Scope, uuid.UUID) (forecastDomain.FinancialSnapshot, error)
}

type application struct {
	Companies    companyOperations
	Ledger       ledgerOperations
	Payments     paymentsOperations
	Forecasting  forecastingOperations
	ForecastJobs forecastJobs
	Scenarios    scenarioOperations
	Imports      importOperations
	db           *sqlx.DB
}

// New owns the opened pool. Call Close after operations have finished.
// Migrations and transports are intentionally separate from application construction.
func New(ctx context.Context, cfg config.Config) (*application, error) {
	return NewWithCompanyAuthorizer(ctx, cfg, nil)
}

// NewWithCompanyAuthorizer binds the trusted Auth adapter supplied by runtime.
// The authorizer runs before opening a bootstrap transaction, including replay.
func NewWithCompanyAuthorizer(ctx context.Context, cfg config.Config, authorize func(context.Context, uuid.UUID, uuid.UUID, string) (company.Authorization, error)) (*application, error) {
	return NewWithForecastModel(ctx, cfg, authorize, nil)
}

func NewWithForecastModel(ctx context.Context, cfg config.Config, authorize func(context.Context, uuid.UUID, uuid.UUID, string) (company.Authorization, error), model financialPredictor) (*application, error) {
	db, err := appPostgres.Open(ctx, cfg.Postgres)
	if err != nil {
		return nil, err
	}
	importService, err := newImports(db, cfg.Postgres, cfg.FixtureCoverage...)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	jobs := newForecastJobs(db, cfg.Postgres, model)
	return &application{
		db:           db,
		Companies:    newCompanies(db, cfg.Postgres, authorize),
		ForecastJobs: jobs,
		Scenarios:    newScenarios(db, cfg.Postgres, jobs),
		Imports:      importService,
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
