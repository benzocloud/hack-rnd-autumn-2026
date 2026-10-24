package ledger

// RestoreTransaction retains the stored resource version; it never creates a new fact.
func RestoreTransaction(data TransactionData, version int64) (Transaction, error) {
	if version < 1 {
		return Transaction{}, ErrVersion
	}
	t, err := NewTransaction(data)
	if err != nil {
		return Transaction{}, err
	}
	t.version = version
	return t, nil
}
