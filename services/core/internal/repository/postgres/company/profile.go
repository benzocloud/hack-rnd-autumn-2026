package company

import (
	"context"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/company"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
)

// Each method persists one model; the service coordinates bootstrap atomically.
func (r *repository) InsertCompany(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return storage.ErrUnavailable
	}
	_, err := r.tx.ExecContext(ctx, insertCompanySQL, id)
	return storage.StorageError(err)
}
func (r *repository) InsertAccount(ctx context.Context, account *ledger.BankAccount) error {
	if account == nil || account.Scope().Validate() != nil {
		return storage.ErrUnavailable
	}
	state := account.State()
	if state.DataVersion != 1 || state.Factual != "" || state.Namespace != (ledger.Namespace{}) || len(state.Bindings) != 0 {
		return storage.ErrUnavailable
	}
	_, err := r.tx.ExecContext(ctx, insertAccountSQL, state.Scope.AccountID, state.Scope.CompanyID, money.RUB, state.DataVersion)
	return storage.StorageError(err)
}
func (r *repository) InsertProfile(ctx context.Context, profile domain.Profile) error {
	if profile.Validate() != nil {
		return storage.ErrUnavailable
	}
	_, err := r.tx.ExecContext(ctx, insertProfileSQL, profile.ID, profile.AccountID, profile.Spec.Name, profile.Spec.INN, profile.Spec.Currency, profile.Spec.Timezone, profile.Version, profile.CreatedAt)
	return storage.StorageError(err)
}
func (r *repository) ReadProfile(ctx context.Context, id uuid.UUID) (domain.Profile, error) {
	var row profileRow
	if err := r.tx.GetContext(ctx, &row, readProfileSQL, id); err != nil {
		return domain.Profile{}, storage.StorageError(err)
	}
	return row.profile()
}

func (r *repository) UpdateProfile(ctx context.Context, profile domain.Profile, version int64) error {
	if profile.Validate() != nil || version < 1 || profile.Version-1 != version {
		return storage.ErrUnavailable
	}
	result, err := r.tx.ExecContext(ctx, updateProfileSQL, profile.ID, profile.Spec.Name, profile.Spec.INN, profile.Version, version)
	if err != nil {
		return storage.StorageError(err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return storage.StorageError(err)
	}
	if count != 1 {
		return storage.ErrConflict
	}
	return nil
}
