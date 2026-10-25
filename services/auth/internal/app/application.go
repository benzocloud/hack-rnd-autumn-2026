package app

import (
	"context"
	"errors"

	"github.com/jmoiron/sqlx"
	"google.golang.org/grpc"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/app/consent"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/config"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/challenge"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/companycreation"
	consentSQL "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/consent"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/membership"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/postgres"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/securitycontext"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/user"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/service/account"
)

type accountOperations interface {
	ReconcileOne(context.Context) (bool, error)
	BeginCompanyCreation(context.Context, account.BeginCompanyCreationInput) (account.CreationCommand, error)
	AuthorizeCompanyBootstrap(context.Context, account.AuthorizeCompanyBootstrapInput) (account.CreationCommand, error)
	CompleteCompanyCreation(context.Context, account.CompleteCompanyCreationInput) (account.CreationCommand, error)
	GetPersonalDataConsent(context.Context) (account.Document, error)
	EmailVerificationMode() string
	GetSecurityContext(context.Context, string) (account.SecurityContext, error)
	Register(context.Context, account.RegisterInput) (account.Challenge, error)
	Login(context.Context, account.LoginInput) (account.LoginResult, error)
	VerifyEmail(context.Context, account.VerifyEmailInput) error
	ResendVerification(context.Context, account.WriteContext, string) (account.Challenge, error)
	Logout(context.Context, account.WriteContext) error
	GetCurrentAccount(context.Context, string) (account.User, error)
	AuthorizeCompany(context.Context, account.AuthorizeCompanyInput) (account.Authorization, error)
	ListMemberships(context.Context, string, int, string) (account.MembershipPage, error)
}

type application struct {
	*databaseResources
	Account         accountOperations
	coreConnection  *grpc.ClientConn
	coreUnavailable bool
}

func (app *application) Close() {
	if app.coreConnection != nil {
		_ = app.coreConnection.Close()
	}
	app.databaseResources.Close()
}

// New owns the opened pool. Close it after all RPC and health calls have drained.
func New(ctx context.Context, cfg config.Config) (*application, error) {
	db, err := openDatabase(ctx, cfg)
	if err != nil {
		return nil, err
	}
	resources, ok := db.(*databaseResources)
	if !ok {
		db.Close()
		return nil, account.ErrUnavailable
	}
	app := &application{databaseResources: resources}
	if cfg.GRPCAddress == "" {
		return app, nil
	}
	if err := consentSQL.NewRepository(resources.db, cfg.QueryTimeout).EnsureDocument(ctx, consent.Document()); err != nil {
		resources.Close()
		return nil, err
	}
	options := account.Options{CodeKey: cfg.CodeHMACKey(), CSRFKey: cfg.CSRFHMACKey(), Demo: cfg.DemoEmailVerification, DemoEmails: cfg.DemoTestEmails, Consent: consent.Document()}
	options = app.configureCore(cfg, options)

	accounts, err := account.New(func(ctx context.Context, work account.Work) error {
		err := postgres.WithTx(ctx, resources.db, cfg.QueryTimeout, func(ctx context.Context, tx *sqlx.Tx) error {
			return work(ctx, user.NewRepository(tx, cfg.QueryTimeout), securitycontext.NewRepository(tx, cfg.QueryTimeout), challenge.NewRepository(tx, cfg.QueryTimeout), consentSQL.NewRepository(tx, cfg.QueryTimeout), membership.NewRepository(tx, cfg.QueryTimeout), companycreation.NewRepository(tx, cfg.QueryTimeout))
		})
		if errors.Is(err, postgres.ErrUnavailable) {
			return account.ErrUnavailable
		}
		return err
	}, options)
	if err != nil {
		app.Close()
		return nil, err
	}
	app.Account = accounts
	return app, nil
}
