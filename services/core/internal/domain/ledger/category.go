package ledger

type Category struct {
	ID        string
	CompanyID string
}

func (c Category) Validate(companyID string) error {
	if c == (Category{}) {
		return nil // No category is a valid state.
	}
	if c.ID == "" || c.CompanyID != companyID {
		return ErrScope
	}
	return nil
}
