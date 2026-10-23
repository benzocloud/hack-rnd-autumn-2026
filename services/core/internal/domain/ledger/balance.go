package ledger

import (
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	"time"
)

type BalanceKind string

const (
	StatementEOD BalanceKind = "statement_eod"
	ManualEOD    BalanceKind = "manual"
)

type BalanceSource struct {
	Kind       BalanceKind
	Namespace  Namespace
	RecordedAt time.Time
}

type BalanceObservation struct {
	scope  Scope
	date   Date
	amount money.Money
	source BalanceSource
}

func NewBalanceObservation(scope Scope, date Date, amount money.Money, source BalanceSource) (BalanceObservation, error) {
	if err := validateBalanceObservation(scope, date, amount, source); err != nil {
		return BalanceObservation{}, err
	}
	source.RecordedAt = source.RecordedAt.UTC()
	return BalanceObservation{scope: scope, date: date, amount: amount, source: source}, nil
}

func validateBalanceObservation(scope Scope, date Date, amount money.Money, source BalanceSource) error {
	for _, err := range []error{scope.Validate(), date.Validate(), amount.Validate()} {
		if err != nil {
			return err
		}
	}
	if (source.Kind != StatementEOD && source.Kind != ManualEOD) || source.RecordedAt.IsZero() {
		return ErrInvalid
	}
	if source.Kind == StatementEOD {
		if err := source.Namespace.Validate(); err != nil {
			return err
		}
	} else if source.Namespace != (Namespace{}) {
		return ErrInvalid
	}
	return nil
}

func (b BalanceObservation) Scope() Scope { return b.scope }

func (b BalanceObservation) Date() Date { return b.date }

func (b BalanceObservation) Amount() money.Money { return b.amount }

func (b BalanceObservation) Source() BalanceSource { return b.source }

// CheckReplay compares observations of the same account/day. Repeated statement
// rows are one EOD observation, never amounts to sum or balancing transactions.
func (b BalanceObservation) CheckReplay(stored BalanceObservation) (bool, error) {
	if b.scope != stored.scope {
		return false, ErrScope
	}
	if b.date != stored.date {
		return false, nil
	}
	if b.amount != stored.amount || b.source.Kind != stored.source.Kind || b.source.Namespace != stored.source.Namespace {
		return false, ErrBalanceConflict
	}
	return true, nil
}
