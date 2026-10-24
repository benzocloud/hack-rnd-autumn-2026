package money

import (
	"errors"
)

var (
	ErrCurrency = errors.New("unsupported currency")
	ErrAmount   = errors.New("invalid amount")
	ErrOverflow = errors.New("amount overflow")
)
