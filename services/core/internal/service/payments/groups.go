package payments

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"

	forecast "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

type CreateGroupCommand struct {
	Scope                            ledger.Scope
	ActorID, IdempotencyKey          string
	Cutoff, HorizonStart, HorizonEnd ledger.Date
	Additional                       bool
	Operations                       []forecast.OperationReference
}
type RevokeGroupCommand struct {
	Scope                   ledger.Scope
	GroupID                 uuid.UUID
	ExpectedVersion         int64
	ActorID, IdempotencyKey string
}
type GroupResult struct {
	Group       forecast.ReconciliationGroup
	DataVersion int64
}
type ListGroupsCommand struct {
	Scope          ledger.Scope
	ActorID        string
	Limit          int
	Cursor         string
	IncludeRevoked bool
}
type ListGroupsResult struct {
	Items  []forecast.ReconciliationGroup
	Cursor string
}
type groupStore interface {
	FindReconciliationGroup(context.Context, ledger.Scope, uuid.UUID) (forecast.ReconciliationGroup, error)
	ListReconciliationGroups(context.Context, ledger.Scope, bool, uuid.UUID, int) ([]forecast.ReconciliationGroup, error)
	InsertReconciliationGroup(context.Context, forecast.ReconciliationGroup) error
	SaveReconciliationGroup(context.Context, forecast.ReconciliationGroup, int64) error
}

