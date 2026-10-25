package ledger

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
)

func (r *repository) ListCategories(ctx context.Context, companyID, after uuid.UUID, limit int) (result []domain.CategoryDefinition, err error) {
	rows, err := r.tx.QueryxContext(ctx, queryListCategories, companyID, nullableID(after), limit)
	if err != nil {
		return nil, storage.StorageError(err)
	}
	defer func() { err = storage.StorageError(errors.Join(err, rows.Close())) }()
	result = make([]domain.CategoryDefinition, 0)
	for rows.Next() {
		var c domain.CategoryDefinition
		var direction string
		c.CompanyID = companyID
		if err := rows.Scan(&c.ID, &c.Name, &direction, &c.SemanticKind, &c.CatalogueCode); err != nil {
			return nil, storage.StorageError(err)
		}
		c.Direction = domain.Direction(direction)
		result = append(result, c)
	}
	return result, storage.StorageError(rows.Err())
}
func (r *repository) Category(ctx context.Context, companyID, id uuid.UUID) (domain.CategoryDefinition, error) {
	c := domain.CategoryDefinition{ID: id, CompanyID: companyID}
	var direction string
	err := r.tx.QueryRowxContext(ctx, queryCategory, companyID, id).Scan(&c.Name, &direction, &c.SemanticKind, &c.CatalogueCode)
	c.Direction = domain.Direction(direction)
	return c, storage.StorageError(err)
}
func nullableID(id uuid.UUID) any {
	if id == uuid.Nil {
		return nil
	}
	return id
}
func nullableDate(date domain.Date) any {
	if date == "" {
		return nil
	}
	return string(date)
}

type transactionRowScanner interface{ Scan(...any) error }

func scanTransactionView(row transactionRowScanner, scope domain.Scope) (domain.StoredTransaction, error) {
	d := domain.TransactionData{Scope: scope}
	var date, currency string
	var amount, version int64
	var imported, created *time.Time
	var category, importID uuid.NullUUID
	err := row.Scan(&d.ID, &date, &d.Direction, &amount, &currency, &d.Source.Kind, &d.Source.Namespace.System, &d.Source.Namespace.CompanyID, &d.Source.ExternalID, &imported, &category, &d.CounterpartyINN, &d.CounterpartyName, &d.Purpose, &version, &created, &importID)
	if err != nil {
		return domain.StoredTransaction{}, storage.StorageError(err)
	}
	d.Date = domain.Date(date)
	d.Amount, err = money.New(amount, currency)
	if err != nil {
		return domain.StoredTransaction{}, storage.StorageError(err)
	}
	if imported != nil {
		d.Source.ImportedAt = imported.UTC()
	}
	if category.Valid {
		d.Category = domain.Category{CompanyID: scope.CompanyID, ID: category.UUID}
	}
	record, err := domain.RestoreTransaction(d, version)
	if err != nil {
		return domain.StoredTransaction{}, storage.StorageError(err)
	}
	return domain.StoredTransaction{Transaction: record, CreatedAt: created, ImportID: importID.UUID}, nil
}
func (r *repository) ListTransactionViews(ctx context.Context, scope domain.Scope, from, through domain.Date, direction domain.Direction, categoryID uuid.UUID, before domain.Date, beforeID uuid.UUID, limit int) (result []domain.StoredTransaction, err error) {
	rows, err := r.tx.QueryxContext(ctx, queryListTransactionViews, scope.CompanyID, scope.AccountID, nullableDate(from), nullableDate(through), string(direction), nullableID(categoryID), nullableDate(before), nullableID(beforeID), limit)
	if err != nil {
		return nil, storage.StorageError(err)
	}
	defer func() { err = storage.StorageError(errors.Join(err, rows.Close())) }()
	result = make([]domain.StoredTransaction, 0)
	for rows.Next() {
		record, err := scanTransactionView(rows, scope)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, storage.StorageError(rows.Err())
}
func (r *repository) ReadTransactionView(ctx context.Context, scope domain.Scope, id uuid.UUID) (domain.StoredTransaction, error) {
	return scanTransactionView(r.tx.QueryRowxContext(ctx, queryReadTransactionView, scope.CompanyID, scope.AccountID, id), scope)
}
func (r *repository) SaveTransactionCategory(ctx context.Context, record domain.Transaction) error {
	d := record.Data()
	result, err := r.tx.ExecContext(ctx, querySaveTransactionCategory, d.Scope.CompanyID, d.Scope.AccountID, d.ID, nullableID(d.Category.ID), record.Version(), record.Version()-1)
	if err != nil {
		return storage.StorageError(err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return storage.StorageError(err)
	}
	if count != 1 {
		return storage.ErrStaleVersion
	}
	return nil
}

// CarryHistoryCoverage preserves exact provenance for a category-only mutation.
// Its caller holds the account lock and changed no factual economics or EOD.
func (r *repository) CarryHistoryCoverage(ctx context.Context, scope domain.Scope, previous, next int64) error {
	if previous < 1 || next != previous+1 {
		return storage.ErrStaleVersion
	}
	_, err := r.tx.ExecContext(ctx, queryCarryHistoryCoverage, scope.CompanyID, scope.AccountID, previous, next)
	return storage.StorageError(err)
}
