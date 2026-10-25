package companycreation

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/domain/account"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/companycreation/record"
)

func fromRecord(r record.Command) domain.CompanyCreation {
	return domain.CompanyCreation{ID: r.ID, CompanyID: r.CompanyID, InitiatorID: r.InitiatorID, Key: r.Key, Spec: domain.CompanySpec{Name: r.Name, INN: r.INN.String, Currency: r.Currency, Timezone: r.Timezone}, RequestSHA256: r.RequestSHA256, State: r.State, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, NextAttemptAt: r.NextAttemptAt, Attempts: r.Attempts, LeaseToken: r.LeaseToken.UUID, LeaseExpiresAt: r.LeaseExpiresAt, AccountID: r.AccountID.UUID, CommittedAt: r.CommittedAt}
}

func (r *repository) InsertCreation(ctx context.Context, command domain.CompanyCreation) error {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, insertSQL, command.ID, command.CompanyID, command.InitiatorID, command.Key, command.Spec.Name, command.Spec.INN, command.Spec.Currency, command.Spec.Timezone, command.RequestSHA256, command.CreatedAt)
	return storageError(err)
}

func (r *repository) FindCreation(ctx context.Context, id uuid.UUID, lock bool) (domain.CompanyCreation, error) {
	query := findSQL
	if lock {
		query = findLockedSQL
	}
	return r.find(ctx, query, id)
}

func (r *repository) FindCreationByKey(ctx context.Context, initiator uuid.UUID, key string) (domain.CompanyCreation, error) {
	return r.find(ctx, findByKeySQL, initiator, key)
}

func (r *repository) find(ctx context.Context, query string, args ...any) (domain.CompanyCreation, error) {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	var row record.Command
	if err := sqlx.GetContext(ctx, r.db, &row, query, args...); err != nil {
		return domain.CompanyCreation{}, storageError(err)
	}
	return fromRecord(row), nil
}

// CompleteCreation and InsertOwner must share the caller's transaction and user lock.
// A nil lease is the browser path. Workers supply their current unexpired token.
func (r *repository) CompleteCreation(ctx context.Context, id, accountID uuid.UUID, committed time.Time, lease uuid.UUID) error {
	return r.update(ctx, completeSQL, id, accountID, committed, nullableUUID(lease))
}

func (r *repository) QuarantineCreation(ctx context.Context, id, lease uuid.UUID) error {
	return r.update(ctx, quarantineSQL, id, nullableUUID(lease))
}

// ClaimCreation commits its lease before any network call or user lock is taken.
func (r *repository) ClaimCreation(ctx context.Context, token uuid.UUID) (domain.CompanyCreation, error) {
	return r.find(ctx, claimSQL, token)
}

func (r *repository) RetryCreation(ctx context.Context, id, lease uuid.UUID, delay time.Duration) error {
	return r.update(ctx, retrySQL, id, lease, delay.Microseconds())
}

func (r *repository) update(ctx context.Context, query string, args ...any) error {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return storageError(err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return storageError(err)
	}
	if count != 1 {
		return domain.ErrLeaseLost
	}
	return nil
}

func nullableUUID(id uuid.UUID) any {
	if id == uuid.Nil {
		return nil
	}
	return id
}
