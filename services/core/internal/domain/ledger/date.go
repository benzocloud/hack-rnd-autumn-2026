package ledger

import (
	"time"
)

// Date is a business calendar date, not an instant or an import timestamp.
type Date string

func (d Date) Validate() error {
	if len(d) != len(time.DateOnly) {
		return ErrInvalid
	}
	if _, err := time.Parse(time.DateOnly, string(d)); err != nil {
		return ErrInvalid
	}
	return nil
}
