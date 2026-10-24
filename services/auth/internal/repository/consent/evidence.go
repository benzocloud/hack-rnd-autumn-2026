package consent

import (
	"context"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/domain/account"
)

func (r *repository) InsertEvidence(ctx context.Context, e domain.ConsentEvidence) error {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, insertEvidenceSQL, e.ID, e.UserID, e.Document.Version, e.Document.SHA256, e.Document.Text, e.AcceptedAt)
	return storageError(err)
}
