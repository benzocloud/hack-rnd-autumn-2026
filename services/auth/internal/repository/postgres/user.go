package postgres

import (
	"context"

	"github.com/jmoiron/sqlx"
)

// InsertUser is a storage primitive; it does not create credentials/consent/outbox.
func (r *repository) InsertUser(ctx context.Context, user UserRecord) error {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, insertUserSQL, user.ID, user.Email, user.DisplayName)
	return storageError(err)
}

func (r *repository) UserByEmail(ctx context.Context, email string) (UserRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	var row userRow
	if err := sqlx.GetContext(ctx, r.db, &row, userByEmailSQL, email); err != nil {
		return UserRecord{}, storageError(err)
	}
	return row.record(), nil
}
