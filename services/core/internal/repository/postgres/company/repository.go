package company

import "github.com/jmoiron/sqlx"

type repository struct{ tx *sqlx.Tx }

// New borrows an app-owned transaction; it never opens a pool or commits.
func New(tx *sqlx.Tx) *repository { return &repository{tx: tx} }
