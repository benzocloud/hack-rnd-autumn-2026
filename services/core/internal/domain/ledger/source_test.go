package ledger_test

import (
	"time"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

func importSource(system, company, id string) ledger.Source {
	return ledger.Source{Kind: ledger.Imported, Namespace: ledger.Namespace{System: system, CompanyID: company},
		ExternalID: id, ImportedAt: time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)}
}
