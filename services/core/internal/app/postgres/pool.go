package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
)

// Open returns a ready pool. The application owns Close and passes the pool to repositories.
func Open(ctx context.Context, settings config.Postgres) (*sqlx.DB, error) {
	if err := validatePostgres(settings); err != nil {
		return nil, err
	}
	cfg, err := pgx.ParseConfig(settings.DSN())
	if err != nil {
		return nil, &connectionError{operation: "configuration", cause: err}
	}
	cfg.ConnectTimeout = settings.ConnectTimeout
	cfg.RuntimeParams["timezone"] = "UTC"
	cfg.RuntimeParams["application_name"] = "saldo-core"
	db := sqlx.NewDb(stdlib.OpenDB(*cfg), "pgx")
	db.SetMaxOpenConns(int(settings.MaxConns))
	db.SetMaxIdleConns(int(settings.MaxIdleConns))
	startup, cancel := context.WithTimeout(ctx, settings.ConnectTimeout)
	defer cancel()
	if err := db.PingContext(startup); err != nil {
		return nil, errors.Join(&connectionError{operation: "ping", cause: err}, db.Close())
	}
	return db, nil
}
