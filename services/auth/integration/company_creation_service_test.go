//go:build integration

package integration_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/app/consent"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/config"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/challenge"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/companycreation"
	consentSQL "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/consent"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/membership"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/postgres"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/securitycontext"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/user"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/service/account"
)

type creationOperations interface {
	ReconcileOne(context.Context) (bool, error)
	BeginCompanyCreation(context.Context, account.BeginCompanyCreationInput) (account.CreationCommand, error)
	CompleteCompanyCreation(context.Context, account.CompleteCompanyCreationInput) (account.CreationCommand, error)
	AuthorizeCompanyBootstrap(context.Context, account.AuthorizeCompanyBootstrapInput) (account.CreationCommand, error)
}

// This is a boundary fixture for A1. Real mTLS Core receipts are an A2/A5 gate.
type creationCoreFixture struct {
	read      func(context.Context, uuid.UUID) (account.CreationReceipt, error)
	bootstrap func(context.Context, account.CreationCommand) error
}

func (c creationCoreFixture) GetCreationResult(ctx context.Context, id uuid.UUID) (account.CreationReceipt, error) {
	return c.read(ctx, id)
}
func (c creationCoreFixture) BootstrapCompany(ctx context.Context, command account.CreationCommand) error {
	if c.bootstrap != nil {
		return c.bootstrap(ctx, command)
	}
	return account.ErrUnavailable
}

