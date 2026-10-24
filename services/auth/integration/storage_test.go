//go:build integration

package integration_test

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jmoiron/sqlx"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/app"
	apppostgres "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/app/postgres"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/config"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/postgres"
	userrepo "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/user"
)

// A random schema is owned by each test. Cleanup never drops the provided DB
// or pre-existing tables. The configured database must be a scratch *_test DB.
func scratch(t *testing.T) (config.Config, *pgx.Conn) {
	t.Helper()
	raw := os.Getenv("AUTH_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("set AUTH_TEST_DATABASE_URL to a PostgreSQL 18 scratch database")
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.HasSuffix(u.Path, "_test") {
		t.Fatal("integration DB must have a name ending in _test")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, raw)
	if err != nil {
		t.Fatal("scratch database connection failed")
	}
	id, err := uuid.NewRandom()
	if err != nil {
		_ = admin.Close(ctx)
		t.Fatal(err)
	}
	schema := "auth_test_" + strings.ReplaceAll(id.String(), "-", "")
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		_ = admin.Close(ctx)
		t.Fatal("create scratch schema failed")
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := admin.Exec(cleanup, "DROP SCHEMA "+quoted+" CASCADE")
		if err != nil {
			t.Error("scratch schema cleanup failed")
		}
		if err := admin.Close(cleanup); err != nil {
			t.Error("scratch connection cleanup failed")
		}
	})
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	env := map[string]string{"AUTH_ENV": "local", "AUTH_DATABASE_URL": u.String(), "AUTH_QUERY_TIMEOUT": "3s", "AUTH_STARTUP_TIMEOUT": "5s"}
	cfg, err := config.Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	connection, err := pgx.Connect(ctx, u.String())
	if err != nil {
		t.Fatal("scratch schema connection failed")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := connection.Close(ctx); err != nil {
			t.Error("test connection cleanup failed")
		}
	})
	var version int
	if err := connection.QueryRow(ctx, "SELECT current_setting('server_version_num')::integer").Scan(&version); err != nil || version < 180000 || version >= 190000 {
		t.Fatal("tests require PostgreSQL 18")
	}
	return cfg, connection
}

func openDatabase(t *testing.T, cfg config.Config) *sqlx.DB {
	t.Helper()
	db, err := apppostgres.Open(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error("test pool cleanup failed")
		}
	})
	return db
}

