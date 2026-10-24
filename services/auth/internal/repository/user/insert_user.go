package user

import "context"

// InsertUser is a storage primitive; it does not create credentials/consent/outbox.
func (r *repository) InsertUser(ctx context.Context, user User) error {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, insertUserSQL, user.ID, user.Email, user.DisplayName)
	return storageError(err)
}
