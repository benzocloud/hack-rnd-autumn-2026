package forecasting

import (
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
)

func scanFact(rows *sqlx.Rows, scope ledger.Scope) (domain.Fact, error) {
	var f factPayload
	var date string
	var imported *time.Time
	var category uuid.NullUUID
	if err := rows.Scan(&f.ID, &date, &f.Direction, &f.AmountMinor, &f.Currency, &f.Source.Kind,
		&f.Source.Namespace.System, &f.Source.Namespace.CompanyID, &f.Source.ExternalID, &imported,
		&category, &f.CounterpartyINN, &f.CounterpartyName, &f.Purpose, &f.Version, &f.DataVersion); err != nil {
		return domain.Fact{}, storage.StorageError(err)
	}
	f.Date = ledger.Date(date)
	if imported != nil {
		f.Source.ImportedAt = imported.UTC()
	}
	if category.Valid {
		f.CategoryID = category.UUID
	}
	fact, err := f.restore(scope)
	if err != nil {
		return domain.Fact{}, storage.WithCause(storage.ErrUnavailable, err)
	}
	return fact, nil
}
