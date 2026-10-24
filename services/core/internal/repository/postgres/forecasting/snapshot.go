package forecasting

import (
	"context"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
)

func (r *repository) InsertSnapshot(ctx context.Context, snapshot domain.FinancialSnapshot) error {
	d := snapshot.Data()
	if _, err := domain.NewFinancialSnapshot(d); err != nil {
		return err
	}
	payload, err := encodeSnapshot(snapshot)
	if err != nil {
		return storage.WithCause(storage.ErrUnavailable, err)
	}
	_, err = r.tx.ExecContext(ctx, queryInsertSnapshot, d.Scope.CompanyID, d.Scope.AccountID,
		d.ID, d.DataVersion, string(d.Cutoff), d.CreatedAt, payload)
	return storage.StorageError(err)
}

func (r *repository) ReadSnapshot(ctx context.Context, scope ledger.Scope, id uuid.UUID) (domain.FinancialSnapshot, error) {
	d := domain.SnapshotData{ID: id, Scope: scope}
	var cutoff string
	var version int
	var payload []byte
	err := r.tx.QueryRowxContext(ctx, queryReadSnapshot, scope.CompanyID, scope.AccountID, id).
		Scan(&d.DataVersion, &cutoff, &d.CreatedAt, &version, &payload)
	if err != nil {
		return domain.FinancialSnapshot{}, storage.StorageError(err)
	}
	d.Cutoff = ledger.Date(cutoff)
	snapshot, err := decodeSnapshot(d, version, payload)
	if err != nil {
		return domain.FinancialSnapshot{}, storage.WithCause(storage.ErrUnavailable, err)
	}
	return snapshot, nil
}
