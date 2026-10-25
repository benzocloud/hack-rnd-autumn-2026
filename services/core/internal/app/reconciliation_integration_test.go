//go:build integration

package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"

	forecast "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	service "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/payments"
)

func TestReconciliationGuardsScopeVersionsOverlapAndReplay(t *testing.T) {
	a, _ := testApplication(t)
	scope := newScope(t, a)
	other := newScope(t, a)
	ctx := t.Context()
	operations := a.Payments.(interface {
		CreateReconciliationGroup(context.Context, service.CreateGroupCommand) (service.GroupResult, error)
		RevokeReconciliationGroup(context.Context, service.RevokeGroupCommand) (service.GroupResult, error)
	})
	fact, err := a.Ledger.CreateTransaction(ctx, transactionCommand(t, scope))
	if err != nil {
		t.Fatal(err)
	}
	cmd := service.CreateGroupCommand{Scope: scope, ActorID: "test-owner", IdempotencyKey: uuid.NewString(), Cutoff: "2026-10-23", HorizonStart: "2026-10-24", HorizonEnd: "2026-11-22", Operations: []forecast.OperationReference{{ID: fact.TransactionID, Version: 1}}}
	first, err := operations.CreateReconciliationGroup(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	again, err := operations.CreateReconciliationGroup(ctx, cmd)
	if err != nil || !reflect.DeepEqual(first, again) {
		t.Fatal("group replay", again, err)
	}
	conflict := cmd
	conflict.Additional = true
	conflict.Operations = nil
	if _, err := operations.CreateReconciliationGroup(ctx, conflict); !errors.Is(err, service.ErrIdempotencyConflict) {
		t.Fatal("group body conflict", err)
	}
	overlap := cmd
	overlap.IdempotencyKey = uuid.NewString()
	if _, err := operations.CreateReconciliationGroup(ctx, overlap); !errors.Is(err, service.ErrInvalid) {
		t.Fatal("overlapping replacement accepted", err)
	}
	foreign := cmd
	foreign.Scope = other
	foreign.IdempotencyKey = uuid.NewString()
	if _, err := operations.CreateReconciliationGroup(ctx, foreign); !errors.Is(err, service.ErrNotFound) {
		t.Fatal("foreign reference", err)
	}
	bad := cmd
	bad.IdempotencyKey = uuid.NewString()
	bad.Cutoff = "2026-10-24"
	bad.HorizonStart = "2026-10-25"
	bad.HorizonEnd = "2026-11-23"
	bad.Operations = []forecast.OperationReference{{ID: fact.TransactionID, Version: 2}}
	if _, err := operations.CreateReconciliationGroup(ctx, bad); !errors.Is(err, service.ErrStaleVersion) {
		t.Fatal("stale factual reference", err)
	}
	attach := paymentCommand(t, other, 1)
	attach.Record.ReconciliationGroupID = first.Group.ID
	if _, err := a.Payments.CreatePayment(ctx, attach); !errors.Is(err, service.ErrNotFound) {
		t.Fatal("foreign group linkage", err)
	}
	revoke := service.RevokeGroupCommand{Scope: scope, GroupID: first.Group.ID, ExpectedVersion: 2, ActorID: "test-owner", IdempotencyKey: uuid.NewString()}
	if _, err := operations.RevokeReconciliationGroup(ctx, revoke); !errors.Is(err, service.ErrStaleVersion) {
		t.Fatal("stale revoke", err)
	}
	revoke.ExpectedVersion = 1
	revoked, err := operations.RevokeReconciliationGroup(ctx, revoke)
	if err != nil || revoked.Group.Status != "revoked" {
		t.Fatal(revoked, err)
	}
	replay, err := operations.RevokeReconciliationGroup(ctx, revoke)
	if err != nil || !reflect.DeepEqual(replay, revoked) {
		t.Fatal("revoke replay", replay, err)
	}
	attach = paymentCommand(t, scope, 4)
	attach.Record.ReconciliationGroupID = first.Group.ID
	if _, err := a.Payments.CreatePayment(ctx, attach); !errors.Is(err, service.ErrConflict) {
		t.Fatal("revoked group linked", err)
	}
	assertRevision(t, a, scope, 4)
	assertRevision(t, a, other, 1)
	// An explicit additional confirmation has no inferred historical exclusion.
	additional := service.CreateGroupCommand{Scope: other, ActorID: "test-owner", IdempotencyKey: uuid.NewString(), Cutoff: ledger.Date("2026-10-23"), HorizonStart: "2026-10-24", HorizonEnd: "2026-11-22", Additional: true}
	if _, err := operations.CreateReconciliationGroup(ctx, additional); err != nil {
		t.Fatal(err)
	}
}
