package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFixtureCoverageRequiresExplicitEnablementAndExactScope(t *testing.T) {
	entry := FixtureCoverage{CompanyID: "019a0c65-9800-7000-8000-000000000001", AccountID: "019a0c65-9800-7000-8000-000000000002", FileSHA256: strings.Repeat("a", 64), SourceCompanyID: "C001", FromDate: "2026-07-03", ThroughDate: "2026-09-30", CutoffDate: "2026-09-30", CutoffBalanceMinor: -100, CutoffRowNumbers: []int64{2}}
	path := filepath.Join(t.TempDir(), "manifest.json")
	data, err := json.Marshal([]FixtureCoverage{entry})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFixtureCoverage(path, false); err == nil {
		t.Fatal("enabled without explicit flag")
	}
	if got, err := LoadFixtureCoverage(path, true); err != nil || len(got) != 1 || got[0].CutoffBalanceMinor != -100 {
		t.Fatal(got, err)
	}
	for _, change := range []func(*FixtureCoverage){func(e *FixtureCoverage) { e.CompanyID = "foreign" }, func(e *FixtureCoverage) { e.FileSHA256 = "" }, func(e *FixtureCoverage) { e.CutoffRowNumbers = []int64{2, 2} }, func(e *FixtureCoverage) { e.FromDate = "2026-10-01" }} {
		invalid := entry
		change(&invalid)
		if validateFixtureCoverage(invalid) == nil {
			t.Fatal("invalid entry accepted")
		}
	}
	if err := os.WriteFile(path, append(data, []byte(` {"extra":true}`)...), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFixtureCoverage(path, true); err == nil {
		t.Fatal("trailing JSON accepted")
	}
}
