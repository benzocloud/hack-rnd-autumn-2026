package payments

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"

	forecast "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
)

func scanGroup(row interface{ Scan(...any) error }, scope ledger.Scope) (forecast.ReconciliationGroup, error) {
	var g forecast.ReconciliationGroup
	g.Scope = scope
	var cutoff, start, end string
	var refs []byte
	err := row.Scan(&g.ID, &g.Version, &g.Status, &cutoff, &start, &end, &g.ConfirmedBy, &g.ConfirmedAt, &g.Additional, &refs)
	if err != nil {
		return g, storage.StorageError(err)
	}
	g.Cutoff, g.HorizonStart, g.HorizonEnd = ledger.Date(cutoff), ledger.Date(start), ledger.Date(end)
	if json.Unmarshal(refs, &g.Operations) != nil || g.Validate() != nil {
		return forecast.ReconciliationGroup{}, storage.ErrUnavailable
	}
	return g, nil
}
func (r *repository) FindReconciliationGroup(ctx context.Context, scope ledger.Scope, id uuid.UUID) (forecast.ReconciliationGroup, error) {
	return scanGroup(r.tx.QueryRowxContext(ctx, queryFindReconciliationGroup, scope.CompanyID, scope.AccountID, id), scope)
}
func (r *repository) ListReconciliationGroups(ctx context.Context, scope ledger.Scope, includeRevoked bool, after uuid.UUID, limit int) (result []forecast.ReconciliationGroup, err error) {
	rows, err := r.tx.QueryxContext(ctx, queryListReconciliationGroups, scope.CompanyID, scope.AccountID, includeRevoked, after, limit)
	if err != nil {
		return nil, storage.StorageError(err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			result = nil
			err = errors.Join(err, storage.StorageError(closeErr))
		}
	}()
	result = make([]forecast.ReconciliationGroup, 0)
	for rows.Next() {
		g, err := scanGroup(rows, scope)
		if err != nil {
			return nil, err
		}
		result = append(result, g)
	}
	return result, storage.StorageError(rows.Err())
}
func (r *repository) InsertReconciliationGroup(ctx context.Context, g forecast.ReconciliationGroup) error {
	refs := g.Operations
	if refs == nil {
		refs = []forecast.OperationReference{}
	}
	payload, err := json.Marshal(refs)
	if err != nil {
		return storage.WithCause(storage.ErrUnavailable, err)
	}
	_, err = r.tx.ExecContext(ctx, queryInsertReconciliationGroup, g.Scope.CompanyID, g.Scope.AccountID, g.ID, g.Version, g.Status, string(g.Cutoff), string(g.HorizonStart), string(g.HorizonEnd), g.ConfirmedBy, g.ConfirmedAt, g.Additional, payload)
	return storage.StorageError(err)
}
func (r *repository) SaveReconciliationGroup(ctx context.Context, g forecast.ReconciliationGroup, expected int64) error {
	tag, err := r.tx.ExecContext(ctx, querySaveReconciliationGroup, g.Scope.CompanyID, g.Scope.AccountID, g.ID, g.Version, g.Status, expected)
	if err != nil {
		return storage.StorageError(err)
	}
	n, err := tag.RowsAffected()
	if err != nil {
		return storage.StorageError(err)
	}
	if n != 1 {
		return storage.ErrStaleVersion
	}
	return nil
}
func (r *repository) ValidatePaymentGroup(ctx context.Context, scope ledger.Scope, id uuid.UUID) error {
	if id == uuid.Nil {
		return nil
	}
	g, err := r.FindReconciliationGroup(ctx, scope, id)
	if err != nil {
		return err
	}
	if g.Status != "active" {
		return storage.ErrConflict
	}
	return nil
}
