package ledger

type sourceBinding struct {
	System    string `db:"source_system"`
	CompanyID string `db:"source_company_id"`
}
