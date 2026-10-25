package imports

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/imports"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
)

func encode(record domain.Record) *pb.Import {
	status := map[string]pb.ImportStatus{"queued": pb.ImportStatus_IMPORT_STATUS_QUEUED, "processing": pb.ImportStatus_IMPORT_STATUS_PROCESSING, "preview_ready": pb.ImportStatus_IMPORT_STATUS_PREVIEW_READY, "failed": pb.ImportStatus_IMPORT_STATUS_FAILED, "applied": pb.ImportStatus_IMPORT_STATUS_APPLIED}[record.Status]
	out := &pb.Import{Id: record.ID.String(), AccountId: record.Metadata.Scope.AccountID.String(), SourceKind: pb.RecordSourceKind_RECORD_SOURCE_KIND_FILE, Adapter: record.Metadata.Adapter, SourceCompanyId: record.Metadata.SourceCompanyID, Status: status, Version: record.Version, CreatedAt: timestamppb.New(record.CreatedAt), UpdatedAt: timestamppb.New(record.UpdatedAt), RecordKinds: []pb.ImportRecordKind{pb.ImportRecordKind_IMPORT_RECORD_KIND_TRANSACTIONS, pb.ImportRecordKind_IMPORT_RECORD_KIND_BALANCE_OBSERVATIONS}}
	if record.AppliedDataVersion > 0 {
		out.AppliedDataVersion = &record.AppliedDataVersion
	}
	if record.Coverage != nil {
		c := record.Coverage
		out.HistoryCoverage = &pb.HistoryCoverage{Kind: coverageKind(c.Kind), FromDate: c.FromDate, ThroughDate: c.ThroughDate, Reference: c.Reference, DataVersion: c.DataVersion, Limitations: c.Limitations}
	}
	if record.Status == "failed" {
		out.Error = &pb.OperationError{Code: record.ErrorCode, Message: "Import could not be validated; upload a corrected file", RequestId: record.ID.String()}
	}
	if !record.ExpiresAt.IsZero() {
		out.ExpiresAt = timestamppb.New(record.ExpiresAt)
	}
	if record.Preview == nil || record.Status != "preview_ready" && record.Status != "applied" {
		return out
	}
	p := record.Preview
	coverage := p.Coverage
	if record.Status == "applied" {
		coverage = record.Coverage
	}
	preview := &pb.ImportPreview{SourceCompanyId: record.Metadata.SourceCompanyID, SourceSystem: "dataset_v1", AccountId: record.Metadata.Scope.AccountID.String(), Currency: "RUB", PeriodStart: p.PeriodStart, PeriodEnd: p.PeriodEnd, RowCount: uint64(p.Rows), ExcludedOtherCompanyRowCount: uint64(p.Excluded), NewTransactionCount: uint64(p.NewTransactions), AlreadyPresentCount: uint64(p.AlreadyPresent), BlockingIssueCount: uint64(p.Blockers), WarningCount: uint64(p.Warnings), BaseDataVersion: p.BaseVersion, PreviewVersion: record.Version, CanCommit: canCommit(p), RecordKinds: out.RecordKinds, IssuesTruncated: p.Blockers+p.Warnings > int64(len(p.Issues)), Issues: []*pb.ImportIssue{}, BalanceObservations: []*pb.BalanceObservation{}}
	if coverage != nil {
		preview.HistoryCoverage = &pb.HistoryCoverage{Kind: coverageKind(coverage.Kind), FromDate: coverage.FromDate, ThroughDate: coverage.ThroughDate, Reference: coverage.Reference, DataVersion: coverage.DataVersion, Limitations: append([]string(nil), coverage.Limitations...)}
	}
	if p.Source.Mode != "" {
		preview.TransactionSource = encodeBinding(p.Source)
	}
	if p.WouldBind != nil {
		preview.WouldBindTransactionSource = encodeBinding(*p.WouldBind)
	}
	for _, issue := range p.Issues {
		preview.Issues = append(preview.Issues, encodeIssue(issue))
	}
	for _, b := range p.Balances {
		preview.BalanceObservations = append(preview.BalanceObservations, &pb.BalanceObservation{Date: b.Date, Balance: &pb.Money{AmountMinor: b.Minor, Currency: "RUB"}, Source: pb.BalanceSource_BALANCE_SOURCE_STATEMENT_EOD})
	}
	out.Preview = preview
	return out
}

func coverageKind(kind string) pb.HistoryCoverageKind {
	switch kind {
	case "certified":
		return pb.HistoryCoverageKind_HISTORY_COVERAGE_KIND_CERTIFIED
	case "dataset_fixture_assumption":
		return pb.HistoryCoverageKind_HISTORY_COVERAGE_KIND_DATASET_FIXTURE_ASSUMPTION
	default:
		return pb.HistoryCoverageKind_HISTORY_COVERAGE_KIND_UNSPECIFIED
	}
}

// Commit rejects a period that ends after Moscow today. A stored blocker would
// still refuse the file after that day arrives, so the clock is read here.
func canCommit(preview *domain.Preview) bool {
	if preview == nil || preview.Blockers > 0 {
		return false
	}
	location, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		return false
	}
	today := time.Now().In(location).Format(time.DateOnly)
	return preview.PeriodEnd <= today
}

func encodeBinding(b domain.Binding) *pb.FactualSourceBinding {
	mode := pb.FactualSourceMode_FACTUAL_SOURCE_MODE_MANUAL
	if b.Mode == "imported" {
		mode = pb.FactualSourceMode_FACTUAL_SOURCE_MODE_IMPORTED
	}
	system := b.System
	if b.Mode == "manual" {
		system = "manual_v1"
	}
	out := &pb.FactualSourceBinding{Mode: mode, SourceSystem: system}
	if b.CompanyID != "" {
		out.SourceCompanyId = &b.CompanyID
	}
	return out
}
func encodeIssue(issue domain.Issue) *pb.ImportIssue {
	severity := pb.IssueSeverity_ISSUE_SEVERITY_WARNING
	if issue.Severity == "blocking" {
		severity = pb.IssueSeverity_ISSUE_SEVERITY_BLOCKING
	}
	out := &pb.ImportIssue{Code: issue.Code, Severity: severity, Message: "Review the indicated input field or source binding", RelatedExternalOperationIds: issue.ExternalIDs}
	if issue.Row > 0 {
		n := uint64(issue.Row)
		out.RowNumber = &n
	}
	if issue.Field != "" {
		out.Field = &issue.Field
	}
	return out
}
