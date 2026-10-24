package imports

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

func TestValidateCoverageManifest(t *testing.T) {
	entry := CoverageManifestEntry{
		Scope:            ledger.Scope{CompanyID: uuid.MustParse("019a0c65-9800-7000-8000-000000000001"), AccountID: uuid.MustParse("019a0c65-9800-7000-8000-000000000002")},
		FileSHA256:       strings.Repeat("a", 64),
		SourceCompanyID:  "C001",
		FromDate:         "2026-07-03",
		ThroughDate:      "2026-09-30",
		CutoffDate:       "2026-09-30",
		CutoffRowNumbers: []int64{2},
	}
	if err := ValidateCoverageManifest([]CoverageManifestEntry{entry}); err != nil {
		t.Fatalf("valid entry rejected: %v", err)
	}

	invalid := entry
	invalid.FromDate = "2026-10-01"
	if err := ValidateCoverageManifest([]CoverageManifestEntry{invalid}); err == nil {
		t.Fatal("invalid date range accepted")
	}
	if err := ValidateCoverageManifest([]CoverageManifestEntry{entry, entry}); err == nil {
		t.Fatal("duplicate entry accepted")
	}
}
