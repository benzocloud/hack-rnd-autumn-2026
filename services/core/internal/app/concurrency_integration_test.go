//go:build integration

package app

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	domainPayments "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
	ledgerService "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
	paymentService "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/payments"
)

func TestConcurrentIdenticalCommand(t *testing.T) {
	application, _ := testApplication(t)
	scope := newScope(t, application)
	cmd := transactionCommand(t, scope)
	const workers = 8
	results := make([]ledgerService.CreateTransactionResult, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range workers {
		wg.Go(func() { <-start; results[i], errs[i] = application.Ledger.CreateTransaction(t.Context(), cmd) })
	}
	close(start)
	wg.Wait()
	for i := range workers {
		if errs[i] != nil || results[i] != results[0] {
			t.Fatalf("worker %d replay=%+v err=%v", i, results[i], errs[i])
		}
	}
	assertRevision(t, application, scope, 2)
	assertCount(t, application, "SELECT count(*) FROM core_transactions WHERE company_id=$1", scope, 1)
	assertCount(t, application, "SELECT count(*) FROM core_command_results WHERE company_id=$1", scope, 1)
}

func TestConcurrentFactualNamespaces(t *testing.T) {
	application, _ := testApplication(t)
	scope := newScope(t, application)
	commands := []ledgerService.TransactionCommand{transactionCommand(t, scope), transactionCommand(t, scope)}
	for i := range commands {
		commands[i].Record.Source = ledger.Source{Kind: ledger.Imported, Namespace: ledger.Namespace{System: "bank", CompanyID: []string{"C001", "C002"}[i]}, ExternalID: "OP1", ImportedAt: time.Now()}
		commands[i].ActorID = ""
		commands[i].IdempotencyKey = ""
	}
	errs := make([]error, 2)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range commands {
		wg.Go(func() { <-start; _, errs[i] = application.Ledger.CreateTransaction(t.Context(), commands[i]) })
	}
	close(start)
	wg.Wait()
	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
		} else {
			requireError(t, err, ledgerService.ErrStaleVersion)
		}
	}
	if successes != 1 {
		t.Fatalf("accepted %d competing factual sources", successes)
	}
	assertRevision(t, application, scope, 2)
	assertCount(t, application, "SELECT count(*) FROM core_source_bindings WHERE company_id=$1", scope, 1)
}

func TestConcurrentMatchesCannotOverallocate(t *testing.T) {
	for _, mode := range []string{"account CAS", "resource versions"} {
		t.Run(mode, func(t *testing.T) {
			application, _ := testApplication(t)
			scope := newScope(t, application)
			fact, err := application.Ledger.CreateTransaction(t.Context(), transactionCommand(t, scope))
			if err != nil {
				t.Fatal(err)
			}
			first, err := application.Payments.CreatePayment(t.Context(), paymentCommand(t, scope, 2))
			if err != nil {
				t.Fatal(err)
			}
			second, err := application.Payments.CreatePayment(t.Context(), paymentCommand(t, scope, 3))
			if err != nil {
				t.Fatal(err)
			}
			cmds := []paymentService.MatchCommand{matchCommand(t, scope, fact.TransactionID, first.PaymentID, 4), matchCommand(t, scope, fact.TransactionID, second.PaymentID, 4)}
			if mode == "resource versions" {
				for i := range cmds {
					cmds[i].ExpectedDataVersion = 0
					cmds[i].ExpectedTransactionVersion = fact.TransactionVersion
				}
			}
			errs := make([]error, 2)
			var wg sync.WaitGroup
			start := make(chan struct{})
			for i := range cmds {
				wg.Go(func() { <-start; _, errs[i] = application.Payments.MatchPayment(t.Context(), cmds[i]) })
			}
			close(start)
			wg.Wait()
			successes, loser := 0, -1
			for i, err := range errs {
				switch {
				case err == nil:
					successes++
				case mode == "account CAS" && errors.Is(err, paymentService.ErrStaleVersion),
					mode == "resource versions" && errors.Is(err, domainPayments.ErrAllocation):
					loser = i
				default:
					t.Fatal(err)
				}
			}
			if successes != 1 || loser < 0 {
				t.Fatalf("competing matches: %v", errs)
			}
			// Retry the losing payment against the current revision: the fact only has 40 left.
			if mode == "account CAS" {
				cmds[loser].ExpectedDataVersion = 5
			}
			cmds[loser].IdempotencyKey = uuid.NewString()
			_, err = application.Payments.MatchPayment(t.Context(), cmds[loser])
			requireError(t, err, domainPayments.ErrAllocation)
			assertRevision(t, application, scope, 5)
			assertCount(t, application, "SELECT count(*) FROM core_payment_matches WHERE company_id=$1", scope, 1)
		})
	}
}
