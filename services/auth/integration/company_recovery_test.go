//go:build integration

package integration_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/service/account"
)

func TestCompanyRecoveryRealDatabaseLostReplyRestartAndTwoReplicas(t *testing.T) {
	cfg, db, accounts := accountFixture(t, true)
	challenge, browser := registerAccount(t, accounts, "alice@example.test")
	if err := accounts.VerifyEmail(t.Context(), account.VerifyEmailInput{Context: accountWrite(browser), ChallengeID: challenge.ID, Code: "000000"}); err != nil {
		t.Fatal(err)
	}
	login := loginAccount(t, accounts, browser, "alice@example.test")
	var mu sync.Mutex
	var receipt *account.CreationReceipt
	bootstraps := 0
	core := creationCoreFixture{read: func(context.Context, uuid.UUID) (account.CreationReceipt, error) {
		mu.Lock()
		defer mu.Unlock()
		if receipt == nil {
			return account.CreationReceipt{}, account.ErrNotFound
		}
		return *receipt, nil
	}, bootstrap: func(_ context.Context, command account.CreationCommand) error {
		mu.Lock()
		defer mu.Unlock()
		bootstraps++
		stored := account.CreationReceipt{CommandID: command.ID, CompanyID: command.CompanyID, AccountID: uuid.Must(uuid.NewV7()), Spec: command.Spec, RequestSHA256: command.RequestSHA256, CommittedAt: time.Now().UTC()}
		receipt = &stored
		// Boundary fixture commits but loses the response. Core runtime is an A5 gate.
		return context.DeadlineExceeded
	}}
	first := newCreationAccounts(t, cfg, openDatabase(t, cfg), core)
	input := account.BeginCompanyCreationInput{Context: accountWrite(login.Context), Key: "recovery-lost-reply-key", Spec: account.CompanySpec{Name: "Recovery", Currency: "RUB", Timezone: "Europe/Moscow"}}
	command, err := first.BeginCompanyCreation(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if found, err := first.ReconcileOne(t.Context()); !found || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(found, err)
	}
	var state string
	var members int
	var lease *uuid.UUID
	if err := db.QueryRow(t.Context(), "SELECT state,lease_token FROM company_creation_commands WHERE id=$1", command.ID).Scan(&state, &lease); err != nil {
		t.Fatal(err)
	}
	if state != "PREPARED" || lease != nil {
		t.Fatal("lost reply finalized or retained live lease", state, lease)
	}
	execAccountFixture(t, db, "UPDATE security_contexts SET expires_at=created_at+interval '1 microsecond' WHERE kind='session'")
	execAccountFixture(t, db, "UPDATE users SET credential_version=credential_version+1 WHERE id=$1", login.User.ID)
	execAccountFixture(t, db, "UPDATE company_creation_commands SET next_attempt_at=clock_timestamp()-interval '1 second' WHERE id=$1", command.ID)
	a, b := newCreationAccounts(t, cfg, openDatabase(t, cfg), core), newCreationAccounts(t, cfg, openDatabase(t, cfg), core)
	if _, err := a.BeginCompanyCreation(t.Context(), input); !errors.Is(err, account.ErrAuthRequired) {
		t.Fatal("expired/reset session admitted browser request", err)
	}
	var wg sync.WaitGroup
	results := make(chan bool, 2)
	errs := make(chan error, 2)
	for _, service := range []creationOperations{a, b} {
		wg.Go(func() { found, err := service.ReconcileOne(t.Context()); results <- found; errs <- err })
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	claims := 0
	for found := range results {
		if found {
			claims++
		}
	}
	if claims != 1 {
		t.Fatal("replicas shared a live claim", claims)
	}
	if err := db.QueryRow(t.Context(), "SELECT state FROM company_creation_commands WHERE id=$1", command.ID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(t.Context(), "SELECT count(*) FROM memberships WHERE company_id=$1 AND user_id=$2 AND role='owner'", command.CompanyID, login.User.ID).Scan(&members); err != nil {
		t.Fatal(err)
	}
	if state != "COMPLETED" || members != 1 || bootstraps != 1 {
		t.Fatal(state, members, bootstraps)
	}
	freshBrowser, err := accounts.GetSecurityContext(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	relogin := loginAccount(t, accounts, freshBrowser, "alice@example.test")
	page, err := accounts.ListMemberships(t.Context(), relogin.Context.CookieToken, 10, "")
	if err != nil || len(page.Items) != 1 || page.Items[0].CompanyID != command.CompanyID || page.Items[0].Role != "owner" {
		t.Fatal(page, err)
	}
}

func TestCompanyRecoveryQuarantinesDisabledOrDeletedBeforeCore(t *testing.T) {
	for _, mode := range []string{"disabled", "deleted"} {
		t.Run(mode, func(t *testing.T) {
			cfg, db, accounts := accountFixture(t, true)
			challenge, browser := registerAccount(t, accounts, "alice@example.test")
			if err := accounts.VerifyEmail(t.Context(), account.VerifyEmailInput{Context: accountWrite(browser), ChallengeID: challenge.ID, Code: "000000"}); err != nil {
				t.Fatal(err)
			}
			login := loginAccount(t, accounts, browser, "alice@example.test")
			core := creationCoreFixture{read: func(context.Context, uuid.UUID) (account.CreationReceipt, error) {
				t.Error("ineligible initiator reached Core")
				return account.CreationReceipt{}, account.ErrUnavailable
			}}
			service := newCreationAccounts(t, cfg, openDatabase(t, cfg), core)
			command, err := service.BeginCompanyCreation(t.Context(), account.BeginCompanyCreationInput{Context: accountWrite(login.Context), Key: "quarantine-recovery-key", Spec: account.CompanySpec{Name: "Company", Currency: "RUB", Timezone: "Europe/Moscow"}})
			if err != nil {
				t.Fatal(err)
			}
			if mode == "disabled" {
				execAccountFixture(t, db, "UPDATE users SET state='disabled' WHERE id=$1", login.User.ID)
			} else {
				execAccountFixture(t, db, "DELETE FROM users WHERE id=$1", login.User.ID)
			}
			if found, err := service.ReconcileOne(t.Context()); !found || err != nil {
				t.Fatal(found, err)
			}
			var state string
			var members int
			if err := db.QueryRow(t.Context(), "SELECT state FROM company_creation_commands WHERE id=$1", command.ID).Scan(&state); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow(t.Context(), "SELECT count(*) FROM memberships").Scan(&members); err != nil {
				t.Fatal(err)
			}
			if state != "QUARANTINED" || members != 0 {
				t.Fatal(state, members)
			}
		})
	}
}
