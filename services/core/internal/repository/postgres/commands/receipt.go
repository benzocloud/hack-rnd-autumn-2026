package commands

import (
	"bytes"
	"context"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
)

func (r *repository) Reserve(ctx context.Context, scope ledger.Scope, actor, operation, resource, key string, digest []byte) ([]byte, bool, error) {
	tag, err := r.tx.ExecContext(ctx, queryReserve, actor, scope.CompanyID, scope.AccountID, operation, resource, key, digest)
	if err != nil {
		return nil, false, storage.StorageError(err)
	}
	count, err := tag.RowsAffected()
	if err != nil {
		return nil, false, storage.StorageError(err)
	}
	if count == 1 {
		return nil, false, nil
	}
	var storedDigest, result []byte
	err = r.tx.QueryRowxContext(ctx, queryLoadResult, actor, scope.CompanyID, scope.AccountID, operation, resource, key).Scan(&storedDigest, &result)
	if err != nil {
		return nil, false, storage.StorageError(err)
	}
	if !bytes.Equal(storedDigest, digest) {
		return nil, false, storage.ErrIdempotencyConflict
	}
	return result, true, nil
}

func (r *repository) Complete(ctx context.Context, scope ledger.Scope, actor, operation, resource, key string, payload []byte) error {
	tag, err := r.tx.ExecContext(ctx, querySaveResult, actor, scope.CompanyID, scope.AccountID, operation, resource, key, payload)
	if err != nil {
		return storage.StorageError(err)
	}
	count, err := tag.RowsAffected()
	if err != nil {
		return storage.StorageError(err)
	}
	if count != 1 {
		return storage.ErrUnavailable
	}
	return nil
}
