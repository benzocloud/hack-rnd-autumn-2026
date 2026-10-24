package ledger

import (
	"math"
)

type BankAccount struct {
	scope       Scope
	dataVersion int64
	factual     SourceKind
	namespace   Namespace
	bindings    map[string]string
}

func NewBankAccount(scope Scope) (*BankAccount, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	return &BankAccount{scope: scope, dataVersion: 1, bindings: make(map[string]string)}, nil
}

func (a *BankAccount) Scope() Scope { return a.scope }

func (a *BankAccount) DataVersion() int64 { return a.dataVersion }

// BindFacts is called only for an accepted factual write. Preview, connection,
// manual EOD and expected payments must not call it. Persistence supplies atomicity.
func (a *BankAccount) BindFacts(source Source) error {
	if err := a.scope.Validate(); err != nil {
		return err
	}
	if err := source.Validate(); err != nil {
		return err
	}
	return a.bindFacts(source.Kind, source.Namespace)
}

// BindStatementFacts chooses the factual namespace for an accepted external EOD.
// A statement has no transaction external ID; do not fabricate one for binding.
func (a *BankAccount) BindStatementFacts(namespace Namespace) error {
	if err := a.scope.Validate(); err != nil {
		return err
	}
	if err := namespace.Validate(); err != nil {
		return err
	}
	return a.bindFacts(Imported, namespace)
}

func (a *BankAccount) bindFacts(kind SourceKind, namespace Namespace) error {
	if a.factual != "" && (a.factual != kind || a.namespace != namespace) {
		return ErrSourceConflict
	}
	if kind == Imported {
		if err := a.BindSupplemental(namespace); err != nil {
			return err
		}
	}
	a.factual, a.namespace = kind, namespace
	return nil
}

// BindSupplemental also guards planned-payment imports without choosing factual authority.
func (a *BankAccount) BindSupplemental(namespace Namespace) error {
	if err := a.scope.Validate(); err != nil {
		return err
	}
	if err := namespace.Validate(); err != nil {
		return err
	}
	if company, ok := a.bindings[namespace.System]; ok && company != namespace.CompanyID {
		return ErrBinding
	}
	if a.bindings == nil {
		a.bindings = make(map[string]string)
	}
	a.bindings[namespace.System] = namespace.CompanyID
	return nil
}

// AdvanceDataVersion is separate from binding and resource versions. The
// write transaction calls it once after a changed input, never on read or replay.
func (a *BankAccount) AdvanceDataVersion() error {
	if a.dataVersion < 1 || a.dataVersion == math.MaxInt64 {
		return ErrVersion
	}
	a.dataVersion++
	return nil
}
