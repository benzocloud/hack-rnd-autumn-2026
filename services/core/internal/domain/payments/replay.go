package payments

import (
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

// Reimport preserves current remaining/status/version and editable classification.
func (p ExpectedPayment) CheckReplay(stored ExpectedPayment) (bool, error) {
	if p.data.Scope != stored.data.Scope {
		return false, ledger.ErrScope
	}
	if p.data.Source.Kind != ledger.Imported || stored.data.Source.Kind != ledger.Imported ||
		p.data.Source.Namespace != stored.data.Source.Namespace || p.data.Source.ExternalID != stored.data.Source.ExternalID {
		return false, nil
	}
	left, right := p.data, stored.data
	left.ID, right.ID = uuid.Nil, uuid.Nil
	left.CreatedAt, right.CreatedAt = time.Time{}, time.Time{}
	left.Source.ImportedAt, right.Source.ImportedAt = time.Time{}, time.Time{}
	left.Category, right.Category = ledger.Category{}, ledger.Category{}
	if left != right {
		return false, ledger.ErrExternalRecord
	}
	return true, nil
}
