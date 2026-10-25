//go:build integration

package app

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/imports"
	ledgerService "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
)

func TestImportedCategoriesAvailableBeforeForecast(t *testing.T) {
	application, cfg := testApplication(t)
	scope := newScope(t, application)
	body := "company_id;operation_id;operation_date;amount;direction;counterparty_inn;counterparty_name;payment_purpose;balance_eod\n" +
		"C001;TAX;2026-10-24;10.00;Списание;0012345678;test;Единый налоговый платеж за сотрудников;1000.00\n" +
		"C001;PAYROLL;2026-10-24;10.00;Списание;0012345678;test;ЗАРАБОТНАЯ ПЛАТА;1000.00\n" +
		"C001;FEE;2026-10-24;10.00;Списание;0012345678;test;Комиссия банка;1000.00\n" +
		"C001;SALES;2026-10-24;10.00;Поступление;0012345678;test;Оплата по договору;1000.00\n" +
		"C001;OWN;2026-10-24;10.00;Поступление;0012345678;test;Перевод собственных средств;1000.00\n" +
		"C001;LOAN;2026-10-24;10.00;Поступление;0012345678;test;Получение кредита;1000.00\n" +
		"C001;UNKNOWN;2026-10-24;10.00;Списание;0012345678;test;Неизвестное движение;1000.00\n" +
		"C001;REFUND;2026-10-24;10.00;Поступление;0012345678;test;Возврат оплаты;1000.00\n"
	importer, accepted, preview := stageImport(t, application, cfg, scope, body)
	if _, err := importer.Commit(t.Context(), imports.CommitCommand{Scope: scope, ActorID: "actor", Key: uuid.NewString(), ID: accepted.ID, ExpectedVersion: preview.Version, ExpectedDataVersion: 1, AcknowledgeWarnings: true, ViewDate: "2026-10-25"}); err != nil {
		t.Fatal(err)
	}
	reader := application.Ledger.(interface {
		ListTransactions(context.Context, ledgerService.ListTransactionsCommand) (ledgerService.ListTransactionsResult, error)
		UpdateTransactionCategory(context.Context, ledgerService.UpdateTransactionCategoryCommand) (ledgerService.UpdateTransactionCategoryResult, error)
	})
	want := map[string]string{"TAX": "tax", "PAYROLL": "operating", "FEE": "operating", "SALES": "sales", "OWN": "own_transfer", "LOAN": "financing", "UNKNOWN": "other", "REFUND": "other"}
	ids := map[string]uuid.UUID{}
	for _, code := range []string{"tax", "operating", "sales", "own_transfer", "financing", "other"} {
		var id uuid.UUID
		if err := application.db.GetContext(t.Context(), &id, "SELECT id FROM core_categories WHERE company_id=$1 AND catalogue_code=$2", scope.CompanyID, code); err != nil {
			t.Fatal(err)
		}
		ids[code] = id
	}
	list := func(category uuid.UUID) []ledger.StoredTransaction {
		t.Helper()
		result, err := reader.ListTransactions(t.Context(), ledgerService.ListTransactionsCommand{Scope: scope, ActorID: "actor", Limit: 20, CategoryID: category})
		if err != nil {
			t.Fatal(err)
		}
		return result.Items
	}
	items := list(uuid.Nil)
	if len(items) != len(want) {
		t.Fatal("missing imported transactions", len(items))
	}
	var tax ledger.Transaction
	for _, item := range items {
		d := item.Transaction.Data()
		if d.Category.ID != ids[want[d.Source.ExternalID]] || item.Transaction.Version() != 1 {
			t.Fatal("incorrect automatic category or changed version", d.Source.ExternalID)
		}
		if d.Source.ExternalID == "TAX" {
			tax = item.Transaction
		}
	}
	assertCount(t, application, "SELECT count(*) FROM core_transactions WHERE company_id=$1 AND category_id IS NOT NULL", scope, 0)
	assertCount(t, application, "SELECT count(*) FROM core_forecast_jobs WHERE company_id=$1", scope, 0)
	assertRevision(t, application, scope, 2)
	if len(list(ids["operating"])) != 2 {
		t.Fatal("automatic category filter missing rows")
	}
	if _, err := reader.UpdateTransactionCategory(t.Context(), ledgerService.UpdateTransactionCategoryCommand{Scope: scope, ActorID: "actor", TransactionID: tax.Data().ID, ExpectedVersion: 1, CategoryID: ids["operating"]}); err != nil {
		t.Fatal(err)
	}
	if len(list(ids["tax"])) != 0 || len(list(ids["operating"])) != 3 {
		t.Fatal("manual override did not take precedence")
	}
	if _, err := reader.UpdateTransactionCategory(t.Context(), ledgerService.UpdateTransactionCategoryCommand{Scope: scope, ActorID: "actor", TransactionID: tax.Data().ID, ExpectedVersion: 2}); err != nil {
		t.Fatal(err)
	}
	if len(list(ids["tax"])) != 1 || len(list(ids["operating"])) != 2 {
		t.Fatal("clearing manual override did not restore automatic category")
	}
}
