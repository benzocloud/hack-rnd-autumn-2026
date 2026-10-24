package postgres

import (
	"time"

	"github.com/jmoiron/sqlx"
)

type repository struct {
	db           sqlx.ExtContext
	queryTimeout time.Duration
}

// NewRepository borrows a ready DB or transaction; its caller owns its lifetime.
func NewRepository(db sqlx.ExtContext, queryTimeout time.Duration) *repository {
	return &repository{db: db, queryTimeout: queryTimeout}
}
