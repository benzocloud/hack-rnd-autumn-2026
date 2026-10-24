package imports

import (
	"context"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

func (r *repository) SeedCategories(ctx context.Context, definitions []ledger.CategoryDefinition) error {
	for _, c := range definitions {
		if _, err := r.tx.ExecContext(ctx, querySeedCategory, c.CompanyID, c.ID, c.Name, string(c.Direction), c.SemanticKind, c.CatalogueCode); err != nil {
			return classify(err)
		}
	}
	return nil
}
