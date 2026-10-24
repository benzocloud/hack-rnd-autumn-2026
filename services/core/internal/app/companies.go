package app

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	appPostgres "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/app/postgres"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/company"
	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
	companySQL "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres/company"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/company"
)

type companyOperations interface {
	Bootstrap(context.Context, company.BootstrapCommand) (company.BootstrapResult, error)
	GetBootstrapResult(context.Context, uuid.UUID) (company.BootstrapResult, error)
	ReadProfile(context.Context, uuid.UUID) (domain.Profile, error)
	Update(context.Context, company.UpdateCommand) (domain.Profile, error)
}

func newCompanies(db *sqlx.DB, cfg config.Postgres, authorize func(context.Context, uuid.UUID, uuid.UUID, string) (company.Authorization, error)) companyOperations {
	return company.New(func(ctx context.Context, work company.Work) error {
		err := appPostgres.WithinTx(ctx, db, cfg.QueryTimeout, func(ctx context.Context, tx *sqlx.Tx) error { return work(ctx, companySQL.New(tx)) })
		return companyError(err)
	}, authorize)
}
func companyError(err error) error {
	for _, pair := range []struct{ storage, application error }{
		{storage.ErrNotFound, company.ErrNotFound}, {storage.ErrConflict, company.ErrConflict}, {storage.ErrUnavailable, company.ErrUnavailable},
	} {
		if errors.Is(err, pair.storage) {
			return storage.WithCause(pair.application, err)
		}
	}
	return err
}
