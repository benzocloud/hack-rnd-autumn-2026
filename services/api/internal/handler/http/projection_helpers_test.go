package httpapi

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	authv1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/auth/v1"
	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

func TestProjectionHelperEdges(t *testing.T) {
	if optionalText("") != nil || optionalText("x") != "x" {
		t.Fatal("optionalText")
	}
	if validCursor(string(make([]byte, 1025))) || !validCursor("") || validCursor("a\nb") {
		t.Fatal("validCursor")
	}
	if roleName(authv1.CompanyRole_COMPANY_ROLE_UNSPECIFIED) != "" || roleName(authv1.CompanyRole_COMPANY_ROLE_OWNER) != "owner" || roleName(authv1.CompanyRole_COMPANY_ROLE_FINANCIER) != "financier" || roleName(authv1.CompanyRole(99)) != "" {
		t.Fatal("roleName")
	}
	for status, want := range map[corev1.ForecastStatus]string{
		corev1.ForecastStatus_FORECAST_STATUS_UNSPECIFIED: "",
		corev1.ForecastStatus_FORECAST_STATUS_QUEUED:      "queued",
		corev1.ForecastStatus_FORECAST_STATUS_RUNNING:     "running",
		corev1.ForecastStatus_FORECAST_STATUS_SUCCEEDED:   "succeeded",
		corev1.ForecastStatus_FORECAST_STATUS_FAILED:      "failed",
		corev1.ForecastStatus(99):                         "",
	} {
		if got := statusName(status); got != want {
			t.Fatalf("statusName %v: %q", status, got)
		}
	}
	for severity, want := range map[corev1.IssueSeverity]string{
		corev1.IssueSeverity_ISSUE_SEVERITY_UNSPECIFIED: "",
		corev1.IssueSeverity_ISSUE_SEVERITY_WARNING:     "warning",
		corev1.IssueSeverity_ISSUE_SEVERITY_BLOCKING:    "blocking",
		corev1.IssueSeverity(99):                        "",
	} {
		if got := issueSeverity(severity); got != want {
			t.Fatalf("issueSeverity %v: %q", severity, got)
		}
	}
	if dateSequence("bad", "2026-01-02", "2026-01-31", 30) || dateSequence("2026-01-01", "2026-01-02", "2026-01-31", 29) || !dateSequence("2026-01-01", "2026-01-02", "2026-01-31", 30) {
		t.Fatal("dateSequence")
	}
	if unsignedDecimal("") || unsignedDecimal("12a") || !unsignedDecimal("0") {
		t.Fatal("unsignedDecimal")
	}
}

func TestValidAccountAndAccountDTOEdges(t *testing.T) {
	company, id := "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	base := &corev1.Account{Id: id, CompanyId: company, Name: "Main", Currency: "RUB", DataVersion: 1}
	if !validAccount(base, company) || validAccount(nil, company) || validAccount(base, "other") {
		t.Fatal("base account")
	}
	badMoney := &corev1.Account{Id: id, CompanyId: company, Name: "Main", Currency: "RUB", DataVersion: 1, Balance: &corev1.DatedBalance{Amount: &corev1.Money{AmountMinor: 1, Currency: "USD"}, AsOf: "2026-01-01", Source: corev1.BalanceSource_BALANCE_SOURCE_MANUAL}}
	if validAccount(badMoney, company) {
		t.Fatal("foreign currency balance")
	}
	sourceCompany := "SRC"
	imported := &corev1.Account{Id: id, CompanyId: company, Name: "Main", Currency: "RUB", DataVersion: 2, Balance: &corev1.DatedBalance{Amount: &corev1.Money{AmountMinor: 10, Currency: "RUB"}, AsOf: "2026-01-01", Source: corev1.BalanceSource_BALANCE_SOURCE_STATEMENT_EOD}, TransactionSource: &corev1.FactualSourceBinding{Mode: corev1.FactualSourceMode_FACTUAL_SOURCE_MODE_IMPORTED, SourceSystem: "dataset_v1", SourceCompanyId: &sourceCompany}}
	if !validAccount(imported, company) {
		t.Fatal("imported account")
	}
	dto := accountDTO(imported)
	if dto["balance_source"] != "statement_eod" || dto["transaction_source"].(object)["mode"] != "external" {
		t.Fatal(dto)
	}
	manualBad := &corev1.Account{Id: id, CompanyId: company, Name: "Main", Currency: "RUB", DataVersion: 1, TransactionSource: &corev1.FactualSourceBinding{Mode: corev1.FactualSourceMode_FACTUAL_SOURCE_MODE_MANUAL, SourceSystem: "other"}}
	if validAccount(manualBad, company) {
		t.Fatal("manual wrong system")
	}
	if validAccount(&corev1.Account{Id: id, CompanyId: company, Name: "Main", Currency: "RUB", DataVersion: 1, TransactionSource: &corev1.FactualSourceBinding{Mode: corev1.FactualSourceMode_FACTUAL_SOURCE_MODE_UNSPECIFIED}}, company) {
		t.Fatal("unspecified source")
	}
	if validAccount(&corev1.Account{Id: id, CompanyId: company, Name: "Main", Currency: "RUB", DataVersion: 1, TransactionSource: &corev1.FactualSourceBinding{Mode: corev1.FactualSourceMode(99)}}, company) {
		t.Fatal("unknown source")
	}
}

