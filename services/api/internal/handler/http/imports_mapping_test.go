package httpapi

import (
	"testing"

	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

func TestImportStatusName(t *testing.T) {
	for _, test := range []struct {
		status corev1.ImportStatus
		want   string
	}{
		{corev1.ImportStatus_IMPORT_STATUS_UNSPECIFIED, ""},
		{corev1.ImportStatus_IMPORT_STATUS_QUEUED, "queued"},
		{corev1.ImportStatus_IMPORT_STATUS_PROCESSING, "processing"},
		{corev1.ImportStatus_IMPORT_STATUS_PREVIEW_READY, "preview_ready"},
		{corev1.ImportStatus_IMPORT_STATUS_APPLIED, "applied"},
		{corev1.ImportStatus_IMPORT_STATUS_FAILED, "failed"},
		{corev1.ImportStatus(99), ""},
	} {
		if got := importStatusName(test.status); got != test.want {
			t.Fatalf("%v: got %q want %q", test.status, got, test.want)
		}
	}
}

func TestImportKindsDTORejectsBadSets(t *testing.T) {
	if _, err := importKindsDTO(nil); err == nil {
		t.Fatal("empty kinds")
	}
	if _, err := importKindsDTO([]corev1.ImportRecordKind{corev1.ImportRecordKind_IMPORT_RECORD_KIND_UNSPECIFIED}); err == nil {
		t.Fatal("unspecified kind")
	}
	if _, err := importKindsDTO([]corev1.ImportRecordKind{corev1.ImportRecordKind(99)}); err == nil {
		t.Fatal("unknown kind")
	}
	if _, err := importKindsDTO([]corev1.ImportRecordKind{
		corev1.ImportRecordKind_IMPORT_RECORD_KIND_TRANSACTIONS,
		corev1.ImportRecordKind_IMPORT_RECORD_KIND_TRANSACTIONS,
	}); err == nil {
		t.Fatal("duplicate kind")
	}
	got, err := importKindsDTO([]corev1.ImportRecordKind{
		corev1.ImportRecordKind_IMPORT_RECORD_KIND_TRANSACTIONS,
		corev1.ImportRecordKind_IMPORT_RECORD_KIND_BALANCE_OBSERVATIONS,
		corev1.ImportRecordKind_IMPORT_RECORD_KIND_PLANNED_PAYMENTS,
	})
	if err != nil || len(got) != 3 || got[2] != "planned_payments" {
		t.Fatal(got, err)
	}
}

func TestImportBindingAndCoverageDTO(t *testing.T) {
	if value, err := importBindingDTO(nil); value != nil || err != nil {
		t.Fatal(value, err)
	}
	if _, err := importBindingDTO(&corev1.FactualSourceBinding{Mode: corev1.FactualSourceMode_FACTUAL_SOURCE_MODE_UNSPECIFIED}); err == nil {
		t.Fatal("unspecified mode")
	}
	if _, err := importBindingDTO(&corev1.FactualSourceBinding{Mode: corev1.FactualSourceMode(99)}); err == nil {
		t.Fatal("unknown mode")
	}
	if _, err := importBindingDTO(&corev1.FactualSourceBinding{Mode: corev1.FactualSourceMode_FACTUAL_SOURCE_MODE_MANUAL, SourceSystem: "other"}); err == nil {
		t.Fatal("manual wrong system")
	}
	manual, err := importBindingDTO(&corev1.FactualSourceBinding{Mode: corev1.FactualSourceMode_FACTUAL_SOURCE_MODE_MANUAL, SourceSystem: "manual_v1"})
	if err != nil || manual.(object)["mode"] != "manual" {
		t.Fatal(manual, err)
	}
	company := "C001"
	if _, err := importBindingDTO(&corev1.FactualSourceBinding{Mode: corev1.FactualSourceMode_FACTUAL_SOURCE_MODE_IMPORTED, SourceSystem: "dataset_v1"}); err == nil {
		t.Fatal("imported without company")
	}
	external, err := importBindingDTO(&corev1.FactualSourceBinding{Mode: corev1.FactualSourceMode_FACTUAL_SOURCE_MODE_IMPORTED, SourceSystem: "dataset_v1", SourceCompanyId: &company})
	if err != nil || external.(object)["mode"] != "external" {
		t.Fatal(external, err)
	}

	if value, err := importCoverageDTO(nil); value != nil || err != nil {
		t.Fatal(value, err)
	}
	if _, err := importCoverageDTO(&corev1.HistoryCoverage{}); err == nil {
		t.Fatal("empty coverage")
	}
	if _, err := importCoverageDTO(&corev1.HistoryCoverage{Kind: corev1.HistoryCoverageKind_HISTORY_COVERAGE_KIND_UNSPECIFIED, FromDate: "2026-01-01", ThroughDate: "2026-01-31", Reference: "ref", DataVersion: 1}); err == nil {
		t.Fatal("unspecified kind")
	}
	if _, err := importCoverageDTO(&corev1.HistoryCoverage{Kind: corev1.HistoryCoverageKind(99), FromDate: "2026-01-01", ThroughDate: "2026-01-31", Reference: "ref", DataVersion: 1}); err == nil {
		t.Fatal("unknown kind")
	}
	certified, err := importCoverageDTO(&corev1.HistoryCoverage{Kind: corev1.HistoryCoverageKind_HISTORY_COVERAGE_KIND_CERTIFIED, FromDate: "2026-01-01", ThroughDate: "2026-01-31", Reference: "ref", DataVersion: 1})
	if err != nil || certified.(object)["kind"] != "certified" {
		t.Fatal(certified, err)
	}
	if _, err := importCoverageDTO(&corev1.HistoryCoverage{Kind: corev1.HistoryCoverageKind_HISTORY_COVERAGE_KIND_DATASET_FIXTURE_ASSUMPTION, FromDate: "2026-01-01", ThroughDate: "2026-01-31", Reference: "ref", DataVersion: 1}); err == nil {
		t.Fatal("fixture without limitation")
	}
	fixture, err := importCoverageDTO(&corev1.HistoryCoverage{
		Kind: corev1.HistoryCoverageKind_HISTORY_COVERAGE_KIND_DATASET_FIXTURE_ASSUMPTION, FromDate: "2026-01-01", ThroughDate: "2026-01-31", Reference: "ref", DataVersion: 1,
		Limitations: []string{"dataset_coverage_assumed_not_bank_certified"},
	})
	if err != nil || fixture.(object)["kind"] != "dataset_fixture_assumption" {
		t.Fatal(fixture, err)
	}
}

func TestImportDTOAndIssuesRejectMalformed(t *testing.T) {
	if _, err := importDTO(nil, accountID); err == nil {
		t.Fatal("nil import")
	}
	if _, err := importIssuesDTO([]*corev1.ImportIssue{nil}); err == nil {
		t.Fatal("nil issue")
	}
	if _, err := importIssuesDTO([]*corev1.ImportIssue{{Code: "FACTUAL_SOURCE_CONFLICT", Severity: corev1.IssueSeverity_ISSUE_SEVERITY_WARNING, Message: "bad"}}); err == nil {
		t.Fatal("conflict must be blocking")
	}
	row := uint64(1)
	if _, err := importIssuesDTO([]*corev1.ImportIssue{{Code: "INVALID_ROW", Severity: corev1.IssueSeverity_ISSUE_SEVERITY_WARNING, Message: "row", RowNumber: &row}}); err == nil {
		t.Fatal("row number too small")
	}
	if _, err := importIssuesDTO([]*corev1.ImportIssue{{Code: "INVALID_ROW", Severity: corev1.IssueSeverity_ISSUE_SEVERITY_WARNING, Message: "row", RelatedExternalOperationIds: []string{""}}}); err == nil {
		t.Fatal("empty related id")
	}
	ok, err := importIssuesDTO([]*corev1.ImportIssue{{Code: "INVALID_ROW", Severity: corev1.IssueSeverity_ISSUE_SEVERITY_WARNING, Message: "row"}})
	if err != nil || len(ok) != 1 {
		t.Fatal(ok, err)
	}
}
