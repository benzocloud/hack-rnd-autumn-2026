package challenge

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/domain/account"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/challenge/record"
)

func (r *repository) FindChallenge(ctx context.Context, id uuid.UUID, lock bool) (domain.Challenge, error) {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	q := findChallengeSQL
	if lock {
		q = findChallengeLockedSQL
	}
	var row record.Challenge
	if err := sqlx.GetContext(ctx, r.db, &row, q, id); err != nil {
		return domain.Challenge{}, storageError(err)
	}
	result := domain.Challenge{ID: row.ID, UserID: row.UserID, Purpose: row.Purpose, CodeHMAC: row.CodeHMAC, CodeKeyID: row.CodeKeyID, FailedAttempts: row.FailedAttempts, CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt}
	if row.ConsumedAt.Valid {
		result.ConsumedAt = &row.ConsumedAt.Time
	}
	if row.RevokedAt.Valid {
		result.RevokedAt = &row.RevokedAt.Time
	}
	return result, nil
}
func (r *repository) RevokeChallenges(ctx context.Context, id uuid.UUID, now time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, revokeChallengesSQL, id, now)
	return storageError(err)
}
func (r *repository) InsertChallenge(ctx context.Context, c domain.Challenge) error {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, insertChallengeSQL, c.ID, c.UserID, c.Purpose, c.CodeHMAC, c.CodeKeyID, c.CreatedAt, c.ExpiresAt)
	return storageError(err)
}
func (r *repository) SaveChallenge(ctx context.Context, c domain.Challenge) error {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, saveChallengeSQL, c.ID, c.FailedAttempts, c.ConsumedAt)
	return storageError(err)
}
