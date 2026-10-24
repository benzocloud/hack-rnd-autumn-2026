package consent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/jmoiron/sqlx"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/domain/account"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/consent/record"
)

// EnsureDocument bootstraps an empty registry without overwriting an active version.
func (r *repository) EnsureDocument(ctx context.Context, document domain.ConsentDocument) error {
	sum := sha256.Sum256([]byte(document.Text))
	if document.Version == "" || document.Text == "" || hex.EncodeToString(sum[:]) != document.SHA256 {
		return domain.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, ensureDocumentSQL, document.Version, document.SHA256, document.Text)
	return storageError(err)
}
func (r *repository) GetActiveDocument(ctx context.Context, lock bool) (domain.ConsentDocument, error) {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	q := activeDocumentSQL
	if lock {
		q = activeDocumentLockedSQL
	}
	var row record.Document
	if err := sqlx.GetContext(ctx, r.db, &row, q); err != nil {
		return domain.ConsentDocument{}, storageError(err)
	}
	return domain.ConsentDocument{Text: row.Text, Version: row.Version, SHA256: row.SHA256}, nil
}
