//go:build integration && mlintegration

package app

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
	forecastDomain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	paymentDomain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/payments"
)

func TestPaymentForecastFrozenCalendarPostgresPythonRestart(t *testing.T) {
	f := newManualBalanceFixture(t)
	cutoff, _ := seedForecastHistory(t, f, true)
	at, _ := time.Parse(time.DateOnly, string(cutoff))
	groups := f.app.Payments.(interface {
		CreateReconciliationGroup(context.Context, payments.CreateGroupCommand) (payments.GroupResult, error)
		RevokeReconciliationGroup(context.Context, payments.RevokeGroupCommand) (payments.GroupResult, error)
	})
	group, err := groups.CreateReconciliationGroup(t.Context(), payments.CreateGroupCommand{Scope: f.scope, ActorID: "auth-owned-user", IdempotencyKey: uuid.NewString(), Cutoff: cutoff, HorizonStart: ledger.Date(at.AddDate(0, 0, 1).Format(time.DateOnly)), HorizonEnd: ledger.Date(at.AddDate(0, 0, 30).Format(time.DateOnly)), Additional: true})
	if err != nil {
		t.Fatal(err)
	}
	created, err := f.app.Payments.CreatePayment(t.Context(), payments.CreateCommand{Record: paymentDomain.PaymentData{Scope: f.scope, Direction: ledger.Outflow, Amount: rub(t, 5000), ScheduledDate: ledger.Date(at.AddDate(0, 0, 2).Format(time.DateOnly)), Source: ledger.Source{Kind: ledger.Manual}, ReconciliationGroupID: group.Group.ID}, ActorID: "auth-owned-user", IdempotencyKey: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	jobs := newForecastJobs(f.app.db, config.Postgres{QueryTimeout: 5 * time.Second}, pythonFinancialModel(t, f))
	accept := func() forecasting.ForecastAccepted {
		t.Helper()
		accepted, err := jobs.Create(t.Context(), forecasting.CreateForecastCommand{Scope: f.scope, ActorID: "auth-owned-user", IdempotencyKey: uuid.NewString(), Cutoff: cutoff, Today: f.today, HorizonDays: 30})
		if err != nil {
			t.Fatal(err)
		}
		return accepted
	}
	first := accept()
	// Change live state before the worker starts: its only calendar is the saved
	// snapshot. This is a real PostgreSQL roundtrip, not a predictor test double.
	editedAmount := rub(t, 7000)
	changed, err := f.app.Payments.UpdatePayment(t.Context(), payments.UpdatePaymentCommand{Scope: f.scope, PaymentID: created.PaymentID, ExpectedVersion: 1, ActorID: "auth-owned-user", Change: paymentDomain.PaymentChange{Amount: &editedAmount}})
	if err != nil {
		t.Fatal(err)
	}
	if worked, err := jobs.ProcessOne(t.Context()); err != nil || !worked {
		t.Fatal(worked, err)
	}
	firstView, err := jobs.Read(t.Context(), f.scope, first.ID)
	if err != nil || firstView.Status != "succeeded" || firstView.Result == nil || len(firstView.Result.Days) != 30 || firstView.Result.Days[29].Balance != -7700 || firstView.CurrentDataVersion == firstView.DataVersion {
		t.Fatal("frozen first forecast", firstView.Status, err)
	}
	if firstView.Snapshot.Data().Calendar.Payments[0].Remaining().Minor() != 5000 {
		t.Fatal("worker re-read live calendar")
	}
	var savedInput, savedResult []byte
	if err := f.app.db.QueryRowxContext(t.Context(), "SELECT input,result FROM core_forecast_jobs WHERE company_id=$1 AND account_id=$2 AND id=$3", f.scope.CompanyID, f.scope.AccountID, first.ID).Scan(&savedInput, &savedResult); err != nil {
		t.Fatal(err)
	}
	check := func(want int64) forecasting.ForecastView {
		t.Helper()
		accepted := accept()
		if worked, err := jobs.ProcessOne(t.Context()); err != nil || !worked {
			t.Fatal(worked, err)
		}
		view, err := jobs.Read(t.Context(), f.scope, accepted.ID)
		if err != nil || view.Status != "succeeded" || view.Result == nil || len(view.Result.Days) != 30 || view.Result.Days[29].Balance != want {
			t.Fatal("new forecast", view.Status, want, err)
		}
		return view
	}
	check(-9700)
	newDate := ledger.Date(at.AddDate(0, 0, 3).Format(time.DateOnly))
	changed, err = f.app.Payments.UpdatePayment(t.Context(), payments.UpdatePaymentCommand{Scope: f.scope, PaymentID: created.PaymentID, ExpectedVersion: changed.Payment.Version(), ActorID: "auth-owned-user", Change: paymentDomain.PaymentChange{ScheduledDate: &newDate}})
	if err != nil {
		t.Fatal(err)
	}
	dated := check(-9700)
	if dated.Result.Days[1].Balance != 100 || dated.Result.Days[2].Balance != -7000 {
		t.Fatal("payment date ignored", dated.Result.Days[:3])
	}
	var transactionID uuid.UUID
	if err := f.app.db.GetContext(t.Context(), &transactionID, "SELECT id FROM core_transactions WHERE company_id=$1 AND account_id=$2 AND direction='outflow' ORDER BY operation_date,id LIMIT 1", f.scope.CompanyID, f.scope.AccountID); err != nil {
		t.Fatal(err)
	}
	matched, err := f.app.Payments.MatchPayment(t.Context(), payments.MatchCommand{Scope: f.scope, PaymentID: created.PaymentID, TransactionID: transactionID, Amount: rub(t, 100), ExpectedVersion: changed.Payment.Version(), ExpectedTransactionVersion: 1, ActorID: "auth-owned-user", IdempotencyKey: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	partial := check(-9600)
	if partial.Snapshot.Data().Calendar.Payments[0].Remaining().Minor() != 6900 {
		t.Fatal("wrong frozen remaining")
	}
	unmatched, err := f.app.Payments.RemovePaymentMatch(t.Context(), payments.RemoveMatchCommand{Scope: f.scope, PaymentID: created.PaymentID, MatchID: matched.MatchID, ExpectedVersion: matched.PaymentVersion, ActorID: "auth-owned-user"})
	if err != nil {
		t.Fatal(err)
	}
	check(-9700)
	matched, err = f.app.Payments.MatchPayment(t.Context(), payments.MatchCommand{Scope: f.scope, PaymentID: created.PaymentID, TransactionID: transactionID, Amount: rub(t, 100), ExpectedVersion: unmatched.Payment.Version(), ExpectedTransactionVersion: 1, ActorID: "auth-owned-user", IdempotencyKey: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := f.app.Payments.CancelPayment(t.Context(), payments.CancelPaymentCommand{Scope: f.scope, PaymentID: created.PaymentID, ExpectedVersion: matched.PaymentVersion, ActorID: "auth-owned-user"})
	if err != nil || cancelled.Payment.Remaining().Minor() != 6900 {
		t.Fatal("partial cancel lost remaining", err)
	}
	check(-2700)
	if _, err := f.app.Payments.RemovePaymentMatch(t.Context(), payments.RemoveMatchCommand{Scope: f.scope, PaymentID: created.PaymentID, MatchID: matched.MatchID, ExpectedVersion: cancelled.Payment.Version(), ActorID: "auth-owned-user"}); !errors.Is(err, paymentDomain.ErrState) {
		t.Fatal("unmatch after cancel must explicitly reject", err)
	}
	replacement, err := groups.CreateReconciliationGroup(t.Context(), payments.CreateGroupCommand{Scope: f.scope, ActorID: "auth-owned-user", IdempotencyKey: uuid.NewString(), Cutoff: cutoff, HorizonStart: group.Group.HorizonStart, HorizonEnd: group.Group.HorizonEnd, Operations: []forecastDomain.OperationReference{{ID: transactionID, Version: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	check(-2640)
	if _, err := groups.RevokeReconciliationGroup(t.Context(), payments.RevokeGroupCommand{Scope: f.scope, GroupID: replacement.Group.ID, ExpectedVersion: 1, ActorID: "auth-owned-user", IdempotencyKey: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	check(-2700)
	second, cfg := testApplication(t)
	restarted := newForecastJobs(second.db, cfg.Postgres, pythonFinancialModel(t, f))
	old, err := restarted.Read(t.Context(), f.scope, first.ID)
	if err != nil || old.Result.Days[29].Balance != -7700 || old.Snapshot.Data().Calendar.Payments[0].Version() != 1 {
		t.Fatal("restart changed frozen result", err)
	}
	var inputAfter, resultAfter []byte
	if err := second.db.QueryRowxContext(t.Context(), "SELECT input,result FROM core_forecast_jobs WHERE company_id=$1 AND account_id=$2 AND id=$3", f.scope.CompanyID, f.scope.AccountID, first.ID).Scan(&inputAfter, &resultAfter); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(savedInput, inputAfter) || !bytes.Equal(savedResult, resultAfter) {
		t.Fatal("old durable job mutated")
	}
}
