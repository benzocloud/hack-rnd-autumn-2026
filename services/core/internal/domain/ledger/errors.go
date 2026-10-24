package ledger

import (
	"errors"
)

var (
	ErrInvalid         = errors.New("invalid ledger value")
	ErrScope           = errors.New("company or account scope mismatch")
	ErrSourceConflict  = errors.New("factual source conflict")
	ErrBinding         = errors.New("source company binding conflict")
	ErrExternalRecord  = errors.New("external record content conflict")
	ErrBalanceConflict = errors.New("balance observation conflict")
	ErrVersion         = errors.New("invalid or exhausted version")
)
