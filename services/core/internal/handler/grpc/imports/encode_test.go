package imports

import (
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/imports"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
)

func TestFailedImportOmitsRetainedPreviewAndPendingCannotCommit(t *testing.T) {
	record := domain.Record{ID: uuid.New(), Status: "preview_ready", Version: 2, CreatedAt: time.Now(), UpdatedAt: time.Now(), Metadata: domain.Metadata{Scope: ledger.Scope{CompanyID: uuid.New(), AccountID: uuid.New()}, Adapter: "dataset_v1", Currency: "RUB", SourceCompanyID: "C001"}, Preview: &domain.Preview{Blockers: 1, BaseVersion: 1, Source: domain.Binding{Mode: "manual"}}}
	preview := encode(record).Preview
	if preview == nil || preview.CanCommit || preview.TransactionSource.SourceSystem != "manual_v1" || preview.TransactionSource.Mode != pb.FactualSourceMode_FACTUAL_SOURCE_MODE_MANUAL {
		t.Fatal("preview exposed unsupported commit or invalid manual namespace", preview)
	}
	record.Status = "failed"
	record.ErrorCode = "IMPORT_STATE_CONFLICT"
	failed := encode(record)
	if failed.Preview != nil || failed.Error == nil || failed.Error.Code != "IMPORT_STATE_CONFLICT" || record.Preview == nil {
		t.Fatal("terminal projection leaked retained preview or erased evidence", failed)
	}
}

func TestFuturePeriodCannotCommitAndUnknownCoverageIsUnspecified(t *testing.T) {
	location, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().In(location)
	record := domain.Record{ID: uuid.New(), Status: "preview_ready", Version: 2, CreatedAt: time.Now(), UpdatedAt: time.Now(), Metadata: domain.Metadata{Scope: ledger.Scope{CompanyID: uuid.New(), AccountID: uuid.New()}, Adapter: "dataset_v1", Currency: "RUB", SourceCompanyID: "C001"}, Preview: &domain.Preview{PeriodStart: "2026-09-01", PeriodEnd: today.AddDate(0, 0, 1).Format(time.DateOnly), BaseVersion: 1, Coverage: &domain.HistoryCoverage{Kind: "bank_export"}}}
	preview := encode(record).Preview
	if preview == nil || preview.CanCommit || preview.HistoryCoverage.Kind != pb.HistoryCoverageKind_HISTORY_COVERAGE_KIND_UNSPECIFIED {
		t.Fatal("future period or unknown coverage was presented as committable proof", preview)
	}
	record.Preview.PeriodEnd = today.Format(time.DateOnly)
	record.Preview.Coverage.Kind = "certified"
	preview = encode(record).Preview
	if preview == nil || !preview.CanCommit || preview.HistoryCoverage.Kind != pb.HistoryCoverageKind_HISTORY_COVERAGE_KIND_CERTIFIED {
		t.Fatal("current certified period was not committable", preview)
	}
}
