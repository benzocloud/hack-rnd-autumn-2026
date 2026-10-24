package payments

import (
	"errors"
)

var (
	ErrInvalid    = errors.New("invalid expected payment")
	ErrState      = errors.New("payment state does not allow this operation")
	ErrAllocation = errors.New("allocation exceeds available amount")
	ErrDirection  = errors.New("payment direction mismatch")
)
