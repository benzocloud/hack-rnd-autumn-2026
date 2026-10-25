package ledger

import "errors"

var ErrSuspectedDuplicate = errors.New("manual transaction matches an imported fact")

var ErrInvalid = errors.New("invalid ledger command")

var (
	ErrNotFound            = errors.New("financial resource not found")
	ErrStaleVersion        = errors.New("stale financial data version")
	ErrConflict            = errors.New("financial write conflict")
	ErrIdempotencyConflict = errors.New("idempotency key body conflict")
	ErrUnavailable         = errors.New("financial storage unavailable")
	ErrCursor              = errors.New("invalid ledger cursor")
)
