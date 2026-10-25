package membership

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/domain/account"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/membership/record"
)

func fromRecord(row record.Membership) domain.Membership {
	return domain.Membership{CompanyID: row.CompanyID, UserID: row.UserID, Role: row.Role, CreatedAt: row.CreatedAt}
}

// InsertOwner never promotes an existing financier or replaces another owner.
// The caller holds the user lock and completes its command in this same Tx.
func (r *repository) InsertOwner(ctx context.Context, company, user uuid.UUID, created time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	result, err := r.db.ExecContext(ctx, insertOwnerSQL, company, user, created)
	if err != nil {
		return storageError(err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return storageError(err)
	}
	if count != 1 {
		return domain.ErrConflict
	}
	return nil
}
func (r *repository) FindMembership(ctx context.Context, company, user uuid.UUID) (domain.Membership, error) {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	var row record.Membership
	if err := sqlx.GetContext(ctx, r.db, &row, findMembershipSQL, company, user); err != nil {
		return domain.Membership{}, storageError(err)
	}
	return fromRecord(row), nil
}
func (r *repository) ListMemberships(ctx context.Context, user, after uuid.UUID, created *time.Time, limit int) ([]domain.Membership, error) {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	var rows []record.Membership
	if err := sqlx.SelectContext(ctx, r.db, &rows, listMembershipsSQL, user, created, after, limit); err != nil {
		return nil, storageError(err)
	}
	result := make([]domain.Membership, 0, len(rows))
	for _, row := range rows {
		result = append(result, fromRecord(row))
	}
	return result, nil
}
