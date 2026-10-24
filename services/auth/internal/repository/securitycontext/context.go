package securitycontext

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/domain/account"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/securitycontext/record"
)

func (r *repository) FindContext(ctx context.Context, hash []byte, lock bool) (domain.SecurityContext, error) {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	q := findContextSQL
	if lock {
		q = findContextLockedSQL
	}
	var row record.Context
	if err := sqlx.GetContext(ctx, r.db, &row, q, hash); err != nil {
		return domain.SecurityContext{}, storageError(err)
	}
	result := domain.SecurityContext{ID: row.ID, Kind: row.Kind, TokenHash: row.TokenHash, CSRFNonce: row.CSRFNonce, CSRFKeyID: row.CSRFKeyID, CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt, UserID: row.UserID, CredentialVersion: row.CredentialVersion, LastSeenAt: row.LastSeenAt}
	if row.RevokedAt.Valid {
		result.RevokedAt = &row.RevokedAt.Time
	}
	return result, nil
}
func (r *repository) InsertContext(ctx context.Context, c domain.SecurityContext) error {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	if _, err := r.db.ExecContext(ctx, insertContextSQL, c.ID, c.Kind, c.TokenHash, c.CSRFNonce, c.CSRFKeyID, c.CreatedAt, c.ExpiresAt); err != nil {
		return storageError(err)
	}
	if c.Kind == "session" {
		_, err := r.db.ExecContext(ctx, insertSessionSQL, c.ID, c.UserID, c.CredentialVersion, c.LastSeenAt)
		return storageError(err)
	}
	return nil
}
func (r *repository) RevokeContext(ctx context.Context, id uuid.UUID, now time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, revokeContextSQL, id, now)
	return storageError(err)
}
func (r *repository) TouchSession(ctx context.Context, id uuid.UUID, now time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, touchSessionSQL, id, now)
	return storageError(err)
}
