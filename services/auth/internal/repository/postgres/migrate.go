package postgres

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/config"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/migrations"
)

// Migrate is an explicit admin operation, never run implicitly by the server.
// Provider-local state avoids goose's mutable package-level configuration.
func Migrate(ctx context.Context, cfg config.Config) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	poolCfg, err := cfg.PoolConfig()
	if err != nil {
		return err
	}
	db := stdlib.OpenDB(*poolCfg.ConnConfig)
	defer db.Close()
	// The session lock holds one connection; migrations need a second one.
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(0)
	db.SetConnMaxLifetime(5 * time.Minute)
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return errors.New("auth migration lock configuration failed")
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Files,
		goose.WithDisableGlobalRegistry(true), goose.WithSessionLocker(boundedSessionLocker{SessionLocker: locker, timeout: cfg.QueryTimeout}),
		goose.WithSlog(slog.New(slog.NewTextHandler(io.Discard, nil))))
	if err != nil {
		return errors.New("auth migration configuration failed")
	}
	if _, err := provider.Up(ctx); err != nil {
		return errors.New("auth migration failed")
	}
	return nil
}

// Goose detaches its unlock context. Restore a deadline around the SQL call,
// since the upstream retry-count limit does not bound a stalled network query.
type boundedSessionLocker struct {
	lock.SessionLocker
	timeout time.Duration
}

func (l boundedSessionLocker) SessionUnlock(ctx context.Context, conn *sql.Conn) error {
	ctx, cancel := context.WithTimeout(ctx, l.timeout)
	defer cancel()
	return l.SessionLocker.SessionUnlock(ctx, conn)
}
