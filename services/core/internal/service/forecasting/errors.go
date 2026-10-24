package forecasting

import "errors"

var (
	ErrInvalid        = errors.New("invalid snapshot request")
	ErrNotFound       = errors.New("snapshot financial resource not found")
	ErrMissingBalance = errors.New("closing balance on snapshot cutoff is missing")
	ErrUnavailable    = errors.New("snapshot storage unavailable")
)
