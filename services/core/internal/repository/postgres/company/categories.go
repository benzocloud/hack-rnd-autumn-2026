package company

import (
	"context"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
)

func (r *repository) SeedCategories(ctx context.Context, definitions []ledger.CategoryDefinition) error {
	for _, c := range definitions {
		if _, err := r.tx.ExecContext(ctx, seedCategorySQL, c.CompanyID, c.ID, c.Name, string(c.Direction), c.SemanticKind, c.CatalogueCode); err != nil {
			return storage.StorageError(err)
		}
	}
	return nil
}
