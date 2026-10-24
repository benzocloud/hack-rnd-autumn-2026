package forecasting

import "github.com/jmoiron/sqlx"

type repository struct {
	tx *sqlx.Tx
}

// New borrows an app-owned transaction, including its isolation and deadline.
func New(tx *sqlx.Tx) *repository { return &repository{tx: tx} }
