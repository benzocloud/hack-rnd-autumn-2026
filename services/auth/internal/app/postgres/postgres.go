package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/config"
)

// Open configures and checks the pool. The composing application owns Close.
func Open(ctx context.Context, cfg config.Config) (*sqlx.DB, error) {
	connection, err := cfg.ConnectionConfig()
	if err != nil {
		return nil, err
	}
	db := sqlx.NewDb(stdlib.OpenDB(*connection), "pgx")
	db.SetMaxOpenConns(cfg.MaxConnections)
	db.SetMaxIdleConns(cfg.MaxConnections)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)
	startup, cancel := context.WithTimeout(ctx, cfg.StartupTimeout)
	defer cancel()
	if err := db.PingContext(startup); err != nil {
		_ = db.Close()
		return nil, errors.New("auth database unavailable")
	}
	return db, nil
}
