package imports

import (
	"errors"
	"io"
	"strings"
	"testing"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/imports"
)

const header = "company_id;operation_id;operation_date;amount;direction;counterparty_inn;counterparty_name;payment_purpose;balance_eod\n"

func TestDatasetParserExactMoneyCompanySelectionAndMultiline(t *testing.T) {
	input := "\xef\xbb\xbf" + header + "C001;OP1;2026-09-29;92233720368547758.07;Поступление;0012345678;name;\"line1\nline2\";-92233720368547758.08\nC002;OP2;2026-09-30;1.00;Списание;;other;;3.00\n"
	var rows []domain.NormalizedRow
	var issues []domain.Issue
	stats, err := parseDataset(strings.NewReader(input), "C001", func(r []domain.NormalizedRow, i []domain.Issue) error {
		rows = append(rows, r...)
		issues = append(issues, i...)
		return nil
	})
	if err != nil || stats.Rows != 2 || stats.Excluded != 1 || len(rows) != 1 || rows[0].Amount != 9223372036854775807 || rows[0].Balance != -9223372036854775808 || rows[0].INN != "0012345678" || rows[0].Purpose != "line1\nline2" || len(issues) != 1 || issues[0].Code != "MULTIPLE_SOURCE_COMPANIES" {
		t.Fatal(stats, rows, issues, err)
	}
}
func TestDatasetParserRejectsMalformedFieldsAndUnboundedRecord(t *testing.T) {
	for _, value := range []string{"1e2", "1.001", "+1", "NaN", "92233720368547758.08", "1.", ".3"} {
		t.Run(value, func(t *testing.T) {
			if _, err := parseMinor(value); err == nil {
				t.Fatal("invalid money accepted")
			}
		})
	}
	for _, input := range []string{header + "C001;OP1;2026-09-30;1.00;Списание;;name;\"" + strings.Repeat("x", 65536), header + "C001;OP1;2026-09-30;1.00;Списание;;name;\"unterminated\n"} {
		_, err := parseDataset(strings.NewReader(input), "C001", func([]domain.NormalizedRow, []domain.Issue) error { return nil })
		if !errors.Is(err, domain.ErrInvalid) {
			t.Fatal("malformed record accepted", err)
		}
	}
	var issues []domain.Issue
	_, err := parseDataset(strings.NewReader(header+"C001;OP1;bad-date;0;sideways;123;name;purpose;1.00\n"), "C001", func(_ []domain.NormalizedRow, i []domain.Issue) error { issues = append(issues, i...); return nil })
	if err != nil || len(issues) < 3 {
		t.Fatal(issues, err)
	}
}
func TestDatasetParserPreservesStorageFailure(t *testing.T) {
	failure := errors.New("private storage failure")
	_, err := parseDataset(strings.NewReader(header+"C001;OP1;2026-09-30;1.00;Списание;0012345678;name;purpose;1.00\n"), "C001", func([]domain.NormalizedRow, []domain.Issue) error { return failure })
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if _, err := parseDataset(strings.NewReader(""), "C001", func([]domain.NormalizedRow, []domain.Issue) error { return nil }); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
	var record recordLimitReader
	record.reader = strings.NewReader("a\n")
	if _, err := io.ReadAll(&record); err != nil {
		t.Fatal(err)
	}
}

func TestDatasetParserUsesCanonicalIssueCodes(t *testing.T) {
	for _, scenario := range []struct {
		body string
		want []string
	}{
		{body: header + "C001;OP1;2026-09-30;1.00;Списание;;name;;1.00\nC002;OP2;2026-09-30;1.00;Списание;;other;;1.00\n", want: []string{"MISSING_COUNTERPARTY_INN", "EMPTY_PAYMENT_PURPOSE", "MULTIPLE_SOURCE_COMPANIES"}},
		{body: header + "C001;OP1;bad;1.00;Списание;;name;;1.00\n", want: []string{"INVALID_ROW"}},
		{body: header + "C002;OP1;2026-09-30;1.00;Списание;;name;;1.00\n", want: []string{"INVALID_ROW", "MULTIPLE_SOURCE_COMPANIES"}},
	} {
		var issues []domain.Issue
		_, err := parseDataset(strings.NewReader(scenario.body), "C001", func(_ []domain.NormalizedRow, batch []domain.Issue) error {
			issues = append(issues, batch...)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(issues) != len(scenario.want) {
			t.Fatal(issues, scenario.want)
		}
		for i, want := range scenario.want {
			if issues[i].Code != want {
				t.Fatal(issues, scenario.want)
			}
		}
	}
}
