package ledger

import "github.com/google/uuid"

type Category struct {
	ID        uuid.UUID
	CompanyID uuid.UUID
}

func (c Category) Validate(companyID uuid.UUID) error {
	if c == (Category{}) {
		return nil // No category is a valid state.
	}
	if c.ID == uuid.Nil || c.CompanyID != companyID {
		return ErrScope
	}
	return nil
}
