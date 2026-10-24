package forecastjobs

import "github.com/jmoiron/sqlx"

type repository struct{ tx *sqlx.Tx }

// New borrows one app-owned transaction; callers own commit/rollback and deadline.
func New(tx *sqlx.Tx) *repository { return &repository{tx: tx} }