func validateCreateGroup(c CreateGroupCommand) error {
	if c.Scope.Validate() != nil || validateActorAndKey(c.ActorID, c.IdempotencyKey, true) != nil || c.Cutoff.Validate() != nil || c.HorizonStart.Validate() != nil || c.HorizonEnd.Validate() != nil || len(c.Operations) > 20000 || c.Additional != (len(c.Operations) == 0) {
		return ErrInvalid
	}
	cutoff, _ := time.Parse(time.DateOnly, string(c.Cutoff))
	if string(c.HorizonStart) != cutoff.AddDate(0, 0, 1).Format(time.DateOnly) || string(c.HorizonEnd) != cutoff.AddDate(0, 0, 30).Format(time.DateOnly) {
		return ErrInvalid
	}
	seen := map[uuid.UUID]bool{}
	for _, op := range c.Operations {
		if op.ID == uuid.Nil || op.Version < 1 || seen[op.ID] {
			return ErrInvalid
		}
		seen[op.ID] = true
	}
	return nil
}
func (s *service) CreateReconciliationGroup(ctx context.Context, c CreateGroupCommand) (GroupResult, error) {
	if err := validateCreateGroup(c); err != nil {
		return GroupResult{}, err
	}
	if s.withinTx == nil {
		return GroupResult{}, ErrUnavailable
	}
	var result GroupResult
	err := s.withinTx(ctx, func(ctx context.Context, st store, commands commandStore) error {
		replay, err := replayCommand(ctx, commands, c.Scope, c.ActorID, "create_reconciliation_group", "", c.IdempotencyKey, c, &result)
		if err != nil {
			return err
		}
		if replay {
			if result.Group.Scope != c.Scope || result.Group.ID == uuid.Nil || result.Group.Validate() != nil || result.DataVersion < 1 {
				return ErrUnavailable
			}
			return nil
		}
		groups, ok := st.(groupStore)
		if !ok {
			return ErrUnavailable
		}
		account, err := st.LoadAccount(ctx, c.Scope)
		if err != nil {
			return err
		}
		active, err := groups.ListReconciliationGroups(ctx, c.Scope, false, uuid.Nil, 1001)
		if err != nil {
			return err
		}
		if len(active) >= 1000 {
			return ErrInvalid
		}
		covered := map[uuid.UUID]bool{}
		for _, g := range active {
			if g.Cutoff == c.Cutoff && g.HorizonStart == c.HorizonStart && g.HorizonEnd == c.HorizonEnd {
				for _, op := range g.Operations {
					covered[op.ID] = true
				}
			}
		}
		cutoff, _ := time.Parse(time.DateOnly, string(c.Cutoff))
		from := ledger.Date(cutoff.AddDate(0, 0, -89).Format(time.DateOnly))
		for _, ref := range c.Operations {
			if covered[ref.ID] {
				return ErrInvalid
			}
			op, _, found, err := st.FindTransaction(ctx, c.Scope, ref.ID, ledger.Source{})
			if err != nil {
				return err
			}
			if !found {
				return ErrNotFound
			}
			if op.Version() != ref.Version {
				return ErrStaleVersion
			}
			if op.Data().Date < from || op.Data().Date > c.Cutoff {
				return ErrInvalid
			}
		}
		id, err := uuid.NewV7()
		if err != nil {
			return ErrUnavailable
		}
		g := forecast.ReconciliationGroup{ID: id, Scope: c.Scope, Version: 1, Status: "active", Cutoff: c.Cutoff, HorizonStart: c.HorizonStart, HorizonEnd: c.HorizonEnd, ConfirmedBy: c.ActorID, ConfirmedAt: time.Now().UTC().Truncate(time.Microsecond), Additional: c.Additional, Operations: c.Operations}
		if err := groups.InsertReconciliationGroup(ctx, g); err != nil {
			return err
		}
		previous := account.DataVersion()
		if err := account.AdvanceDataVersion(); err != nil {
			return err
		}
		if err := st.SaveAccount(ctx, account); err != nil {
			return err
		}
		if err := st.CarryHistoryCoverage(ctx, c.Scope, previous, account.DataVersion()); err != nil {
			return err
		}
		result = GroupResult{g, account.DataVersion()}
		return completeCommand(ctx, commands, c.Scope, c.ActorID, "create_reconciliation_group", "", c.IdempotencyKey, result)
	})
	if err != nil {
		return GroupResult{}, err
	}
	return result, nil
}
func (s *service) ReadReconciliationGroup(ctx context.Context, scope ledger.Scope, id uuid.UUID) (forecast.ReconciliationGroup, error) {
	if scope.Validate() != nil || id == uuid.Nil {
		return forecast.ReconciliationGroup{}, ErrInvalid
	}
	var result forecast.ReconciliationGroup
	if s.withinTx == nil {
		return result, ErrUnavailable
	}
	err := s.withinTx(ctx, func(ctx context.Context, st store, _ commandStore) error {
		groups, ok := st.(groupStore)
		if !ok {
			return ErrUnavailable
		}
		if _, err := st.LoadAccount(ctx, scope); err != nil {
			return err
		}
		var err error
		result, err = groups.FindReconciliationGroup(ctx, scope, id)
		return err
	})
	if err != nil {
		return forecast.ReconciliationGroup{}, err
	}
	return result, nil
}
func (s *service) ListReconciliationGroups(ctx context.Context, c ListGroupsCommand) (ListGroupsResult, error) {
	if c.Limit == 0 {
		c.Limit = 50
	}
	if c.Scope.Validate() != nil || strings.TrimSpace(c.ActorID) == "" || c.Limit < 1 || c.Limit > 100 {
		return ListGroupsResult{}, ErrInvalid
	}
	kind := "groups_active"
	if c.IncludeRevoked {
		kind = "groups_all"
	}
	cursor, err := decodePage(c.Cursor, pageCursor{Scope: c.Scope, Actor: c.ActorID, Kind: kind})
	if err != nil {
		return ListGroupsResult{}, err
	}
	var result ListGroupsResult
	if s.withinTx == nil {
		return result, ErrUnavailable
	}
	err = s.withinTx(ctx, func(ctx context.Context, st store, _ commandStore) error {
		groups, ok := st.(groupStore)
		if !ok {
			return ErrUnavailable
		}
		if _, err := st.LoadAccount(ctx, c.Scope); err != nil {
			return err
		}
		items, err := groups.ListReconciliationGroups(ctx, c.Scope, c.IncludeRevoked, cursor.ID, c.Limit+1)
		if err != nil {
			return err
		}
		result.Items = items
		if len(items) > c.Limit {
			result.Items = items[:c.Limit]
			cursor.ID = result.Items[c.Limit-1].ID
			result.Cursor, err = encodePage(cursor)
		}
		return err
	})
	if err != nil {
		return ListGroupsResult{}, err
	}
	return result, nil
}
func (s *service) RevokeReconciliationGroup(ctx context.Context, c RevokeGroupCommand) (GroupResult, error) {
	if c.Scope.Validate() != nil || c.GroupID == uuid.Nil || c.ExpectedVersion < 1 || validateActorAndKey(c.ActorID, c.IdempotencyKey, true) != nil {
		return GroupResult{}, ErrInvalid
	}
	if s.withinTx == nil {
		return GroupResult{}, ErrUnavailable
	}
	var result GroupResult
	err := s.withinTx(ctx, func(ctx context.Context, st store, commands commandStore) error {
		replay, err := replayCommand(ctx, commands, c.Scope, c.ActorID, "revoke_reconciliation_group", c.GroupID.String(), c.IdempotencyKey, c, &result)
		if err != nil {
			return err
		}
		if replay {
			if result.Group.Scope != c.Scope || result.Group.ID == uuid.Nil || result.Group.Validate() != nil || result.DataVersion < 1 {
				return ErrUnavailable
			}
			return nil
		}
		groups, ok := st.(groupStore)
		if !ok {
			return ErrUnavailable
		}
		account, err := st.LoadAccount(ctx, c.Scope)
		if err != nil {
			return err
		}
		g, err := groups.FindReconciliationGroup(ctx, c.Scope, c.GroupID)
		if err != nil {
			return err
		}
		if g.Version != c.ExpectedVersion {
			return ErrStaleVersion
		}
		if g.Status != "active" || g.Version == math.MaxInt64 {
			return ErrConflict
		}
		g.Status = "revoked"
		g.Version++
		if err := groups.SaveReconciliationGroup(ctx, g, c.ExpectedVersion); err != nil {
			return err
		}
		previous := account.DataVersion()
		if err := account.AdvanceDataVersion(); err != nil {
			return err
		}
		if err := st.SaveAccount(ctx, account); err != nil {
			return err
		}
		if err := st.CarryHistoryCoverage(ctx, c.Scope, previous, account.DataVersion()); err != nil {
			return err
		}
		result = GroupResult{g, account.DataVersion()}
		return completeCommand(ctx, commands, c.Scope, c.ActorID, "revoke_reconciliation_group", c.GroupID.String(), c.IdempotencyKey, result)
	})
	if err != nil {
		return GroupResult{}, err
	}
	return result, nil
}