func TestMigrationsStorageRestartAndRollback(t *testing.T) {
	cfg, db := scratch(t)
	pool := openDatabase(t, cfg)
	if !errors.Is(postgres.NewSchemaRepository(pool, cfg.QueryTimeout).CheckSchema(t.Context()), postgres.ErrSchema) {
		t.Fatal("empty schema reported ready")
	}
	if err := app.Migrate(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	if err := app.Migrate(t.Context(), cfg); err != nil {
		t.Fatal("second migration was not idempotent")
	}
	if err := postgres.NewSchemaRepository(pool, cfg.QueryTimeout).CheckSchema(t.Context()); err != nil {
		t.Fatal(err)
	}
	var tables int
	if err := db.QueryRow(t.Context(), "SELECT count(*) FROM information_schema.tables WHERE table_schema=current_schema()").Scan(&tables); err != nil || tables != 7 {
		t.Fatalf("expected six Auth tables and goose history, got %d", tables)
	}
	id, err := uuid.NewRandom()
	if err != nil {
		t.Fatal(err)
	}
	user := userrepo.User{ID: id, Email: "owner@example.test", DisplayName: "Иван"}
	if err := postgres.WithTx(t.Context(), pool, cfg.QueryTimeout, func(ctx context.Context, tx *sqlx.Tx) error {
		return userrepo.NewRepository(tx, cfg.QueryTimeout).InsertUser(ctx, user)
	}); err != nil {
		t.Fatal(err)
	}
	if err := pool.Close(); err != nil {
		t.Fatal(err)
	}
	pool = openDatabase(t, cfg)
	store := userrepo.NewRepository(pool, cfg.QueryTimeout)
	got, err := store.UserByEmail(t.Context(), user.Email)
	if err != nil || got.ID != id || got.Email != user.Email || got.DisplayName != user.DisplayName ||
		got.CredentialVersion != 1 || got.EmailVerified || got.State != "active" || got.CreatedAt.IsZero() {
		t.Fatal("reopened repository lost or changed user")
	}
	if _, err := store.UserByEmail(t.Context(), "absent@example.test"); !errors.Is(err, userrepo.ErrNotFound) {
		t.Fatal("missing user not distinguished")
	}
	sentinel := errors.New("rollback requested")
	rollbackUser := userrepo.User{ID: uuid.New(), Email: "rollback@example.test", DisplayName: "Rollback"}
	err = postgres.WithTx(t.Context(), pool, cfg.QueryTimeout, func(ctx context.Context, tx *sqlx.Tx) error {
		if err := userrepo.NewRepository(tx, cfg.QueryTimeout).InsertUser(ctx, rollbackUser); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatal("transaction lost callback error")
	}
	if _, err := store.UserByEmail(t.Context(), rollbackUser.Email); !errors.Is(err, userrepo.ErrNotFound) {
		t.Fatal("callback error did not roll back")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := postgres.WithTx(ctx, pool, cfg.QueryTimeout, func(context.Context, *sqlx.Tx) error {
		t.Fatal("cancelled transaction entered callback")
		return nil
	}); err == nil {
		t.Fatal("cancelled transaction succeeded")
	}

	ctx, cancel = context.WithCancel(t.Context())
	defer cancel()
	cancelledUser := userrepo.User{ID: uuid.New(), Email: "cancelled@example.test", DisplayName: "Cancelled"}
	err = postgres.WithTx(ctx, pool, cfg.QueryTimeout, func(ctx context.Context, tx *sqlx.Tx) error {
		if err := userrepo.NewRepository(tx, cfg.QueryTimeout).InsertUser(ctx, cancelledUser); err != nil {
			return err
		}
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation before commit: got %v, want context.Canceled", err)
	}
	if _, err := store.UserByEmail(t.Context(), cancelledUser.Email); !errors.Is(err, userrepo.ErrNotFound) {
		t.Fatal("cancellation before commit did not roll back inserted user")
	}
}

func TestConcurrentEmailUniquenessAndStorageConstraints(t *testing.T) {
	cfg, db := scratch(t)
	if err := app.Migrate(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	pool := openDatabase(t, cfg)
	store := userrepo.NewRepository(pool, cfg.QueryTimeout)
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			results <- postgres.WithTx(t.Context(), pool, cfg.QueryTimeout, func(ctx context.Context, tx *sqlx.Tx) error {
				return userrepo.NewRepository(tx, cfg.QueryTimeout).InsertUser(ctx, userrepo.User{ID: uuid.New(), Email: "same@example.test", DisplayName: "Same"})
			})
		})
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, userrepo.ErrEmailExists):
			conflict++
		default:
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("concurrent unique email did not produce one winner")
	}
	for _, tc := range []struct {
		name  string
		id    uuid.UUID
		email string
	}{
		{"zero ID", uuid.Nil, "zero@example.test"},
		{"wrong version", uuid.MustParse("0192b9be-1234-7000-8000-000000000001"), "wrong@example.test"},
		{"unnormalized email", uuid.New(), "Mixed@Example.test"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := postgres.WithTx(t.Context(), pool, cfg.QueryTimeout, func(ctx context.Context, tx *sqlx.Tx) error {
				return userrepo.NewRepository(tx, cfg.QueryTimeout).InsertUser(ctx, userrepo.User{ID: tc.id, Email: tc.email, DisplayName: "Test"})
			})
			if !errors.Is(err, userrepo.ErrInvalidRecord) {
				t.Fatal("invalid ID/email passed database constraints")
			}
		})
	}
	user, err := store.UserByEmail(t.Context(), "same@example.test")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, sql string
		args      []any
	}{
		{"zero credential version", "UPDATE users SET credential_version=0 WHERE id=$1", []any{user.ID}},
		{"plaintext password", "INSERT INTO credentials(user_id,password_hash) VALUES($1,'plaintext')", []any{user.ID}},
		{"overlong challenge", "INSERT INTO verification_challenges(id,user_id,code_hmac,code_key_id,expires_at) VALUES($1,$2,$3,'key',now()+interval '11 minutes')", []any{uuid.New(), user.ID, make([]byte, 32)}},
		{"six failed attempts", "INSERT INTO verification_challenges(id,user_id,code_hmac,code_key_id,failed_attempts,expires_at) VALUES($1,$2,$3,'key',6,now()+interval '10 minutes')", []any{uuid.New(), user.ID, make([]byte, 32)}},
		{"oversized reset TTL", "INSERT INTO reset_tokens(id,user_id,token_hash,expires_at) VALUES($1,$2,$3,now()+interval '16 minutes')", []any{uuid.New(), user.ID, make([]byte, 32)}},
		{"short context hash", "INSERT INTO security_contexts(id,kind,token_hash,csrf_nonce,csrf_key_id,expires_at) VALUES($1,'pre_auth',$2,$3,'key',now()+interval '10 minutes')", []any{uuid.New(), make([]byte, 1), make([]byte, 32)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := db.Exec(t.Context(), tc.sql, tc.args...); err == nil {
				t.Fatal("invalid storage row accepted")
			}
		})
	}
	// A pre-auth context cannot accidentally become a live session row.
	contextID := uuid.New()
	if _, err := db.Exec(t.Context(), "INSERT INTO security_contexts(id,kind,token_hash,csrf_nonce,csrf_key_id,expires_at) VALUES($1,'pre_auth',$2,$3,'key',now()+interval '10 minutes')", contextID, make([]byte, 32), make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(t.Context(), "INSERT INTO sessions(context_id,user_id,credential_version) VALUES($1,$2,1)", contextID, user.ID); err == nil {
		t.Fatal("pre-auth became session")
	}
	if _, err := db.Exec(t.Context(), "UPDATE goose_db_version SET version_id=99 WHERE version_id=1"); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(postgres.NewSchemaRepository(pool, cfg.QueryTimeout).CheckSchema(t.Context()), postgres.ErrSchema) {
		t.Fatal("future schema version reported ready")
	}
}

func TestConcurrentMigrationCommands(t *testing.T) {
	cfg, _ := scratch(t)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	results := make(chan error, 2)
	for range 2 {
		go func() { results <- app.Migrate(ctx, cfg) }()
	}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("concurrent migration failed: %v", err)
		}
	}
	pool := openDatabase(t, cfg)
	if err := postgres.NewSchemaRepository(pool, cfg.QueryTimeout).CheckSchema(t.Context()); err != nil {
		t.Fatal(err)
	}
}
