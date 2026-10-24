package ledger

import (
	"context"
	"database/sql"
	"maps"
	"slices"

	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"

	"github.com/jmoiron/sqlx"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

func LoadAccount(ctx context.Context, tx *sqlx.Tx, scope domain.Scope) (*domain.BankAccount, error) {
	state := domain.AccountState{Scope: scope, Bindings: make(map[string]string)}
	err := tx.QueryRowxContext(ctx, queryLoadAccount, scope.CompanyID, scope.AccountID).Scan(
		&state.DataVersion, &state.Factual, &state.Namespace.System, &state.Namespace.CompanyID)
	if err != nil {
		return nil, storage.StorageError(err)
	}
	var bindings []sourceBinding
	if err := tx.SelectContext(ctx, &bindings, queryLoadBindings, scope.CompanyID, scope.AccountID); err != nil {
		return nil, storage.StorageError(err)
	}
	for _, binding := range bindings {
		state.Bindings[binding.System] = binding.CompanyID
	}
	return domain.RestoreBankAccount(state)
}

func SaveAccount(ctx context.Context, tx *sqlx.Tx, account *domain.BankAccount) error {
	state := account.State()
	for _, system := range slices.Sorted(maps.Keys(state.Bindings)) {
		tag, err := tx.ExecContext(ctx, querySaveBinding, state.Scope.CompanyID, state.Scope.AccountID, system, state.Bindings[system])
		if err != nil {
			return storage.StorageError(err)
		}
		affected, err := tag.RowsAffected()
		if err != nil {
			return storage.StorageError(err)
		}
		if affected != 1 {
			return domain.ErrBinding
		}
	}
	result, err := tx.ExecContext(ctx, querySaveAccount, state.Scope.CompanyID, state.Scope.AccountID, state.DataVersion,
		state.Factual, state.Namespace.System, state.Namespace.CompanyID)
	if err != nil {
		return storage.StorageError(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return storage.StorageError(err)
	}
	if affected != 1 {
		return storage.StorageError(sql.ErrNoRows)
	}
	return nil
}

func (r *repository) LoadAccount(ctx context.Context, scope domain.Scope) (*domain.BankAccount, error) {
	return LoadAccount(ctx, r.tx, scope)
}

func (r *repository) SaveAccount(ctx context.Context, account *domain.BankAccount) error {
	return SaveAccount(ctx, r.tx, account)
}
