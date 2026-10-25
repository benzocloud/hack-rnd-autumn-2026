package ledger

import (
	"context"
	"strings"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

type UpdateTransactionCategoryCommand struct {
	Scope           domain.Scope
	ActorID         string
	TransactionID   uuid.UUID
	ExpectedVersion int64
	// Nil explicitly clears; omission is rejected by the transport union decoder.
	CategoryID uuid.UUID
}
type UpdateTransactionCategoryResult struct {
	Transaction domain.StoredTransaction
	DataVersion int64
}

func (s *service) UpdateTransactionCategory(ctx context.Context, cmd UpdateTransactionCategoryCommand) (UpdateTransactionCategoryResult, error) {
	if err := validateUpdateTransactionCategory(cmd); err != nil {
		return UpdateTransactionCategoryResult{}, err
	}
	if s.withinTx == nil {
		return UpdateTransactionCategoryResult{}, ErrUnavailable
	}
	var result UpdateTransactionCategoryResult
	err := s.withinTx(ctx, func(ctx context.Context, st store, _ commandStore) error {
		catalog, ok := st.(catalogStore)
		if !ok {
			return ErrUnavailable
		}
		account, err := st.LoadAccount(ctx, cmd.Scope)
		if err != nil {
			return err
		}
		stored, err := catalog.ReadTransactionView(ctx, cmd.Scope, cmd.TransactionID)
		if err != nil {
			return err
		}
		if stored.Transaction.Version() != cmd.ExpectedVersion {
			return ErrStaleVersion
		}
		category := domain.Category{}
		if cmd.CategoryID != uuid.Nil {
			definition, err := catalog.Category(ctx, cmd.Scope.CompanyID, cmd.CategoryID)
			if err != nil {
				return err
			}
			if !definition.Allows(stored.Transaction.Data().Direction) {
				return ErrInvalid
			}
			category = domain.Category{ID: cmd.CategoryID, CompanyID: cmd.Scope.CompanyID}
		}
		updated, err := stored.Transaction.Categorize(category)
		if err != nil {
			return err
		}
		if updated.Version() == stored.Transaction.Version() {
			result = UpdateTransactionCategoryResult{Transaction: stored, DataVersion: account.DataVersion()}
			return nil
		}
		previousRevision := account.DataVersion()
		if err := account.AdvanceDataVersion(); err != nil {
			return err
		}
		if err := catalog.SaveTransactionCategory(ctx, updated); err != nil {
			return err
		}
		if err := st.SaveAccount(ctx, account); err != nil {
			return err
		}
		if err := catalog.CarryHistoryCoverage(ctx, cmd.Scope, previousRevision, account.DataVersion()); err != nil {
			return err
		}
		stored.Transaction = updated
		result = UpdateTransactionCategoryResult{Transaction: stored, DataVersion: account.DataVersion()}
		return nil
	})
	if err != nil {
		return UpdateTransactionCategoryResult{}, err
	}
	return result, nil
}

func validateUpdateTransactionCategory(cmd UpdateTransactionCategoryCommand) error {
	if cmd.Scope.Validate() != nil || strings.TrimSpace(cmd.ActorID) == "" || cmd.TransactionID == uuid.Nil || cmd.ExpectedVersion < 1 {
		return ErrInvalid
	}
	return nil
}
