package postgres

import (
	"context"

	"github.com/jmoiron/sqlx"
)

const SchemaVersion = 1

func (r *repository) CheckSchema(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	var schema struct {
		Version int64 `db:"version_id"`
		Applied bool  `db:"is_applied"`
	}
	if err := sqlx.GetContext(ctx, r.db, &schema, schemaVersionSQL); err != nil {
		return ErrSchema
	}
	if schema.Version != SchemaVersion || !schema.Applied {
		return ErrSchema
	}
	return nil
}
