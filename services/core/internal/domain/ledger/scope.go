package ledger

import "github.com/google/uuid"

type Scope struct {
	CompanyID uuid.UUID
	AccountID uuid.UUID
}

func (s Scope) Validate() error {
	if s.CompanyID == uuid.Nil || s.AccountID == uuid.Nil {
		return ErrScope
	}
	return nil
}
