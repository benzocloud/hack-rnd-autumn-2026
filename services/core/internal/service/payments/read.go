package payments

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
)

type ListPaymentsCommand struct {
	Scope         ledger.Scope
	ActorID       string
	Limit         int
	Cursor        string
	From, Through ledger.Date
	Status        domain.Status
}
type ListPaymentsResult struct {
	Items  []domain.ExpectedPayment
	Cursor string
}
type ListMatchesCommand struct {
	Scope     ledger.Scope
	PaymentID uuid.UUID
	ActorID   string
	Limit     int
	Cursor    string
}
type ListMatchesResult struct {
	Items  []domain.PaymentMatch
	Cursor string
}
type readStore interface {
	ListPayments(context.Context, ledger.Scope, ledger.Date, ledger.Date, domain.Status, ledger.Date, uuid.UUID, int) ([]domain.ExpectedPayment, error)
	ListMatches(context.Context, ledger.Scope, uuid.UUID, time.Time, uuid.UUID, int) ([]domain.PaymentMatch, error)
}
type pageCursor struct {
	Scope         ledger.Scope
	Actor, Kind   string
	From, Through ledger.Date
	Status        domain.Status
	PaymentID     uuid.UUID
	Date          ledger.Date
	At            time.Time
	ID            uuid.UUID
}

func decodePage(raw string, expected pageCursor) (pageCursor, error) {
	if raw == "" {
		return expected, nil
	}
	if len(raw) > 1024 {
		return pageCursor{}, ErrCursor
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return pageCursor{}, ErrCursor
	}
	var got pageCursor
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&got) != nil || decoder.Decode(new(any)) != io.EOF || got.ID == uuid.Nil {
		return pageCursor{}, ErrCursor
	}
	compare := got
	compare.ID = uuid.Nil
	compare.Date = ""
	compare.At = time.Time{}
	if compare != expected || got.Kind == "payments" && got.Date.Validate() != nil || got.Kind == "matches" && got.At.IsZero() {
		return pageCursor{}, ErrCursor
	}
	return got, nil
}
func encodePage(p pageCursor) (string, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return "", ErrUnavailable
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func (s *service) ReadPayment(ctx context.Context, scope ledger.Scope, id uuid.UUID) (domain.ExpectedPayment, error) {
	if scope.Validate() != nil || id == uuid.Nil {
		return domain.ExpectedPayment{}, ErrInvalid
	}
	var result domain.ExpectedPayment
	if s.withinTx == nil {
		return result, ErrUnavailable
	}
	err := s.withinTx(ctx, func(ctx context.Context, st store, _ commandStore) error {
		if _, err := st.LoadAccount(ctx, scope); err != nil {
			return err
		}
		p, _, found, err := st.FindPayment(ctx, scope, id, ledger.Source{})
		if err != nil {
			return err
		}
		if !found {
			return ErrNotFound
		}
		result = p
		return nil
	})
	if err != nil {
		return domain.ExpectedPayment{}, err
	}
	return result, nil
}
func (s *service) ListPayments(ctx context.Context, cmd ListPaymentsCommand) (ListPaymentsResult, error) {
	if cmd.Limit == 0 {
		cmd.Limit = 50
	}
	if cmd.Scope.Validate() != nil || strings.TrimSpace(cmd.ActorID) == "" || cmd.Limit < 1 || cmd.Limit > 100 || cmd.From != "" && cmd.From.Validate() != nil || cmd.Through != "" && cmd.Through.Validate() != nil || cmd.From != "" && cmd.Through != "" && cmd.From > cmd.Through {
		return ListPaymentsResult{}, ErrInvalid
	}
	switch cmd.Status {
	case "", domain.Planned, domain.PartiallyMatched, domain.Matched, domain.Cancelled:
	default:
		return ListPaymentsResult{}, ErrInvalid
	}
	c, err := decodePage(cmd.Cursor, pageCursor{Scope: cmd.Scope, Actor: cmd.ActorID, Kind: "payments", From: cmd.From, Through: cmd.Through, Status: cmd.Status})
	if err != nil {
		return ListPaymentsResult{}, err
	}
	var result ListPaymentsResult
	if s.withinTx == nil {
		return result, ErrUnavailable
	}
	err = s.withinTx(ctx, func(ctx context.Context, st store, _ commandStore) error {
		read, ok := st.(readStore)
		if !ok {
			return ErrUnavailable
		}
		if _, err := st.LoadAccount(ctx, cmd.Scope); err != nil {
			return err
		}
		items, err := read.ListPayments(ctx, cmd.Scope, cmd.From, cmd.Through, cmd.Status, c.Date, c.ID, cmd.Limit+1)
		if err != nil {
			return err
		}
		result.Items = items
		if len(items) > cmd.Limit {
			result.Items = items[:cmd.Limit]
			last := result.Items[cmd.Limit-1].Data()
			c.Date, c.ID = last.ScheduledDate, last.ID
			result.Cursor, err = encodePage(c)
		}
		return err
	})
	if err != nil {
		return ListPaymentsResult{}, err
	}
	return result, nil
}
func (s *service) ListPaymentMatches(ctx context.Context, cmd ListMatchesCommand) (ListMatchesResult, error) {
	if cmd.Limit == 0 {
		cmd.Limit = 50
	}
	if cmd.Scope.Validate() != nil || cmd.PaymentID == uuid.Nil || strings.TrimSpace(cmd.ActorID) == "" || cmd.Limit < 1 || cmd.Limit > 100 {
		return ListMatchesResult{}, ErrInvalid
	}
	c, err := decodePage(cmd.Cursor, pageCursor{Scope: cmd.Scope, Actor: cmd.ActorID, Kind: "matches", PaymentID: cmd.PaymentID})
	if err != nil {
		return ListMatchesResult{}, err
	}
	var result ListMatchesResult
	if s.withinTx == nil {
		return result, ErrUnavailable
	}
	err = s.withinTx(ctx, func(ctx context.Context, st store, _ commandStore) error {
		read, ok := st.(readStore)
		if !ok {
			return ErrUnavailable
		}
		if _, err := st.LoadAccount(ctx, cmd.Scope); err != nil {
			return err
		}
		_, _, found, err := st.FindPayment(ctx, cmd.Scope, cmd.PaymentID, ledger.Source{})
		if err != nil {
			return err
		}
		if !found {
			return ErrNotFound
		}
		items, err := read.ListMatches(ctx, cmd.Scope, cmd.PaymentID, c.At, c.ID, cmd.Limit+1)
		if err != nil {
			return err
		}
		result.Items = items
		if len(items) > cmd.Limit {
			result.Items = items[:cmd.Limit]
			last := result.Items[cmd.Limit-1]
			c.At, c.ID = last.CreatedAt(), last.ID()
			result.Cursor, err = encodePage(c)
		}
		return err
	})
	if err != nil {
		return ListMatchesResult{}, err
	}
	return result, nil
}
