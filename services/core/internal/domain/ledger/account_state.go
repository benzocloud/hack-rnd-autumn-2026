package ledger

import "maps"

type AccountState struct {
	Scope       Scope
	DataVersion int64
	Factual     SourceKind
	Namespace   Namespace
	Bindings    map[string]string
}

// RestoreBankAccount validates persisted state without choosing new versions or authority.
func RestoreBankAccount(state AccountState) (*BankAccount, error) {
	if err := validateAccountState(state); err != nil {
		return nil, err
	}
	bindings := maps.Clone(state.Bindings)
	if bindings == nil {
		bindings = make(map[string]string)
	}
	return &BankAccount{scope: state.Scope, dataVersion: state.DataVersion, factual: state.Factual,
		namespace: state.Namespace, bindings: bindings}, nil
}

func validateAccountState(state AccountState) error {
	if err := state.Scope.Validate(); err != nil {
		return err
	}
	if state.DataVersion < 1 {
		return ErrVersion
	}
	for system, company := range state.Bindings {
		if err := (Namespace{System: system, CompanyID: company}).Validate(); err != nil {
			return err
		}
	}
	switch state.Factual {
	case "", Manual:
		if state.Namespace != (Namespace{}) {
			return ErrInvalid
		}
	case Imported:
		if err := state.Namespace.Validate(); err != nil {
			return err
		}
		if state.Bindings[state.Namespace.System] != state.Namespace.CompanyID {
			return ErrBinding
		}
	default:
		return ErrInvalid
	}
	return nil
}

func (a *BankAccount) State() AccountState {
	return AccountState{Scope: a.scope, DataVersion: a.dataVersion, Factual: a.factual,
		Namespace: a.namespace, Bindings: maps.Clone(a.bindings)}
}
