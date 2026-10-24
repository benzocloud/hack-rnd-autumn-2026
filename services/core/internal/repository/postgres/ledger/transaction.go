package ledger

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/jmoiron/sqlx"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

// A nil UUID selects only an explicitly scoped external identity, never a global ID.
func LoadTransaction(ctx context.Context, tx *sqlx.Tx, scope domain.Scope, id uuid.UUID, source domain.Source) (domain.Transaction, int64, error) {
	var internalID any
	if id != uuid.Nil {
		internalID = id
	}
	row := tx.QueryRowxContext(ctx, queryLoadTransaction, scope.CompanyID, scope.AccountID, internalID,
		source.Namespace.System, source.Namespace.CompanyID, source.ExternalID)
	return scanTransaction(row, scope)
}

func scanTransaction(row *sqlx.Row, scope domain.Scope) (domain.Transaction, int64, error) {
	d := domain.TransactionData{Scope: scope}
	var date string
	var amount, version, revision int64
	var currency string
	var imported *time.Time
	var category uuid.NullUUID
	err := row.Scan(&d.ID, &date, &d.Direction, &amount, &currency, &d.Source.Kind, &d.Source.Namespace.System,
		&d.Source.Namespace.CompanyID, &d.Source.ExternalID, &imported, &category, &d.CounterpartyINN, &d.CounterpartyName, &d.Purpose, &version, &revision)
	if err != nil {
		return domain.Transaction{}, 0, err
	}
	d.Date = domain.Date(date)
	d.Amount, err = money.New(amount, currency)
	if err != nil {
		return domain.Transaction{}, 0, err
	}
	if imported != nil {
		d.Source.ImportedAt = imported.UTC()
	}
	if category.Valid {
		d.Category = domain.Category{ID: category.UUID, CompanyID: scope.CompanyID}
	}
	record, err := domain.RestoreTransaction(d, version)
	return record, revision, err
}

func InsertTransaction(ctx context.Context, tx *sqlx.Tx, record domain.Transaction, revision int64) error {
	d := record.Data()
	var imported *time.Time
	if !d.Source.ImportedAt.IsZero() {
		at := d.Source.ImportedAt
		imported = &at
	}
	_, err := tx.ExecContext(ctx, queryInsertTransaction, d.Scope.CompanyID, d.Scope.AccountID, d.ID, string(d.Date), d.Direction,
		d.Amount.Minor(), d.Amount.Currency(), d.Source.Kind, d.Source.Namespace.System, d.Source.Namespace.CompanyID,
		d.Source.ExternalID, imported, uuid.NullUUID{UUID: d.Category.ID, Valid: d.Category.ID != uuid.Nil}, d.CounterpartyINN, d.CounterpartyName, d.Purpose, record.Version(), revision)
	return err
}
