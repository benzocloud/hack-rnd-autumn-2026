package company

import "errors"

var (
	ErrInvalid = errors.New("invalid company")
	ErrReceipt = errors.New("invalid company bootstrap record")
)