func newCreationAccounts(t *testing.T, cfg config.Config, pool *sqlx.DB, core creationCoreFixture) creationOperations {
	t.Helper()
	service, err := account.New(func(ctx context.Context, work account.Work) error {
		return postgres.WithTx(ctx, pool, cfg.QueryTimeout, func(ctx context.Context, tx *sqlx.Tx) error {
			return work(ctx, user.NewRepository(tx, cfg.QueryTimeout), securitycontext.NewRepository(tx, cfg.QueryTimeout), challenge.NewRepository(tx, cfg.QueryTimeout), consentSQL.NewRepository(tx, cfg.QueryTimeout), membership.NewRepository(tx, cfg.QueryTimeout), companycreation.NewRepository(tx, cfg.QueryTimeout))
		})
	}, account.Options{Core: core, CodeKey: cfg.CodeHMACKey(), CSRFKey: cfg.CSRFHMACKey(), Demo: cfg.DemoEmailVerification, DemoEmails: cfg.DemoTestEmails, Consent: consent.Document()})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestCompanyCreationServiceTwoReplicasAndRestart(t *testing.T) {
	cfg, db, accounts := accountFixture(t, true)
	challenge, browser := registerAccount(t, accounts, "alice@example.test")
	if err := accounts.VerifyEmail(t.Context(), account.VerifyEmailInput{Context: accountWrite(browser), ChallengeID: challenge.ID, Code: "000000"}); err != nil {
		t.Fatal(err)
	}
	login := loginAccount(t, accounts, browser, "alice@example.test")
	poolA, poolB := openDatabase(t, cfg), openDatabase(t, cfg)
	core := creationCoreFixture{read: func(context.Context, uuid.UUID) (account.CreationReceipt, error) {
		return account.CreationReceipt{}, account.ErrNotFound
	}}
	a, b := newCreationAccounts(t, cfg, poolA, core), newCreationAccounts(t, cfg, poolB, core)
	input := account.BeginCompanyCreationInput{Context: accountWrite(login.Context), Key: "real-company-creation-key", Spec: account.CompanySpec{Name: " Кофейня <&> ", Currency: "RUB", Timezone: "Europe/Moscow"}}
	var wg sync.WaitGroup
	results := make(chan account.CreationCommand, 2)
	errs := make(chan error, 2)
	for _, service := range []creationOperations{a, b} {
		wg.Go(func() { cmd, err := service.BeginCompanyCreation(t.Context(), input); results <- cmd; errs <- err })
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var command account.CreationCommand
	for got := range results {
		if command.ID == uuid.Nil {
			command = got
		} else if got.ID != command.ID || got.CompanyID != command.CompanyID || got.Spec != command.Spec || got.RequestSHA256 != command.RequestSHA256 || !got.CreatedAt.Equal(command.CreatedAt) {
			t.Fatal("replica replay changed command", got, command)
		}
	}
	changed := input
	changed.Spec.Name = "Other"
	if _, err := b.BeginCompanyCreation(t.Context(), changed); !errors.Is(err, account.ErrIdempotencyConflict) {
		t.Fatal(err)
	}
	if _, err := a.CompleteCompanyCreation(t.Context(), account.CompleteCompanyCreationInput{Context: input.Context, CommandID: command.ID}); !errors.Is(err, account.ErrCreationInProgress) {
		t.Fatal(err)
	}
	receipt := account.CreationReceipt{CommandID: command.ID, CompanyID: command.CompanyID, AccountID: uuid.Must(uuid.NewV7()), Spec: command.Spec, RequestSHA256: command.RequestSHA256, CommittedAt: time.Now().UTC().Truncate(time.Microsecond)}
	core.read = func(ctx context.Context, id uuid.UUID) (account.CreationReceipt, error) {
		if id != command.ID {
			return account.CreationReceipt{}, account.ErrNotFound
		}
		// NOWAIT proves the browser transaction released its user lock before RPC.
		tx, err := poolB.BeginTxx(ctx, nil)
		if err != nil {
			return account.CreationReceipt{}, err
		}
		defer func() { _ = tx.Rollback() }()
		if _, err := tx.ExecContext(ctx, "SELECT id FROM users WHERE id=$1 FOR UPDATE NOWAIT", login.User.ID); err != nil {
			return account.CreationReceipt{}, err
		}
		if err := tx.Commit(); err != nil {
			return account.CreationReceipt{}, err
		}
		return receipt, nil
	}
	// A fresh service/pool reads the PREPARED command and completes after restart.
	restarted := newCreationAccounts(t, cfg, openDatabase(t, cfg), core)
	finished, err := restarted.CompleteCompanyCreation(t.Context(), account.CompleteCompanyCreationInput{Context: input.Context, CommandID: command.ID})
	if err != nil || finished.State != "COMPLETED" || finished.CompanyID != command.CompanyID {
		t.Fatal(finished, err)
	}
	if _, err := b.CompleteCompanyCreation(t.Context(), account.CompleteCompanyCreationInput{Context: input.Context, CommandID: command.ID}); err != nil {
		t.Fatal(err)
	}
	page, err := accounts.ListMemberships(t.Context(), login.Context.CookieToken, 10, "")
	if err != nil || len(page.Items) != 1 || page.Items[0].CompanyID != command.CompanyID || page.Items[0].Role != "owner" {
		t.Fatal(page, err)
	}
	var commands, members int
	if err := db.QueryRow(t.Context(), "SELECT count(*) FROM company_creation_commands").Scan(&commands); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(t.Context(), "SELECT count(*) FROM memberships").Scan(&members); err != nil {
		t.Fatal(err)
	}
	if commands != 1 || members != 1 {
		t.Fatal(commands, members)
	}
	// Revocation and browser retry cannot restore membership from completed replay.
	execAccountFixture(t, db, "DELETE FROM memberships WHERE company_id=$1", command.CompanyID)
	if _, err := restarted.CompleteCompanyCreation(t.Context(), account.CompleteCompanyCreationInput{Context: input.Context, CommandID: command.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := accounts.AuthorizeCompany(t.Context(), account.AuthorizeCompanyInput{CookieToken: login.Context.CookieToken, CompanyID: command.CompanyID, Permission: "read"}); !errors.Is(err, account.ErrNotFound) {
		t.Fatal("revoked access restored", err)
	}
}

func TestCompanyCreationServiceDisablesInitiatorDuringReceiptRead(t *testing.T) {
	cfg, db, accounts := accountFixture(t, true)
	challenge, browser := registerAccount(t, accounts, "alice@example.test")
	if err := accounts.VerifyEmail(t.Context(), account.VerifyEmailInput{Context: accountWrite(browser), ChallengeID: challenge.ID, Code: "000000"}); err != nil {
		t.Fatal(err)
	}
	login := loginAccount(t, accounts, browser, "alice@example.test")
	var receipt account.CreationReceipt
	core := creationCoreFixture{read: func(context.Context, uuid.UUID) (account.CreationReceipt, error) {
		execAccountFixture(t, db, "UPDATE users SET state='disabled' WHERE id=$1", login.User.ID)
		return receipt, nil
	}}
	service := newCreationAccounts(t, cfg, openDatabase(t, cfg), core)
	command, err := service.BeginCompanyCreation(t.Context(), account.BeginCompanyCreationInput{Context: accountWrite(login.Context), Key: "disable-while-reading-key", Spec: account.CompanySpec{Name: "Company", Currency: "RUB", Timezone: "Europe/Moscow"}})
	if err != nil {
		t.Fatal(err)
	}
	receipt = account.CreationReceipt{CommandID: command.ID, CompanyID: command.CompanyID, AccountID: uuid.Must(uuid.NewV7()), Spec: command.Spec, RequestSHA256: command.RequestSHA256, CommittedAt: time.Now().UTC()}
	if _, err := service.CompleteCompanyCreation(t.Context(), account.CompleteCompanyCreationInput{Context: accountWrite(login.Context), CommandID: command.ID}); !errors.Is(err, account.ErrForbidden) {
		t.Fatal(err)
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
}
