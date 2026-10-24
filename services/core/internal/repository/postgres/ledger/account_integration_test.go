//go:build integration

package ledger_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"

	appPostgres "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/app/postgres"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres/ledger"
)

func testDatabase(t *testing.T) *sqlx.DB {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Postgres.Host != "127.0.0.1" || cfg.Postgres.Database != "saldo_core_test" {
		t.Fatal("integration tests require the dedicated loopback saldo_core_test database")
	}
	db, err := appPostgres.Open(t.Context(), cfg.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var marker string
	if err := db.GetContext(ctx, &marker, "SELECT coalesce(current_setting('saldo.test_database', true), '')"); err != nil {
		t.Fatal(err)
	}
	if marker != "saldo-core-integration" {
		t.Fatal("refusing writes: dedicated test database marker is missing")
	}
	return db
}

func testTransaction(t *testing.T, db *sqlx.DB) (context.Context, *sqlx.Tx) {
	t.Helper()
	// testing cancels t.Context before Cleanup; keep rollback usable until our cancel.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
	t.Cleanup(cancel)
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			t.Error(err)
		}
	})
	return ctx, tx
}

func testScope(t *testing.T) domain.Scope {
	t.Helper()
	companyID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	accountID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return domain.Scope{CompanyID: companyID, AccountID: accountID}
}

func TestSaveAccountMissingRow(t *testing.T) {
	db := testDatabase(t)
	ctx, tx := testTransaction(t, db)
	account, err := domain.NewBankAccount(testScope(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.SaveAccount(ctx, tx, account); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing account must not report a successful save: %v", err)
	}
}

func TestAccountSourceConstraint(t *testing.T) {
	db := testDatabase(t)
	for _, tc := range []struct {
		name                    string
		kind                    any
		system, externalCompany any
		valid                   bool
	}{
		{name: "no authority", valid: true},
		{name: "manual authority", kind: "manual", valid: true},
		{name: "imported authority", kind: "imported", system: "dataset_v1", externalCompany: "C001", valid: true},
		{name: "null authority with namespace", system: "dataset_v1", externalCompany: "C001"},
		{name: "manual authority with namespace", kind: "manual", system: "dataset_v1", externalCompany: "C001"},
		{name: "imported authority without namespace", kind: "imported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, tx := testTransaction(t, db)
			scope := testScope(t)
			if _, err := tx.ExecContext(ctx, "INSERT INTO core_companies(id) VALUES($1)", scope.CompanyID); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO core_accounts(id,company_id,currency,data_version) VALUES($1,$2,'RUB',1)", scope.AccountID, scope.CompanyID); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO core_source_bindings(company_id,account_id,source_system,source_company_id) VALUES($1,$2,'dataset_v1','C001')", scope.CompanyID, scope.AccountID); err != nil {
				t.Fatal(err)
			}
			_, err := tx.ExecContext(ctx, "UPDATE core_accounts SET factual_kind=$3,source_system=$4,source_company_id=$5 WHERE company_id=$1 AND id=$2", scope.CompanyID, scope.AccountID, tc.kind, tc.system, tc.externalCompany)
			if !tc.valid {
				var pgErr *pgconn.PgError
				if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
					t.Fatalf("invalid source state must fail CHECK: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			account, err := ledger.LoadAccount(ctx, tx, scope)
			if err != nil {
				t.Fatal(err)
			}
			if account.Scope() != scope || account.DataVersion() != 1 {
				t.Fatal("UUID scope or stored version changed during roundtrip")
			}
			if err := ledger.SaveAccount(ctx, tx, account); err != nil {
				t.Fatal(err)
			}
		})
	}
}
