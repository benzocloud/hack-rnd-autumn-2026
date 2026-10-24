//go:build integration

package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	appPostgres "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/app/postgres"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/company"
	companySQL "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres/company"
	companyService "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/company"
)

func bootstrapCommand(t *testing.T) companyService.BootstrapCommand {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	spec := domain.Spec{Name: "Кофейня <ИП>", INN: "001234567890", Currency: "RUB", Timezone: "Europe/Moscow"}
	hash, err := domain.Fingerprint(spec)
	if err != nil {
		t.Fatal(err)
	}
	return companyService.BootstrapCommand{CommandID: uuid.New(), CompanyID: id, Spec: spec, RequestSHA256: hash}
}
func authorizeCommand(cmd companyService.BootstrapCommand) func(context.Context, uuid.UUID, uuid.UUID, string) (companyService.Authorization, error) {
	return func(_ context.Context, id, companyID uuid.UUID, hash string) (companyService.Authorization, error) {
		if id != cmd.CommandID || companyID != cmd.CompanyID || hash != cmd.RequestSHA256 {
			return companyService.Authorization{}, companyService.ErrUnauthorized
		}
		return companyService.Authorization(cmd), nil
	}
}
func cleanupCompany(t *testing.T, app *application, id uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
		defer cancel()
		for _, query := range []string{
			"DELETE FROM core_forecast_jobs WHERE company_id=$1",
			"DELETE FROM core_history_coverage WHERE company_id=$1",
			"DELETE FROM core_financial_snapshots WHERE company_id=$1",
			"DELETE FROM core_company_bootstraps WHERE company_id=$1",
			"DELETE FROM core_company_profiles WHERE company_id=$1",
			"DELETE FROM core_command_results WHERE company_id=$1",
			"DELETE FROM core_balances WHERE company_id=$1",
			"DELETE FROM core_transactions WHERE company_id=$1",
			"DELETE FROM core_categories WHERE company_id=$1",
			"UPDATE core_accounts SET factual_kind=NULL,source_system=NULL,source_company_id=NULL WHERE company_id=$1",
			"DELETE FROM core_source_bindings WHERE company_id=$1",
			"DELETE FROM core_accounts WHERE company_id=$1",
			"DELETE FROM core_companies WHERE id=$1",
		} {
			if _, err := app.db.ExecContext(ctx, query, id); err != nil {
				t.Error(err)
			}
		}
	})
}
func companyCount(t *testing.T, app *application, id uuid.UUID, table string) int {
	t.Helper()
	queries := map[string]string{
		"company": "SELECT count(*) FROM core_companies WHERE id=$1",
		"account": "SELECT count(*) FROM core_accounts WHERE company_id=$1",
		"profile": "SELECT count(*) FROM core_company_profiles WHERE company_id=$1",
		"receipt": "SELECT count(*) FROM core_company_bootstraps WHERE company_id=$1",
	}
	query, ok := queries[table]
	if !ok {
		t.Fatal("unknown test table")
	}
	var count int
	if err := app.db.GetContext(t.Context(), &count, query, id); err != nil {
		t.Fatal(err)
	}
	return count
}
func TestCompanyBootstrapDurableReplay(t *testing.T) {
	app, cfg := testApplication(t)
	cmd := bootstrapCommand(t)
	cleanupCompany(t, app, cmd.CompanyID)
	service := newCompanies(app.db, cfg.Postgres, authorizeCommand(cmd))
	first, err := service.Bootstrap(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if first.AccountID.Version() != 7 || first.CompanyID != cmd.CompanyID || first.Spec != cmd.Spec {
		t.Fatal(first)
	}
	replay, err := service.Bootstrap(t.Context(), cmd)
	if err != nil || replay != first {
		t.Fatal(replay, err)
	}
	// A second app/connection reads the persisted receipt, not in-process cache.
	second, _ := testApplication(t)
	result, err := second.Companies.GetBootstrapResult(t.Context(), cmd.CommandID)
	if err != nil || result != first {
		t.Fatal(result, err)
	}
	profile, err := second.Companies.ReadProfile(t.Context(), cmd.CompanyID)
	if err != nil || profile.AccountID != first.AccountID || profile.Version != 1 || profile.Spec != cmd.Spec {
		t.Fatal(profile, err)
	}
	for _, table := range []string{"company", "account", "profile", "receipt"} {
		if companyCount(t, app, cmd.CompanyID, table) != 1 {
			t.Fatal("duplicate/missing", table)
		}
	}
	var version int64
	if err = app.db.GetContext(t.Context(), &version, "SELECT data_version FROM core_accounts WHERE company_id=$1", cmd.CompanyID); err != nil || version != 1 {
		t.Fatal(version, err)
	}
	if _, err = app.db.ExecContext(t.Context(), "UPDATE core_company_profiles SET name='Renamed',version=version+1 WHERE company_id=$1", cmd.CompanyID); err != nil {
		t.Fatal(err)
	}
	result, err = service.Bootstrap(t.Context(), cmd)
	if err != nil || result != first {
		t.Fatal("receipt changed after rename", result, err)
	}
	profile, err = service.ReadProfile(t.Context(), cmd.CompanyID)
	if err != nil || profile.Spec.Name != "Renamed" || profile.Version != 2 {
		t.Fatal(profile, err)
	}
}
func TestCompanyBootstrapConcurrency(t *testing.T) {
	app, cfg := testApplication(t)
	cmd := bootstrapCommand(t)
	cleanupCompany(t, app, cmd.CompanyID)
	service := newCompanies(app.db, cfg.Postgres, authorizeCommand(cmd))
	const workers = 8
	results := make([]companyService.BootstrapResult, workers)
	errs := make([]error, workers)
	var group sync.WaitGroup
	start := make(chan struct{})
	for i := range workers {
		group.Go(func() { <-start; results[i], errs[i] = service.Bootstrap(t.Context(), cmd) })
	}
	close(start)
	group.Wait()
	for i := range workers {
		if errs[i] != nil || results[i] != results[0] {
			t.Fatalf("concurrent retry %d: %+v %v", i, results[i], errs[i])
		}
	}
	for _, table := range []string{"company", "account", "profile", "receipt"} {
		if companyCount(t, app, cmd.CompanyID, table) != 1 {
			t.Fatal("duplicate/missing", table)
		}
	}
}
func TestCompanyBootstrapRejectsConflictingAuthCommand(t *testing.T) {
	app, cfg := testApplication(t)
	cmd := bootstrapCommand(t)
	cleanupCompany(t, app, cmd.CompanyID)
	service := newCompanies(app.db, cfg.Postgres, authorizeCommand(cmd))
	if _, err := service.Bootstrap(t.Context(), cmd); err != nil {
		t.Fatal(err)
	}
	changed := cmd
	changed.Spec.Name = "Other"
	changed.RequestSHA256, _ = domain.Fingerprint(changed.Spec)
	otherService := newCompanies(app.db, cfg.Postgres, authorizeCommand(changed))
	if result, err := otherService.Bootstrap(t.Context(), changed); !errors.Is(err, companyService.ErrConflict) || result != (companyService.BootstrapResult{}) {
		t.Fatal(result, err)
	}
	changed = cmd
	changed.CommandID = uuid.New()
	otherService = newCompanies(app.db, cfg.Postgres, authorizeCommand(changed))
	if result, err := otherService.Bootstrap(t.Context(), changed); !errors.Is(err, companyService.ErrConflict) || result != (companyService.BootstrapResult{}) {
		t.Fatal(result, err)
	}
	if companyCount(t, app, cmd.CompanyID, "receipt") != 1 || companyCount(t, app, cmd.CompanyID, "account") != 1 {
		t.Fatal("conflicting command modified state")
	}
}
func TestCompanyBootstrapAtomicRollback(t *testing.T) {
	app, cfg := testApplication(t)
	for _, mode := range []string{"existing company", "commit constraint"} {
		t.Run(mode, func(t *testing.T) {
			cmd := bootstrapCommand(t)
			cleanupCompany(t, app, cmd.CompanyID)
			if mode == "existing company" {
				if _, err := app.db.ExecContext(t.Context(), "INSERT INTO core_companies(id) VALUES($1)", cmd.CompanyID); err != nil {
					t.Fatal(err)
				}
			}
			service := companyService.New(func(ctx context.Context, w companyService.Work) error {
				err := appPostgres.WithinTx(ctx, app.db, cfg.Postgres.QueryTimeout, func(ctx context.Context, tx *sqlx.Tx) error {
					if err := w(ctx, companySQL.New(tx)); err != nil {
						return err
					}
					// Simulate a deferred integrity failure at COMMIT, after Work returned a result.
					if mode == "commit constraint" {
						_, err := tx.ExecContext(ctx, "DELETE FROM core_company_profiles WHERE company_id=$1", cmd.CompanyID)
						return err
					}
					return nil
				})
				return companyError(err)
			}, authorizeCommand(cmd))
			result, err := service.Bootstrap(t.Context(), cmd)
			if !errors.Is(err, companyService.ErrConflict) || result != (companyService.BootstrapResult{}) {
				t.Fatal("failed transaction published success", result, err)
			}
			for _, table := range []string{"account", "profile", "receipt"} {
				if companyCount(t, app, cmd.CompanyID, table) != 0 {
					t.Fatal("partial write", table)
				}
			}
			expected := 0
			if mode == "existing company" {
				expected = 1
			}
			if companyCount(t, app, cmd.CompanyID, "company") != expected {
				t.Fatal("company write was not atomic")
			}
		})
	}
}
func TestCompanyReadsMissingAndFailClosed(t *testing.T) {
	app, _ := testApplication(t)
	cmd := bootstrapCommand(t)
	if result, err := app.Companies.Bootstrap(t.Context(), cmd); !errors.Is(err, companyService.ErrUnavailable) || result != (companyService.BootstrapResult{}) {
		t.Fatal("nil authorizer permitted bootstrap", result, err)
	}
	if companyCount(t, app, cmd.CompanyID, "company") != 0 {
		t.Fatal("unauthorized company created")
	}
	if _, err := app.Companies.GetBootstrapResult(t.Context(), cmd.CommandID); !errors.Is(err, companyService.ErrNotFound) {
		t.Fatal(err)
	}
	// A legacy ID-only company must not acquire an invented profile.
	if _, err := app.db.ExecContext(t.Context(), "INSERT INTO core_companies(id) VALUES($1)", cmd.CompanyID); err != nil {
		t.Fatal(err)
	}
	cleanupCompany(t, app, cmd.CompanyID)
	if _, err := app.Companies.ReadProfile(t.Context(), cmd.CompanyID); !errors.Is(err, companyService.ErrNotFound) {
		t.Fatal(err)
	}
}
