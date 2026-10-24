package app

import (
	"context"

	"github.com/jmoiron/sqlx"

	appPostgres "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/app/postgres"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/config"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/postgres"
)

type schemaReader interface {
	CheckSchema(context.Context) error
}

type databaseResources struct {
	db     *sqlx.DB
	schema schemaReader
}

func openDatabase(ctx context.Context, cfg config.Config) (database, error) {
	db, err := appPostgres.Open(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &databaseResources{db: db, schema: postgres.NewRepository(db, cfg.QueryTimeout)}, nil
}

func (r *databaseResources) Close() { _ = r.db.Close() }

func (r *databaseResources) Ping(ctx context.Context) error { return r.db.PingContext(ctx) }

func (r *databaseResources) CheckSchema(ctx context.Context) error { return r.schema.CheckSchema(ctx) }
