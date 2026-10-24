package imports

import (
	"errors"
	"io"
	"strings"
	"testing"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/imports"
)

type repeatedCSVRow struct {
	remaining int
	pending   string
}

func (r *repeatedCSVRow) Read(p []byte) (int, error) {
	if r.pending == "" {
		if r.remaining == 0 {
			return 0, io.EOF
		}
		r.remaining--
		r.pending = "C001;OP1;2026-09-30;1.00;Списание;0012345678;name;purpose;1.00\n"
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}
func TestDatasetRowLimitIsExactAndBatchesStayBounded(t *testing.T) {
	for _, count := range []int{domain.MaxRows, domain.MaxRows + 1} {
		t.Run(stringCount(count), func(t *testing.T) {
			reader := io.MultiReader(strings.NewReader(header), &repeatedCSVRow{remaining: count})
			rows := 0
			stats, err := parseDataset(reader, "C001", func(batch []domain.NormalizedRow, issues []domain.Issue) error {
				if len(batch)+len(issues) > 250 {
					t.Fatal("unbounded parsing batch")
				}
				rows += len(batch)
				return nil
			})
			if count == domain.MaxRows {
				if err != nil || stats.Rows != domain.MaxRows || rows != domain.MaxRows {
					t.Fatal(stats, rows, err)
				}
			} else if !errors.Is(err, domain.ErrTooLarge) {
				t.Fatal("row limit accepted", stats, err)
			}
		})
	}
}
func stringCount(count int) string {
	if count == domain.MaxRows {
		return "maximum"
	}
	return "above maximum"
}
