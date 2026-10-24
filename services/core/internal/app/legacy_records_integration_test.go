//go:build integration

package app

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/imports"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/imports"
	ledgerService "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
)

func TestPersistedCommitReceiptReplaysWithoutRewrite(t *testing.T) {
	application, cfg := testApplication(t)
	fixture := commitReceiptFixture(t)
	scope := ledger.Scope{
		CompanyID: uuid.MustParse("019a0c65-9800-7000-8000-000000000001"),
		AccountID: uuid.MustParse("019a0c65-9800-7000-8000-000000000002"),
	}
	importID := uuid.MustParse("019a0c65-9800-7000-8000-000000000003")
	deleteCompanyRows(t, application, scope.CompanyID)
	cleanupCompany(t, application, scope.CompanyID)
	if _, err := application.db.ExecContext(t.Context(), "INSERT INTO core_companies(id) VALUES($1)", scope.CompanyID); err != nil {
		t.Fatal(err)
	}
	if _, err := application.db.ExecContext(t.Context(), "INSERT INTO core_accounts(id,company_id,currency,data_version) VALUES($1,$2,'RUB',1)", scope.AccountID, scope.CompanyID); err != nil {
		t.Fatal(err)
	}
	digest, err := hex.DecodeString(fixture.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.db.ExecContext(t.Context(), "INSERT INTO core_command_results(actor_id,company_id,account_id,operation,resource_id,idempotency_key,digest,result) VALUES('fixture-owner',$1,$2,'commit_import_v1',$3,'commit-key-123',$4,NULL)", scope.CompanyID, scope.AccountID, importID.String(), digest); err != nil {
		t.Fatal(err)
	}
	service, err := newImports(application.db, cfg.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	cmd := imports.CommitCommand{Scope: scope, ActorID: "fixture-owner", Key: "commit-key-123", ID: importID, ExpectedVersion: 2, ExpectedDataVersion: 1, AcknowledgeWarnings: true, ViewDate: "2026-10-25"}
	if _, err := service.Commit(t.Context(), cmd); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("null receipt applied", err)
	}
	if _, err := application.db.ExecContext(t.Context(), "UPDATE core_command_results SET result=$1::jsonb WHERE company_id=$2 AND idempotency_key='commit-key-123'", fixture.Receipt, scope.CompanyID); err != nil {
		t.Fatal(err)
	}
	result, err := service.Commit(t.Context(), cmd)
	if err != nil || result.Import.Status != "applied" || result.Import.AppliedDataVersion != 2 {
		t.Fatal(result, err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(fixture.Receipt), &payload); err != nil {
		t.Fatal(err)
	}
	payload["Import"].(map[string]any)["FutureField"] = "ignored"
	extra, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.db.ExecContext(t.Context(), "UPDATE core_command_results SET result=$1::jsonb WHERE company_id=$2 AND idempotency_key='commit-key-123'", extra, scope.CompanyID); err != nil {
		t.Fatal(err)
	}
	result, err = service.Commit(t.Context(), cmd)
	if err != nil || result.Import.AppliedDataVersion != 2 {
		t.Fatal("unknown receipt field broke replay", result, err)
	}
	assertCount(t, application, "SELECT count(*) FROM core_transactions WHERE company_id=$1", scope, 0)
}

func TestLegacyRowsStayReadable(t *testing.T) {
	application, _ := testApplication(t)
	scope := newScope(t, application)
	categoryID := uuid.MustParse("019a0c65-9800-7000-8000-000000000004")
	factID := uuid.MustParse("019a0c65-9800-7000-8000-000000000005")
	if _, err := application.db.ExecContext(t.Context(), "INSERT INTO core_categories(company_id,id,name) VALUES($1,$2,'Legacy')", scope.CompanyID, categoryID); err != nil {
		t.Fatal(err)
	}
	if _, err := application.db.ExecContext(t.Context(), "INSERT INTO core_transactions(company_id,account_id,id,operation_date,direction,amount_minor,currency,source_kind,counterparty_inn,counterparty_name,purpose,version,created_data_version,created_at) VALUES($1,$2,$3,'2026-09-01','inflow',100,'RUB','manual','','name','purpose',1,1,NULL)", scope.CompanyID, scope.AccountID, factID); err != nil {
		t.Fatal(err)
	}
	reader, ok := application.Ledger.(interface {
		ListTransactions(context.Context, ledgerService.ListTransactionsCommand) (ledgerService.ListTransactionsResult, error)
		ListCategories(context.Context, ledgerService.ListCategoriesCommand) (ledgerService.ListCategoriesResult, error)
	})
	if !ok {
		t.Fatal("ledger does not expose catalogue reads")
	}
	facts, err := reader.ListTransactions(t.Context(), ledgerService.ListTransactionsCommand{Scope: scope, ActorID: "fixture-owner", Limit: 10})
	if err != nil || len(facts.Items) != 1 || facts.Items[0].CreatedAt != nil || facts.Items[0].ImportID != uuid.Nil || facts.Items[0].Transaction.Data().ID != factID {
		t.Fatal(facts, err)
	}
	categories, err := reader.ListCategories(t.Context(), ledgerService.ListCategoriesCommand{Scope: scope, ActorID: "fixture-owner", Limit: 10})
	if err != nil || len(categories.Items) != 1 || categories.Items[0].ID != categoryID || categories.Items[0].Direction != "" || categories.Items[0].SemanticKind != "other" || categories.Items[0].CatalogueCode != "" {
		t.Fatal(categories, err)
	}
}

func commitReceiptFixture(t *testing.T) struct{ Body, Digest, Receipt string } {
	t.Helper()
	encoded, err := os.ReadFile("../service/imports/testdata/commit_receipt_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct{ Body, Digest, Receipt string }
	if err := json.Unmarshal(encoded, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func deleteCompanyRows(t *testing.T, application *application, companyID uuid.UUID) {
	t.Helper()
	for _, query := range []string{
		"DELETE FROM core_forecast_jobs WHERE company_id=$1",
		"DELETE FROM core_history_coverage WHERE company_id=$1",
		"DELETE FROM core_command_results WHERE company_id=$1",
		"DELETE FROM core_balances WHERE company_id=$1",
		"DELETE FROM core_transactions WHERE company_id=$1",
		"DELETE FROM core_categories WHERE company_id=$1",
		"DELETE FROM core_accounts WHERE company_id=$1",
		"DELETE FROM core_companies WHERE id=$1",
	} {
		if _, err := application.db.ExecContext(t.Context(), query, companyID); err != nil {
			t.Fatal(err)
		}
	}
}
