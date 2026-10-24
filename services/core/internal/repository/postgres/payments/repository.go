package payments

import "github.com/jmoiron/sqlx"

type repository struct{ tx *sqlx.Tx }

// New borrows tx; its caller owns transaction completion.
func New(tx *sqlx.Tx) *repository { return &repository{tx: tx} }
