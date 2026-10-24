package postgres

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/postgres/record"
)

const SchemaVersion = 1

type schemaRepository struct {
	db           sqlx.ExtContext
	queryTimeout time.Duration
}

// NewSchemaRepository borrows a ready DB or transaction; its caller owns its lifetime.
func NewSchemaRepository(db sqlx.ExtContext, queryTimeout time.Duration) *schemaRepository {
	return &schemaRepository{db: db, queryTimeout: queryTimeout}
}

func (r *schemaRepository) CheckSchema(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	var schema record.SchemaVersion
	if err := sqlx.GetContext(ctx, r.db, &schema, schemaVersionSQL); err != nil {
		return ErrSchema
	}
	if schema.Version != SchemaVersion || !schema.Applied {
		return ErrSchema
	}
	return nil
}
