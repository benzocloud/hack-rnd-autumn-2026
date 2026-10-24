package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
)

func TestOpenRejectsInvalidPoolSettings(t *testing.T) {
	t.Parallel()
	cfg := config.Postgres{Host: "127.0.0.1", Port: 5432, User: "core", Password: "secret",
		Database: "core", SSLMode: "disable", MaxConns: 2, MaxIdleConns: 3,
		ConnectTimeout: time.Second, QueryTimeout: time.Second}
	db, err := Open(t.Context(), cfg)
	if db != nil || !errors.Is(err, errInvalidConfig) {
		t.Fatalf("invalid idle limit accepted: %v, %v", db, err)
	}
}

func TestOpenCancellationDoesNotExposeCredentials(t *testing.T) {
	t.Parallel()
	cfg := config.Postgres{Host: "127.0.0.1", Port: 5432, User: "core", Password: "private-password",
		Database: "core", SSLMode: "disable", MaxConns: 2,
		ConnectTimeout: time.Second, QueryTimeout: time.Second}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	db, err := Open(ctx, cfg)
	if db != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled startup: %v, %v", db, err)
	}
	if strings.Contains(err.Error(), cfg.Password) || strings.Contains(err.Error(), cfg.DSN()) {
		t.Fatal("startup error exposes credentials")
	}
}
