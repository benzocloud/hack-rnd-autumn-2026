package postgres

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/migrations"
)

// Migrate is an explicit admin operation, never run implicitly by the server.
// Provider-local state avoids goose's mutable package-level configuration.
func Migrate(ctx context.Context, db *sqlx.DB, queryTimeout time.Duration) error {
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return errors.New("auth migration lock configuration failed")
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db.DB, migrations.Files,
		goose.WithDisableGlobalRegistry(true), goose.WithSessionLocker(boundedSessionLocker{SessionLocker: locker, timeout: queryTimeout}),
		goose.WithSlog(slog.New(slog.DiscardHandler)))
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