func TestImportPreviewAndTransactionProjectionRejects(t *testing.T) {
	account := "33333333-3333-4333-8333-333333333333"
	preview := &corev1.ImportPreview{
		AccountId: account, SourceCompanyId: "SRC", SourceSystem: "dataset_v1", Currency: "RUB",
		PeriodStart: "2026-01-01", PeriodEnd: "2026-01-31", BaseDataVersion: 1, PreviewVersion: 1, RowCount: 1,
		RecordKinds:         []corev1.ImportRecordKind{corev1.ImportRecordKind_IMPORT_RECORD_KIND_TRANSACTIONS, corev1.ImportRecordKind_IMPORT_RECORD_KIND_BALANCE_OBSERVATIONS},
		BalanceObservations: []*corev1.BalanceObservation{{Date: "2026-01-15", Balance: &corev1.Money{AmountMinor: 1, Currency: "RUB"}, Source: corev1.BalanceSource_BALANCE_SOURCE_MANUAL}},
	}
	if _, err := importPreviewDTO(preview, account); err == nil {
		t.Fatal("non-statement balance source")
	}
	preview.BalanceObservations[0].Source = corev1.BalanceSource_BALANCE_SOURCE_STATEMENT_EOD
	preview.BalanceObservations = append(preview.BalanceObservations, &corev1.BalanceObservation{Date: "2026-01-15", Balance: &corev1.Money{AmountMinor: 2, Currency: "RUB"}, Source: corev1.BalanceSource_BALANCE_SOURCE_STATEMENT_EOD})
	if _, err := importPreviewDTO(preview, account); err == nil {
		t.Fatal("duplicate observation date")
	}
	if _, err := transactionDTO(nil, account); err == nil {
		t.Fatal("nil transaction")
	}
	now := timestamppb.New(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	manual := &corev1.Transaction{
		Id: "44444444-4444-4444-8444-444444444444", AccountId: account, OperationDate: "2026-01-01",
		Direction: corev1.Direction_DIRECTION_INFLOW, Amount: &corev1.Money{AmountMinor: 1, Currency: "RUB"},
		CounterpartyName: "A", PaymentPurpose: "B", Version: 1, CreatedAt: now,
		Source: &corev1.RecordSource{Kind: corev1.RecordSourceKind_RECORD_SOURCE_KIND_MANUAL, SourceSystem: "manual_v1"},
	}
	if _, err := transactionDTO(manual, account); err != nil {
		t.Fatal(err)
	}
	manual.Source.SourceSystem = "other"
	if _, err := transactionDTO(manual, account); err == nil {
		t.Fatal("manual wrong system")
	}
}
