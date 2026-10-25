package imports

import (
	"bufio"
	"encoding/csv"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/imports"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

type parseStats struct{ Rows, Excluded int64 }

// Bound logical CSV records, including quoted multiline text, before encoding/csv
// allocates an unbounded record. Doubled quotes toggle twice and stay in-field.
type recordLimitReader struct {
	reader io.Reader
	quoted bool
	size   int
}

func (r *recordLimitReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	for i, b := range p[:n] {
		r.size++
		if b == '"' {
			r.quoted = !r.quoted
		}
		if r.size > 64<<10 {
			return i, domain.ErrInvalid
		}
		if b == '\n' && !r.quoted {
			r.size = 0
		}
	}
	return n, err
}

func parseDataset(input io.Reader, company string, flush func([]domain.NormalizedRow, []domain.Issue) error) (parseStats, error) {
	stats := parseStats{}
	buffer := bufio.NewReader(&recordLimitReader{reader: input})
	if bom, _ := buffer.Peek(3); string(bom) == "\xef\xbb\xbf" {
		if _, err := buffer.Discard(3); err != nil {
			return stats, err
		}
	}
	reader := csv.NewReader(buffer)
	reader.Comma = ';'
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return stats, parserError(err)
	}
	columns := []string{"company_id", "operation_id", "operation_date", "amount", "direction", "counterparty_inn", "counterparty_name", "payment_purpose", "balance_eod"}
	if len(header) != len(columns) {
		return stats, domain.ErrInvalid
	}
	index := map[string]int{}
	for n, name := range header {
		if _, exists := index[name]; exists {
			return stats, domain.ErrInvalid
		}
		index[name] = n
	}
	for _, name := range columns {
		if _, exists := index[name]; !exists {
			return stats, domain.ErrInvalid
		}
	}
	var rows []domain.NormalizedRow
	var issues []domain.Issue
	selected := int64(0)
	for {
		raw, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return stats, parserError(err)
		}
		stats.Rows++
		if stats.Rows > domain.MaxRows {
			return stats, domain.ErrTooLarge
		}
		switch {
		case len(raw) != len(columns):
			issues = append(issues, domain.Issue{Code: "INVALID_ROW", Severity: "blocking", Row: stats.Rows + 1})
		case raw[index["company_id"]] != company:
			stats.Excluded++
		default:
			selected++
			row, rowIssues := parseRow(raw, index, stats.Rows+1)
			issues = append(issues, rowIssues...)
			if row != nil {
				rows = append(rows, *row)
			}
		}
		if len(rows)+len(issues) >= 250 {
			if err := flush(rows, issues); err != nil {
				return stats, err
			}
			rows = nil
			issues = nil
		}
	}
	if selected == 0 {
		issues = append(issues, domain.Issue{Code: "INVALID_ROW", Severity: "blocking", Field: "source_company_id"})
	}
	if stats.Excluded > 0 {
		issues = append(issues, domain.Issue{Code: "MULTIPLE_SOURCE_COMPANIES", Severity: "warning"})
	}
	if err := flush(rows, issues); err != nil {
		return stats, err
	}
	return stats, nil
}

func parserError(err error) error {
	var parseError *csv.ParseError
	if errors.As(err, &parseError) || errors.Is(err, io.EOF) {
		return domain.ErrInvalid
	}
	return err
}

func parseRow(raw []string, index map[string]int, number int64) (*domain.NormalizedRow, []domain.Issue) {
	get := func(name string) string { return raw[index[name]] }
	row := domain.NormalizedRow{Row: number, ExternalID: get("operation_id"), Date: get("operation_date"), INN: get("counterparty_inn"), Name: get("counterparty_name"), Purpose: get("payment_purpose")}
	var issues []domain.Issue
	bad := func(field string) {
		issues = append(issues, domain.Issue{Code: "INVALID_ROW", Severity: "blocking", Row: number, Field: field})
	}
	for _, value := range raw {
		if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			bad("row")
			return nil, issues
		}
	}
	if len(row.ExternalID) < 1 || len(row.ExternalID) > 128 {
		bad("operation_id")
	}
	if ledger.Date(row.Date).Validate() != nil {
		bad("operation_date")
	}
	amount, err := parseMinor(get("amount"))
	if err != nil || amount <= 0 {
		bad("amount")
	}
	row.Amount = amount
	balance, err := parseMinor(get("balance_eod"))
	if err != nil {
		bad("balance_eod")
	}
	row.Balance = balance
	switch get("direction") {
	case "Поступление":
		row.Direction = "inflow"
	case "Списание":
		row.Direction = "outflow"
	default:
		bad("direction")
	}
	if row.INN != "" {
		if len(row.INN) != 10 && len(row.INN) != 12 {
			bad("counterparty_inn")
		}
		for _, c := range row.INN {
			if c < '0' || c > '9' {
				bad("counterparty_inn")
				break
			}
		}
	}
	if utf8.RuneCountInString(row.Name) > 500 {
		bad("counterparty_name")
	}
	if utf8.RuneCountInString(row.Purpose) > 2000 {
		bad("payment_purpose")
	}
	if len(issues) > 0 {
		return nil, issues
	}
	if row.INN == "" {
		issues = append(issues, domain.Issue{Code: "MISSING_COUNTERPARTY_INN", Severity: "warning", Row: number, Field: "counterparty_inn"})
	}
	if row.Purpose == "" {
		issues = append(issues, domain.Issue{Code: "EMPTY_PAYMENT_PURPOSE", Severity: "warning", Row: number, Field: "payment_purpose"})
	}
	return &row, issues
}

func parseMinor(raw string) (int64, error) {
	negative := strings.HasPrefix(raw, "-")
	if negative {
		raw = raw[1:]
	}
	parts := strings.Split(raw, ".")
	if len(parts) > 2 || len(parts[0]) == 0 || len(parts[0]) > 17 {
		return 0, domain.ErrInvalid
	}
	fraction := "00"
	if len(parts) == 2 {
		if len(parts[1]) < 1 || len(parts[1]) > 2 {
			return 0, domain.ErrInvalid
		}
		fraction = parts[1]
		if len(fraction) == 1 {
			fraction += "0"
		}
	}
	value := parts[0] + fraction
	for _, c := range value {
		if c < '0' || c > '9' {
			return 0, domain.ErrInvalid
		}
	}
	if negative {
		value = "-" + value
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, domain.ErrInvalid
	}
	return n, nil
}
