package company

import (
	"context"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/company"
	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
)

// The unique INSERT serializes retries for the same command. READ COMMITTED's
// next statement observes the winning committed receipt after a conflict wait.
func (r *repository) ReserveBootstrap(ctx context.Context, record domain.BootstrapRecord) (domain.BootstrapRecord, bool, error) {
	if record.Validate() != nil {
		return domain.BootstrapRecord{}, false, storage.ErrUnavailable
	}
	result, err := r.tx.ExecContext(ctx, reserveBootstrapSQL, record.CommandID, record.CompanyID, record.AccountID, record.RequestSHA256, record.Spec.Name, record.Spec.INN, record.Spec.Currency, record.Spec.Timezone, record.CommittedAt)
	if err != nil {
		return domain.BootstrapRecord{}, false, storage.StorageError(err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return domain.BootstrapRecord{}, false, storage.StorageError(err)
	}
	if count == 1 {
		return record, true, nil
	}
	if count != 0 {
		return domain.BootstrapRecord{}, false, storage.ErrUnavailable
	}
	stored, err := r.ReadBootstrap(ctx, record.CommandID)
	return stored, false, err
}
func (r *repository) ReadBootstrap(ctx context.Context, id uuid.UUID) (domain.BootstrapRecord, error) {
	var row bootstrapRow
	if err := r.tx.GetContext(ctx, &row, readBootstrapSQL, id); err != nil {
		return domain.BootstrapRecord{}, storage.StorageError(err)
	}
	return row.record()
}
