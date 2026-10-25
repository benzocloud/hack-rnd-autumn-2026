package scenarios

import (
	"context"
	_ "embed"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
)

//go:embed sql/insert.sql
var insertSQL string

//go:embed sql/read.sql
var readSQL string

//go:embed sql/list.sql
var listSQL string

type repository struct{ tx *sqlx.Tx }

func New(tx *sqlx.Tx) *repository { return &repository{tx: tx} }
func (r *repository) Insert(ctx context.Context, s domain.Scenario) error {
	_, err := r.tx.ExecContext(ctx, insertSQL, s.Scope.CompanyID, s.Scope.AccountID, s.ID, s.ForecastID, s.BaseForecastID, s.Name, s.CreatedAt, string(s.Payload))
	return storage.StorageError(err)
}

type scanner interface{ Scan(...any) error }

func scan(row scanner, scope ledger.Scope) (domain.Scenario, error) {
	s := domain.Scenario{Scope: scope}
	err := row.Scan(&s.ID, &s.ForecastID, &s.BaseForecastID, &s.Name, &s.CreatedAt, &s.Payload)
	return s, storage.StorageError(err)
}
func (r *repository) Read(ctx context.Context, scope ledger.Scope, id uuid.UUID, forecast bool) (domain.Scenario, error) {
	return scan(r.tx.QueryRowxContext(ctx, readSQL, scope.CompanyID, scope.AccountID, id, forecast), scope)
}
func (r *repository) List(ctx context.Context, scope ledger.Scope, base, before uuid.UUID, limit int) (items []domain.Scenario, err error) {
	rows, err := r.tx.QueryxContext(ctx, listSQL, scope.CompanyID, scope.AccountID, base, before, limit)
	if err != nil {
		return nil, storage.StorageError(err)
	}
	defer func() {
		closeErr := rows.Close()
		if err == nil {
			err = storage.StorageError(closeErr)
		}
	}()
	items = []domain.Scenario{}
	for rows.Next() {
		s, e := scan(rows, scope)
		if e != nil {
			return nil, e
		}
		items = append(items, s)
	}
	return items, storage.StorageError(rows.Err())
}
