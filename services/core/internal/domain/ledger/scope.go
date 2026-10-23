package ledger

type Scope struct {
	CompanyID string
	AccountID string
}

func (s Scope) Validate() error {
	if s.CompanyID == "" || s.AccountID == "" {
		return ErrScope
	}
	return nil
}
