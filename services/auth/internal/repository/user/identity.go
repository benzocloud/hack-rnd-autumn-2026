package user

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/domain/account"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/user/record"
)

func identityError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return domain.ErrUnavailable
}
func (r *repository) FindUserByEmail(ctx context.Context, email string) (domain.User, error) {
	return r.identity(ctx, identityByEmailSQL, email)
}
func (r *repository) FindUserByID(ctx context.Context, id uuid.UUID, lock bool) (domain.User, error) {
	q := identityByIdSQL
	if lock {
		q = identityByIdLockedSQL
	}
	return r.identity(ctx, q, id)
}
func (r *repository) identity(ctx context.Context, q string, arg any) (domain.User, error) {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	var row record.Identity
	if err := sqlx.GetContext(ctx, r.db, &row, q, arg); err != nil {
		return domain.User{}, identityError(err)
	}
	result := domain.User{ID: row.ID, Email: row.Email, DisplayName: row.DisplayName, PasswordHash: row.PasswordHash, State: row.State, EmailVerified: row.EmailVerified, CredentialVersion: row.CredentialVersion, EmailVerificationMethod: row.EmailVerificationMethod, VerificationFailures: row.VerificationFailures}
	if row.VerificationWindowStartedAt.Valid {
		result.VerificationWindowStartedAt = &row.VerificationWindowStartedAt.Time
	}
	return result, nil
}
func (r *repository) InsertIdentity(ctx context.Context, user domain.User) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	var id uuid.UUID
	err := sqlx.GetContext(ctx, r.db, &id, insertIdentitySQL, user.ID, user.Email, user.DisplayName)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, identityError(err)
	}
	_, err = r.db.ExecContext(ctx, insertCredentialSQL, user.ID, user.PasswordHash)
	if err != nil {
		return false, identityError(err)
	}
	return true, nil
}
func (r *repository) SetEmailVerified(ctx context.Context, id uuid.UUID, method string) error {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, setEmailVerifiedSQL, id, method)
	return identityError(err)
}
func (r *repository) RecordVerificationFailure(ctx context.Context, id uuid.UUID, now time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, recordVerificationFailureSQL, id, now)
	return identityError(err)
}
