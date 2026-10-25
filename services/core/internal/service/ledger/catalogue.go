package ledger

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

type ListTransactionsCommand struct {
	Scope         domain.Scope
	ActorID       string
	Limit         int
	Cursor        string
	From, Through domain.Date
	Direction     domain.Direction
	CategoryID    uuid.UUID
}
type ListTransactionsResult struct {
	Items  []domain.StoredTransaction
	Cursor string
}
type ListCategoriesCommand struct {
	Scope   domain.Scope
	ActorID string
	Limit   int
	Cursor  string
}
type ListCategoriesResult struct {
	Items  []domain.CategoryDefinition
	Cursor string
}

type catalogStore interface {
	ListCategories(context.Context, uuid.UUID, uuid.UUID, int) ([]domain.CategoryDefinition, error)
	Category(context.Context, uuid.UUID, uuid.UUID) (domain.CategoryDefinition, error)
	ListTransactionViews(context.Context, domain.Scope, domain.Date, domain.Date, domain.Direction, uuid.UUID, domain.Date, uuid.UUID, int) ([]domain.StoredTransaction, error)
	ReadTransactionView(context.Context, domain.Scope, uuid.UUID) (domain.StoredTransaction, error)
	SaveTransactionCategory(context.Context, domain.Transaction) error
	CarryHistoryCoverage(context.Context, domain.Scope, int64, int64) error
}

type catalogCursor struct {
	Scope         domain.Scope
	Actor, Kind   string
	From, Through domain.Date
	Direction     domain.Direction
	CategoryID    uuid.UUID
	Before        domain.Date
	ID            uuid.UUID
}

func catalogPage(raw string, expected catalogCursor) (catalogCursor, error) {
	if raw == "" {
		return expected, nil
	}
	if len(raw) > 1024 {
		return catalogCursor{}, ErrCursor
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return catalogCursor{}, ErrCursor
	}
	var got catalogCursor
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&got) != nil || decoder.Decode(new(any)) != io.EOF || got.ID == uuid.Nil {
		return catalogCursor{}, ErrCursor
	}
	compare := got
	compare.Before = ""
	compare.ID = uuid.Nil
	if compare != expected || got.Kind == "transactions" && got.Before.Validate() != nil {
		return catalogCursor{}, ErrCursor
	}
	return got, nil
}
func catalogNext(c catalogCursor) (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", ErrUnavailable
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func validCatalogPage(scope domain.Scope, actor string, limit int) error {
	if scope.Validate() != nil || strings.TrimSpace(actor) == "" || limit < 1 || limit > 100 {
		return ErrInvalid
	}
	return nil
}

func (s *service) ListTransactions(ctx context.Context, cmd ListTransactionsCommand) (ListTransactionsResult, error) {
	if cmd.Limit == 0 {
		cmd.Limit = 50
	}
	if err := validateListTransactions(cmd); err != nil {
		return ListTransactionsResult{}, err
	}
	if s.withinTx == nil {
		return ListTransactionsResult{}, ErrUnavailable
	}
	c, err := catalogPage(cmd.Cursor, catalogCursor{Scope: cmd.Scope, Actor: cmd.ActorID, Kind: "transactions", From: cmd.From, Through: cmd.Through, Direction: cmd.Direction, CategoryID: cmd.CategoryID})
	if err != nil {
		return ListTransactionsResult{}, err
	}
	var result ListTransactionsResult
	err = s.withinTx(ctx, func(ctx context.Context, st store, _ commandStore) error {
		catalog, ok := st.(catalogStore)
		if !ok {
			return ErrUnavailable
		}
		if _, err := st.LoadAccount(ctx, cmd.Scope); err != nil {
			return err
		}
		items, err := catalog.ListTransactionViews(ctx, cmd.Scope, cmd.From, cmd.Through, cmd.Direction, cmd.CategoryID, c.Before, c.ID, cmd.Limit+1)
		if err != nil {
			return err
		}
		result.Items = items
		if len(items) > cmd.Limit {
			last := items[cmd.Limit-1].Transaction.Data()
			result.Items = items[:cmd.Limit]
			c.Before = last.Date
			c.ID = last.ID
			result.Cursor, err = catalogNext(c)
		}
		return err
	})
	return result, err
}
func validateListTransactions(cmd ListTransactionsCommand) error {
	if err := validCatalogPage(cmd.Scope, cmd.ActorID, cmd.Limit); err != nil {
		return err
	}
	if cmd.From != "" && cmd.From.Validate() != nil || cmd.Through != "" && cmd.Through.Validate() != nil || cmd.From != "" && cmd.Through != "" && cmd.From > cmd.Through || cmd.Direction != "" && cmd.Direction.Validate() != nil {
		return ErrInvalid
	}
	return nil
}

func (s *service) ListCategories(ctx context.Context, cmd ListCategoriesCommand) (ListCategoriesResult, error) {
	if cmd.Limit == 0 {
		cmd.Limit = 50
	}
	if err := validCatalogPage(cmd.Scope, cmd.ActorID, cmd.Limit); err != nil {
		return ListCategoriesResult{}, err
	}
	if s.withinTx == nil {
		return ListCategoriesResult{}, ErrUnavailable
	}
	c, err := catalogPage(cmd.Cursor, catalogCursor{Scope: cmd.Scope, Actor: cmd.ActorID, Kind: "categories"})
	if err != nil {
		return ListCategoriesResult{}, err
	}
	var result ListCategoriesResult
	err = s.withinTx(ctx, func(ctx context.Context, st store, _ commandStore) error {
		catalog, ok := st.(catalogStore)
		if !ok {
			return ErrUnavailable
		}
		if _, err := st.LoadAccount(ctx, cmd.Scope); err != nil {
			return err
		}
		items, err := catalog.ListCategories(ctx, cmd.Scope.CompanyID, c.ID, cmd.Limit+1)
		if err != nil {
			return err
		}
		result.Items = items
		if len(items) > cmd.Limit {
			result.Items = items[:cmd.Limit]
			c.ID = items[cmd.Limit-1].ID
			result.Cursor, err = catalogNext(c)
		}
		return err
	})
	return result, err
}
