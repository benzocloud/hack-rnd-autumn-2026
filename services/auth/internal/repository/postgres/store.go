// Package postgres owns the Auth pool and parameterized persistence operations.
package postgres

import (
	"context"
	_ "embed"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/config"
)

var (
	ErrUnavailable   = errors.New("auth database unavailable")
	ErrSchema        = errors.New("auth database schema does not match this binary; run migrations")
	ErrNotFound      = errors.New("auth user not found")
	ErrEmailExists   = errors.New("auth email already exists")
	ErrInvalidRecord = errors.New("invalid auth storage record")
)

const SchemaVersion = 1

type Store struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func Open(ctx context.Context, cfg config.Config) (*Store, error) {
	poolCfg, err := cfg.PoolConfig()
	if err != nil {
		return nil, err
	}
	startup, cancel := context.WithTimeout(ctx, cfg.StartupTimeout)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(startup, poolCfg)
	if err != nil {
		return nil, ErrUnavailable
	}
	if err := pool.Ping(startup); err != nil {
		pool.Close()
		return nil, ErrUnavailable
	}
	return &Store{pool: pool, queryTimeout: cfg.QueryTimeout}, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	if err := s.pool.Ping(ctx); err != nil {
		return ErrUnavailable
	}
	return nil
}

func (s *Store) CheckSchema(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	var version int64
	var applied bool
	if err := s.pool.QueryRow(ctx, "SELECT version_id, is_applied FROM goose_db_version ORDER BY id DESC LIMIT 1").Scan(&version, &applied); err != nil {
		return ErrSchema
	}
	if version != SchemaVersion || !applied {
		return ErrSchema
	}
	return nil
}

// UserRecord is a storage projection, not a public DTO or a registration use case.
// Credentials/consent/outbox are deliberately not created by InsertUser.
type UserRecord struct {
	ID                uuid.UUID
	Email             string
	DisplayName       string
	EmailVerified     bool
	CredentialVersion int64
	State             string
	CreatedAt         time.Time
}

//go:embed sql/insert_user.sql
var insertUserSQL string

//go:embed sql/user_by_email.sql
var userByEmailSQL string

// WithTx supplies one bounded transaction for future multi-record use cases.
// A callback error or cancellation rolls back; a successful return commits.
func (s *Store) WithTx(ctx context.Context, fn func(context.Context, *Tx) error) error {
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return storageError(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.queryTimeout)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if err := fn(ctx, &Tx{tx: tx, queryTimeout: s.queryTimeout}); err != nil {
		return err
	}
	return storageError(tx.Commit(ctx))
}

type Tx struct {
	tx           pgx.Tx
	queryTimeout time.Duration
}

func (tx *Tx) InsertUser(ctx context.Context, user UserRecord) error {
	ctx, cancel := context.WithTimeout(ctx, tx.queryTimeout)
	defer cancel()
	_, err := tx.tx.Exec(ctx, insertUserSQL, user.ID, user.Email, user.DisplayName)
	return storageError(err)
}

func (s *Store) UserByEmail(ctx context.Context, email string) (UserRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	var user UserRecord
	err := s.pool.QueryRow(ctx, userByEmailSQL, email).Scan(&user.ID, &user.Email, &user.DisplayName, &user.EmailVerified, &user.CredentialVersion, &user.State, &user.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserRecord{}, ErrNotFound
	}
	if err != nil {
		return UserRecord{}, storageError(err)
	}
	return user, nil
}

// Do not wrap PgError.Detail or connection errors: they can contain email/DSN.
func storageError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" && pgErr.ConstraintName == "users_email_key" {
			return ErrEmailExists
		}
		if pgErr.Code == "23514" || pgErr.Code == "23502" || pgErr.Code == "23503" || pgErr.Code == "22P02" {
			return ErrInvalidRecord
		}
	}
	return ErrUnavailable
}
