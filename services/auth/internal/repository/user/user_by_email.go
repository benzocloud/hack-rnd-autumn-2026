package user

import (
	"context"

	"github.com/jmoiron/sqlx"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/user/record"
)

func (r *repository) UserByEmail(ctx context.Context, email string) (User, error) {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	var row record.User
	if err := sqlx.GetContext(ctx, r.db, &row, userByEmailSQL, email); err != nil {
		return User{}, storageError(err)
	}
	return fromRecord(row), nil
}
