package app

import (
	"context"
	"time"

	appPostgres "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/app/postgres"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/config"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/postgres"
)

// Migrate owns its separate admin pool; the server never calls it on startup.
func Migrate(ctx context.Context, cfg config.Config) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	// The session lock holds one connection; migrations need a second one.
	cfg.MaxConnections = 2
	db, err := appPostgres.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	return postgres.Migrate(ctx, db, cfg.QueryTimeout)
}
