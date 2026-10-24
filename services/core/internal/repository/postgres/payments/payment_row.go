package payments

import (
	"context"
	"database/sql"
	"errors"
	"time"

	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
)

func (r *repository) FindPayment(ctx context.Context, scope ledger.Scope, id uuid.UUID, source ledger.Source) (domain.ExpectedPayment, int64, bool, error) {
	var internalID any
	if id != uuid.Nil {
		internalID = id
	}
	row := r.tx.QueryRowxContext(ctx, queryLoadPayment, scope.CompanyID, scope.AccountID, internalID,
		source.Namespace.System, source.Namespace.CompanyID, source.ExternalID)
	record, revision, err := scanPayment(row, scope)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ExpectedPayment{}, 0, false, nil
	}
	return record, revision, err == nil, storage.StorageError(err)
}

func scanPayment(row *sqlx.Row, scope ledger.Scope) (domain.ExpectedPayment, int64, error) {
	d := domain.PaymentData{Scope: scope}
	var amount, remaining, version, revision int64
	var currency, date string
	var status domain.Status
	var imported *time.Time
	var category uuid.NullUUID
	err := row.Scan(&d.ID, &d.Direction, &amount, &remaining, &currency, &date, &d.Source.Kind,
		&d.Source.Namespace.System, &d.Source.Namespace.CompanyID, &d.Source.ExternalID, &imported,
		&category, &d.CounterpartyName, &d.Description, &status, &version, &d.CreatedAt, &revision)
	if err != nil {
		return domain.ExpectedPayment{}, 0, storage.StorageError(err)
	}
	d.ScheduledDate = ledger.Date(date)
	d.Amount, err = money.New(amount, currency)
	if err != nil {
		return domain.ExpectedPayment{}, 0, storage.StorageError(err)
	}
	left, err := money.New(remaining, currency)
	if err != nil {
		return domain.ExpectedPayment{}, 0, storage.StorageError(err)
	}
	if imported != nil {
		d.Source.ImportedAt = imported.UTC()
	}
	if category.Valid {
		d.Category = ledger.Category{ID: category.UUID, CompanyID: scope.CompanyID}
	}
	record, err := domain.RestoreExpectedPayment(d, left, status, version)
	return record, revision, storage.StorageError(err)
}

func (r *repository) InsertPayment(ctx context.Context, record domain.ExpectedPayment, revision int64) error {
	d := record.Data()
	var imported *time.Time
	if !d.Source.ImportedAt.IsZero() {
		at := d.Source.ImportedAt
		imported = &at
	}
	_, err := r.tx.ExecContext(ctx, queryInsertPayment, d.Scope.CompanyID, d.Scope.AccountID, d.ID, d.Direction,
		d.Amount.Minor(), record.Remaining().Minor(), d.Amount.Currency(), string(d.ScheduledDate), d.Source.Kind,
		d.Source.Namespace.System, d.Source.Namespace.CompanyID, d.Source.ExternalID, imported,
		uuid.NullUUID{UUID: d.Category.ID, Valid: d.Category.ID != uuid.Nil}, d.CounterpartyName, d.Description,
		record.Status(), record.Version(), d.CreatedAt, revision)
	return storage.StorageError(err)
}
