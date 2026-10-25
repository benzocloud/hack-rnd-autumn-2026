package payments

import "errors"

var ErrInvalid = errors.New("invalid payments command")

var (
	ErrNotFound            = errors.New("financial resource not found")
	ErrStaleVersion        = errors.New("stale financial data version")
	ErrConflict            = errors.New("financial write conflict")
	ErrIdempotencyConflict = errors.New("idempotency key body conflict")
	ErrUnavailable         = errors.New("financial storage unavailable")
)

var ErrCursor = errors.New("invalid payments cursor")
