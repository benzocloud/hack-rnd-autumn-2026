package imports

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/imports"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

type ListCommand struct {
	Scope   ledger.Scope
	ActorID string
	Limit   int
	Cursor  string
}
type ListResult struct {
	Items  []domain.Record
	Cursor string
}
type IssuesCommand struct {
	Scope   ledger.Scope
	ActorID string
	ID      uuid.UUID
	Limit   int
	Cursor  string
}
type IssuesResult struct {
	Items  []domain.Issue
	Cursor string
}
type cursor struct {
	Scope    ledger.Scope
	Actor    string
	ID       uuid.UUID
	Before   time.Time
	Offset   int64
	ImportID uuid.UUID
}

func decodeCursor(raw string, c *cursor) error {
	if raw == "" {
		return nil
	}
	if len(raw) > 1024 {
		return domain.ErrCursor
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return domain.ErrCursor
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(c) != nil || d.Decode(new(any)) != io.EOF {
		return domain.ErrCursor
	}
	return nil
}
func encodeCursor(c cursor) (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", domain.ErrUnavailable
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *service) Read(ctx context.Context, scope ledger.Scope, id uuid.UUID) (domain.Record, error) {
	if scope.Validate() != nil || id == uuid.Nil {
		return domain.Record{}, domain.ErrInvalid
	}
	var r domain.Record
	err := s.withinTx(ctx, func(ctx context.Context, st store) error { var err error; r, err = st.Read(ctx, scope, id); return err })
	if err != nil {
		return domain.Record{}, err
	}
	if r.Status == "receiving" {
		return domain.Record{}, domain.ErrNotFound
	}
	return r, nil
}
func (s *service) List(ctx context.Context, cmd ListCommand) (ListResult, error) {
	if cmd.Limit == 0 {
		cmd.Limit = 50
	}
	if cmd.Scope.Validate() != nil || cmd.ActorID == "" || cmd.Limit < 1 || cmd.Limit > 100 {
		return ListResult{}, domain.ErrInvalid
	}
	c := cursor{Scope: cmd.Scope, Actor: cmd.ActorID}
	if err := decodeCursor(cmd.Cursor, &c); err != nil {
		return ListResult{}, err
	}
	if c.Scope != cmd.Scope || c.Actor != cmd.ActorID || (cmd.Cursor != "" && (c.ID == uuid.Nil || c.Before.IsZero() || c.ImportID != uuid.Nil)) {
		return ListResult{}, domain.ErrCursor
	}
	var items []domain.Record
	err := s.withinTx(ctx, func(ctx context.Context, st store) error {
		var err error
		items, err = st.List(ctx, cmd.Scope, cmd.Limit+1, c.Before, c.ID)
		return err
	})
	if err != nil {
		return ListResult{}, err
	}
	result := ListResult{Items: items}
	if len(items) > cmd.Limit {
		last := items[cmd.Limit-1]
		result.Items = items[:cmd.Limit]
		result.Cursor, err = encodeCursor(cursor{Scope: cmd.Scope, Actor: cmd.ActorID, Before: last.CreatedAt, ID: last.ID})
	}
	return result, err
}
func (s *service) Issues(ctx context.Context, cmd IssuesCommand) (IssuesResult, error) {
	if cmd.Limit == 0 {
		cmd.Limit = 50
	}
	if cmd.Scope.Validate() != nil || cmd.ID == uuid.Nil || cmd.ActorID == "" || cmd.Limit < 1 || cmd.Limit > 100 {
		return IssuesResult{}, domain.ErrInvalid
	}
	c := cursor{Scope: cmd.Scope, Actor: cmd.ActorID, ImportID: cmd.ID}
	if err := decodeCursor(cmd.Cursor, &c); err != nil {
		return IssuesResult{}, err
	}
	if c.Scope != cmd.Scope || c.Actor != cmd.ActorID || c.ImportID != cmd.ID || c.Offset < 0 {
		return IssuesResult{}, domain.ErrCursor
	}
	var issues []domain.Issue
	var ids []int64
	err := s.withinTx(ctx, func(ctx context.Context, st store) error {
		r, err := st.Read(ctx, cmd.Scope, cmd.ID)
		if err != nil {
			return err
		}
		if r.Status == "receiving" {
			return domain.ErrNotFound
		}
		issues, ids, err = st.Issues(ctx, cmd.ID, c.Offset, cmd.Limit+1)
		return err
	})
	if err != nil {
		return IssuesResult{}, err
	}
	result := IssuesResult{Items: issues}
	if len(issues) > cmd.Limit {
		result.Items = issues[:cmd.Limit]
		c.Offset = ids[cmd.Limit-1]
		result.Cursor, err = encodeCursor(c)
	}
	return result, err
}
