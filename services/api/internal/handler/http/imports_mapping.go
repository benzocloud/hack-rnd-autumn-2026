package httpapi

import (
	"strconv"

	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

func importStatusName(value corev1.ImportStatus) string {
	switch value {
	case corev1.ImportStatus_IMPORT_STATUS_UNSPECIFIED:
		return ""
	case corev1.ImportStatus_IMPORT_STATUS_QUEUED:
		return "queued"
	case corev1.ImportStatus_IMPORT_STATUS_PROCESSING:
		return "processing"
	case corev1.ImportStatus_IMPORT_STATUS_PREVIEW_READY:
		return "preview_ready"
	case corev1.ImportStatus_IMPORT_STATUS_APPLIED:
		return "applied"
	case corev1.ImportStatus_IMPORT_STATUS_FAILED:
		return "failed"
	}
	return ""
}

func importDTO(item *corev1.Import, account string) (object, error) {
	if item == nil || !resourceID(item.Id) || item.AccountId != account || item.Version < 1 || !timestamp(item.CreatedAt) || !timestamp(item.UpdatedAt) || item.UpdatedAt.AsTime().Before(item.CreatedAt.AsTime()) || !nonempty(item.SourceCompanyId, 128) || item.Adapter != "dataset_v1" || item.SourceKind != corev1.RecordSourceKind_RECORD_SOURCE_KIND_FILE || item.ConnectionId != nil {
		return nil, errProjection
	}
	kinds, err := importKindsDTO(item.RecordKinds)
	if err != nil || len(kinds) != 2 || kinds[0] != "transactions" || kinds[1] != "balance_observations" {
		return nil, errProjection
	}
	state := importStatusName(item.Status)
	if state == "" || item.ExpiresAt != nil && !timestamp(item.ExpiresAt) {
		return nil, errProjection
	}
	var preview any
	if item.Preview != nil {
		value, err := importPreviewDTO(item.Preview, account)
		if err != nil || item.Preview.SourceCompanyId != item.SourceCompanyId || item.Preview.SourceSystem != item.Adapter {
			return nil, errProjection
		}
		preview = value
	}
	var applied any
	if item.AppliedDataVersion != nil {
		if *item.AppliedDataVersion < 1 {
			return nil, errProjection
		}
		applied = strconv.FormatInt(*item.AppliedDataVersion, 10)
	}
	var operationError any
	if item.Error != nil {
		if !machineCode.MatchString(item.Error.Code) || !nonempty(item.Error.RequestId, 128) {
			return nil, errProjection
		}
		operationError = errorDetail{item.Error.Code, errorMessage(item.Error.Code), []fieldError{}, item.Error.RequestId}
	}
	switch item.Status {
	case corev1.ImportStatus_IMPORT_STATUS_QUEUED, corev1.ImportStatus_IMPORT_STATUS_PROCESSING:
		if preview != nil || applied != nil || operationError != nil || item.HistoryCoverage != nil {
			return nil, errProjection
		}
	case corev1.ImportStatus_IMPORT_STATUS_PREVIEW_READY:
		if preview == nil || applied != nil || operationError != nil || item.HistoryCoverage != nil {
			return nil, errProjection
		}
	case corev1.ImportStatus_IMPORT_STATUS_APPLIED:
		if preview == nil || applied == nil || operationError != nil {
			return nil, errProjection
		}
	case corev1.ImportStatus_IMPORT_STATUS_FAILED:
		if preview != nil || applied != nil || operationError == nil || item.HistoryCoverage != nil {
			return nil, errProjection
		}
	case corev1.ImportStatus_IMPORT_STATUS_UNSPECIFIED:
		return nil, errProjection
	default:
		return nil, errProjection
	}
	coverage, err := importCoverageDTO(item.HistoryCoverage)
	if err != nil {
		return nil, err
	}
	var expires any
	if item.ExpiresAt != nil {
		expires = utc(item.ExpiresAt)
	}
	return object{"id": item.Id, "account_id": item.AccountId, "source_kind": "file", "adapter": item.Adapter, "source_company_id": item.SourceCompanyId, "connection_id": nil, "status": state, "version": strconv.FormatInt(item.Version, 10), "created_at": utc(item.CreatedAt), "updated_at": utc(item.UpdatedAt), "preview": preview, "applied_data_version": applied, "error": operationError, "record_kinds": kinds, "history_coverage": coverage, "expires_at": expires}, nil
}

func importKindsDTO(kinds []corev1.ImportRecordKind) ([]string, error) {
	result := make([]string, 0, len(kinds))
	seen := make(map[corev1.ImportRecordKind]bool)
	for _, kind := range kinds {
		if seen[kind] {
			return nil, errProjection
		}
		seen[kind] = true
		switch kind {
		case corev1.ImportRecordKind_IMPORT_RECORD_KIND_TRANSACTIONS:
			result = append(result, "transactions")
		case corev1.ImportRecordKind_IMPORT_RECORD_KIND_BALANCE_OBSERVATIONS:
			result = append(result, "balance_observations")
		case corev1.ImportRecordKind_IMPORT_RECORD_KIND_PLANNED_PAYMENTS:
			result = append(result, "planned_payments")
		case corev1.ImportRecordKind_IMPORT_RECORD_KIND_UNSPECIFIED:
			return nil, errProjection
		default:
			return nil, errProjection
		}
	}
	if len(result) == 0 {
		return nil, errProjection
	}
	return result, nil
}

func importPreviewDTO(preview *corev1.ImportPreview, account string) (object, error) {
	if preview == nil || preview.AccountId != account || !nonempty(preview.SourceCompanyId, 128) || preview.SourceSystem != "dataset_v1" || preview.Currency != "RUB" || !businessDate(preview.PeriodStart) || !businessDate(preview.PeriodEnd) || preview.PeriodEnd < preview.PeriodStart || preview.BaseDataVersion < 1 || preview.PreviewVersion < 1 || preview.RowCount > 300000 || preview.ExcludedOtherCompanyRowCount > preview.RowCount || len(preview.Issues) > 100 || preview.CanCommit && preview.BlockingIssueCount > 0 {
		return nil, errProjection
	}
	issues, err := importIssuesDTO(preview.Issues)
	if err != nil {
		return nil, err
	}
	kinds, err := importKindsDTO(preview.RecordKinds)
	if err != nil {
		return nil, err
	}
	balances := make([]object, 0, len(preview.BalanceObservations))
	seen := make(map[string]bool)
	for _, observation := range preview.BalanceObservations {
		if observation == nil || !businessDate(observation.Date) || observation.Date < preview.PeriodStart || observation.Date > preview.PeriodEnd || !validMoney(observation.Balance) || observation.Source != corev1.BalanceSource_BALANCE_SOURCE_STATEMENT_EOD || seen[observation.Date] {
			return nil, errProjection
		}
		seen[observation.Date] = true
		balances = append(balances, object{"date": observation.Date, "balance": moneyDTO(observation.Balance), "source": "statement_eod"})
	}
	source, err := importBindingDTO(preview.TransactionSource)
	if err != nil {
		return nil, err
	}
	wouldBind, err := importBindingDTO(preview.WouldBindTransactionSource)
	if err != nil {
		return nil, err
	}
	coverage, err := importCoverageDTO(preview.HistoryCoverage)
	if err != nil {
		return nil, err
	}
	return object{"source_company_id": preview.SourceCompanyId, "source_system": preview.SourceSystem, "account_id": account, "currency": preview.Currency, "period_start": preview.PeriodStart, "period_end": preview.PeriodEnd, "row_count": preview.RowCount, "excluded_other_company_row_count": preview.ExcludedOtherCompanyRowCount, "new_transaction_count": preview.NewTransactionCount, "already_present_count": preview.AlreadyPresentCount, "new_payment_count": preview.NewPaymentCount, "blocking_issue_count": preview.BlockingIssueCount, "warning_count": preview.WarningCount, "issues": issues, "issues_truncated": preview.IssuesTruncated, "balance_observations": balances, "base_data_version": strconv.FormatInt(preview.BaseDataVersion, 10), "can_commit": preview.CanCommit, "preview_version": strconv.FormatInt(preview.PreviewVersion, 10), "record_kinds": kinds, "transaction_source": source, "would_bind_transaction_source": wouldBind, "history_coverage": coverage}, nil
}

func importIssuesDTO(issues []*corev1.ImportIssue) ([]object, error) {
	result := make([]object, 0, len(issues))
	for _, issue := range issues {
		if issue == nil || !contains(issue.Code, "MISSING_COUNTERPARTY_INN", "EMPTY_PAYMENT_PURPOSE", "SUSPECTED_CONTENT_DUPLICATE", "BALANCE_DISCONTINUITY", "SHORT_HISTORY", "DUPLICATE_EXTERNAL_ID", "CONFLICTING_EXTERNAL_ID", "INVALID_ROW", "UNSUPPORTED_CURRENCY", "MULTIPLE_SOURCE_COMPANIES", "FACTUAL_SOURCE_CONFLICT", "SOURCE_BINDING_CONFLICT") || issueSeverity(issue.Severity) == "" || !nonempty(issue.Message, 2000) || issue.RowNumber != nil && (*issue.RowNumber < 2 || *issue.RowNumber > 300001) || issue.Field != nil && !nonempty(*issue.Field, 128) {
			return nil, errProjection
		}
		if contains(issue.Code, "FACTUAL_SOURCE_CONFLICT", "SOURCE_BINDING_CONFLICT") && issue.Severity != corev1.IssueSeverity_ISSUE_SEVERITY_BLOCKING {
			return nil, errProjection
		}
		for _, id := range issue.RelatedExternalOperationIds {
			if !nonempty(id, 128) {
				return nil, errProjection
			}
		}
		result = append(result, object{"code": issue.Code, "severity": issueSeverity(issue.Severity), "message": issue.Message, "row_number": issue.RowNumber, "field": textPointer(issue.Field), "related_external_operation_ids": stringsDTO(issue.RelatedExternalOperationIds)})
	}
	return result, nil
}

func importBindingDTO(binding *corev1.FactualSourceBinding) (any, error) {
	if binding == nil {
		return nil, nil
	}
	mode := "manual"
	switch binding.Mode {
	case corev1.FactualSourceMode_FACTUAL_SOURCE_MODE_MANUAL:
		if binding.SourceSystem != "manual_v1" || binding.SourceCompanyId != nil {
			return nil, errProjection
		}
	case corev1.FactualSourceMode_FACTUAL_SOURCE_MODE_IMPORTED:
		mode = "external"
		if !nonempty(binding.SourceSystem, 128) || binding.SourceCompanyId == nil || !nonempty(*binding.SourceCompanyId, 128) {
			return nil, errProjection
		}
	case corev1.FactualSourceMode_FACTUAL_SOURCE_MODE_UNSPECIFIED:
		return nil, errProjection
	default:
		return nil, errProjection
	}
	return object{"mode": mode, "source_system": binding.SourceSystem, "source_company_id": textPointer(binding.SourceCompanyId)}, nil
}

func importCoverageDTO(coverage *corev1.HistoryCoverage) (any, error) {
	if coverage == nil {
		return nil, nil
	}
	if !businessDate(coverage.FromDate) || !businessDate(coverage.ThroughDate) || coverage.ThroughDate < coverage.FromDate || !nonempty(coverage.Reference, 256) || coverage.DataVersion < 1 {
		return nil, errProjection
	}
	kind := "certified"
	switch coverage.Kind {
	case corev1.HistoryCoverageKind_HISTORY_COVERAGE_KIND_CERTIFIED:
	case corev1.HistoryCoverageKind_HISTORY_COVERAGE_KIND_DATASET_FIXTURE_ASSUMPTION:
		kind = "dataset_fixture_assumption"
		if !contains("dataset_coverage_assumed_not_bank_certified", coverage.Limitations...) {
			return nil, errProjection
		}
	case corev1.HistoryCoverageKind_HISTORY_COVERAGE_KIND_UNSPECIFIED:
		return nil, errProjection
	default:
		return nil, errProjection
	}
	return object{"kind": kind, "from_date": coverage.FromDate, "through_date": coverage.ThroughDate, "reference": coverage.Reference, "data_version": strconv.FormatInt(coverage.DataVersion, 10), "limitations": stringsDTO(coverage.Limitations)}, nil
}
