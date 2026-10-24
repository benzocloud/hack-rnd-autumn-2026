package ledger

import (
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

type TransactionData struct {
	ID               uuid.UUID
	Scope            Scope
	Date             Date
	Direction        Direction
	Amount           money.Money
	Source           Source
	Category         Category
	CounterpartyINN  string
	CounterpartyName string
	Purpose          string
}

type Transaction struct {
	data    TransactionData
	version int64
}

func NewTransaction(data TransactionData) (Transaction, error) {
	if err := validateTransaction(data); err != nil {
		return Transaction{}, err
	}
	data.Source.ImportedAt = data.Source.ImportedAt.UTC()
	return Transaction{data: data, version: 1}, nil
}

func validateTransaction(data TransactionData) error {
	if data.ID == uuid.Nil {
		return ErrInvalid
	}
	for _, err := range []error{data.Scope.Validate(), data.Date.Validate(), data.Direction.Validate(),
		data.Amount.Positive(), data.Source.Validate(), data.Category.Validate(data.Scope.CompanyID)} {
		if err != nil {
			return err
		}
	}
	return nil
}

func (t Transaction) Data() TransactionData { return t.data }

func (t Transaction) Version() int64 { return t.version }

func (t Transaction) Categorize(category Category) (Transaction, error) {
	if err := t.data.Scope.Validate(); err != nil {
		return Transaction{}, err
	}
	if err := category.Validate(t.data.Scope.CompanyID); err != nil {
		return Transaction{}, err
	}
	if t.data.Category == category {
		return t, nil
	}
	if t.version < 1 || t.version == math.MaxInt64 {
		return Transaction{}, ErrVersion
	}
	t.data.Category = category
	t.version++
	return t, nil
}

// CheckReplay compares a proposed import with a stored record of the same external
// identity. Internal IDs and time of reimport do not change the financial fact.
// Uniqueness and the stored-record lookup belong to the later repository stage.
func (t Transaction) CheckReplay(stored Transaction) (bool, error) {
	if t.data.Scope != stored.data.Scope {
		return false, ErrScope
	}
	if t.data.Source.Kind != Imported || stored.data.Source.Kind != Imported ||
		t.data.Source.Namespace != stored.data.Source.Namespace ||
		t.data.Source.ExternalID != stored.data.Source.ExternalID {
		return false, nil
	}
	left, right := t.data, stored.data
	left.ID, right.ID = uuid.Nil, uuid.Nil
	left.Source.ImportedAt, right.Source.ImportedAt = time.Time{}, time.Time{}
	// Classification is editable independently of imported financial economics.
	// Replay returns the stored record and must not overwrite its classification.
	left.Category, right.Category = Category{}, Category{}
	if left != right {
		return false, ErrExternalRecord
	}
	return true, nil
}
