package ledger

import "github.com/google/uuid"

// CategoryDefinition is the company-owned catalogue. Empty Direction permits
// both directions; semantic meaning is distinct from a model's category code.
type CategoryDefinition struct {
	ID            uuid.UUID
	CompanyID     uuid.UUID
	Name          string
	Direction     Direction
	SemanticKind  string
	CatalogueCode string
}

func (c CategoryDefinition) Allows(direction Direction) bool {
	return c.Direction == "" || c.Direction == direction
}

// DefaultCategories creates company-local identities; callers persist them
// once under the account lock. UUIDs never depend on company names or PII.
func DefaultCategories(companyID uuid.UUID) ([]CategoryDefinition, error) {
	definitions := []CategoryDefinition{
		{Name: "Продажи", Direction: Inflow, SemanticKind: "sales_receipt", CatalogueCode: "sales"},
		{Name: "Операционные расходы", Direction: Outflow, SemanticKind: "operating_expense", CatalogueCode: "operating"},
		{Name: "Налоги", Direction: Outflow, SemanticKind: "tax", CatalogueCode: "tax"},
		{Name: "Финансирование", SemanticKind: "financing", CatalogueCode: "financing"},
		{Name: "Переводы между своими счетами", SemanticKind: "own_transfer", CatalogueCode: "own_transfer"},
		{Name: "Прочее", SemanticKind: "other", CatalogueCode: "other"},
	}
	for n := range definitions {
		id, err := uuid.NewV7()
		if err != nil {
			return nil, err
		}
		definitions[n].ID = id
		definitions[n].CompanyID = companyID
	}
	return definitions, nil
}
