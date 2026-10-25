//go:build integration

package app

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
	service "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/payments"
)

func TestPaymentLifecyclePersistsAuditAndOriginalReplay(t *testing.T) {
	application, _ := testApplication(t)
	scope := newScope(t, application)
	ctx := t.Context()
	create := paymentCommand(t, scope, 1)
	first, err := application.Payments.CreatePayment(ctx, create)
	if err != nil {
		t.Fatal(err)
	}
	amount := rub(t, 150)
	edit := service.UpdatePaymentCommand{Scope: scope, PaymentID: first.PaymentID, ExpectedVersion: 1, ActorID: "test-owner", Change: domain.PaymentChange{Amount: &amount}}
	edited, err := application.Payments.UpdatePayment(ctx, edit)
	if err != nil || edited.Payment.Remaining().Minor() != 150 {
		t.Fatal(edited, err)
	}
	if _, err := application.Payments.UpdatePayment(ctx, edit); !errors.Is(err, service.ErrStaleVersion) {
		t.Fatal("stale edit", err)
	}
	date := ledger.Date("2026-10-30")
	edit.ExpectedVersion = 2
	edit.Change = domain.PaymentChange{ScheduledDate: &date}
	edited, err = application.Payments.UpdatePayment(ctx, edit)
	if err != nil || edited.Payment.Data().ScheduledDate != date {
		t.Fatal(edited, err)
	}
	factCmd := transactionCommand(t, scope)
	factCmd.ExpectedDataVersion = 4
	fact, err := application.Ledger.CreateTransaction(ctx, factCmd)
	if err != nil {
		t.Fatal(err)
	}
	match := matchCommand(t, scope, fact.TransactionID, first.PaymentID, 5)
	match.ExpectedVersion = 3
	match.ExpectedTransactionVersion = 1
	allocated, err := application.Payments.MatchPayment(ctx, match)
	if err != nil {
		t.Fatal(err)
	}
	removed, err := application.Payments.RemovePaymentMatch(ctx, service.RemoveMatchCommand{Scope: scope, PaymentID: first.PaymentID, MatchID: allocated.MatchID, ExpectedVersion: 4, ActorID: "test-owner"})
	if err != nil || removed.Payment.Remaining().Minor() != 150 || removed.Payment.Status() != domain.Planned {
		t.Fatal(removed, err)
	}
	match.ExpectedVersion = 5
	match.ExpectedDataVersion = 7
	match.IdempotencyKey = uuid.NewString()
	allocated, err = application.Payments.MatchPayment(ctx, match)
	if err != nil {
		t.Fatal("reversed amount must be available again", err)
	}
	cancelled, err := application.Payments.CancelPayment(ctx, service.CancelPaymentCommand{Scope: scope, PaymentID: first.PaymentID, ExpectedVersion: 6, ActorID: "test-owner"})
	if err != nil || cancelled.Payment.Remaining().Minor() != 90 || cancelled.Payment.Status() != domain.Cancelled {
		t.Fatal(cancelled, err)
	}
	_, err = application.Payments.RemovePaymentMatch(ctx, service.RemoveMatchCommand{Scope: scope, PaymentID: first.PaymentID, MatchID: allocated.MatchID, ExpectedVersion: 7, ActorID: "test-owner"})
	if !errors.Is(err, domain.ErrState) {
		t.Fatal("cancelled allocation must remain", err)
	}
	history, err := application.Payments.ListPaymentMatches(ctx, service.ListMatchesCommand{Scope: scope, PaymentID: first.PaymentID, ActorID: "test-owner", Limit: 1})
	if err != nil || len(history.Items) != 1 || history.Items[0].RemovedAt() == nil || history.Cursor == "" {
		t.Fatal(history, err)
	}
	next, err := application.Payments.ListPaymentMatches(ctx, service.ListMatchesCommand{Scope: scope, PaymentID: first.PaymentID, ActorID: "test-owner", Limit: 1, Cursor: history.Cursor})
	if err != nil || len(next.Items) != 1 || next.Items[0].RemovedAt() != nil || next.Cursor != "" {
		t.Fatal(next, err)
	}
	_, err = application.Payments.ListPaymentMatches(ctx, service.ListMatchesCommand{Scope: scope, PaymentID: first.PaymentID, ActorID: "other", Cursor: history.Cursor})
	if !errors.Is(err, service.ErrCursor) {
		t.Fatal("foreign cursor", err)
	}
	replay, err := application.Payments.CreatePayment(ctx, create)
	if err != nil || replay != first || replay.Payment.Remaining().Minor() != 100 || replay.Payment.Version() != 1 {
		t.Fatal("create replay changed", replay, err)
	}
	matchReplay, err := application.Payments.MatchPayment(ctx, match)
	if err != nil || matchReplay.MatchID != allocated.MatchID || matchReplay.PaymentVersion != allocated.PaymentVersion {
		t.Fatal("match replay changed", matchReplay, err)
	}
	create.Record.Description = "different"
	if _, err := application.Payments.CreatePayment(ctx, create); !errors.Is(err, service.ErrIdempotencyConflict) {
		t.Fatal("body conflict", err)
	}
	active, err := application.Payments.ListPayments(ctx, service.ListPaymentsCommand{Scope: scope, ActorID: "test-owner"})
	if err != nil || len(active.Items) != 0 {
		t.Fatal(active, err)
	}
	settled, err := application.Payments.ListPayments(ctx, service.ListPaymentsCommand{Scope: scope, ActorID: "test-owner", Status: domain.Cancelled})
	if err != nil || len(settled.Items) != 1 {
		t.Fatal(settled, err)
	}
	got, err := application.Payments.ReadPayment(ctx, scope, first.PaymentID)
	if err != nil || got != cancelled.Payment {
		t.Fatal(got, err)
	}
	other := newScope(t, application)
	if _, err := application.Payments.ReadPayment(ctx, other, first.PaymentID); !errors.Is(err, service.ErrNotFound) {
		t.Fatal("foreign read", err)
	}
	assertRevision(t, application, scope, 9)
	assertCount(t, application, "SELECT count(*) FROM core_transactions WHERE company_id=$1", scope, 1)
	assertCount(t, application, "SELECT count(*) FROM core_payment_matches WHERE company_id=$1", scope, 2)
}
