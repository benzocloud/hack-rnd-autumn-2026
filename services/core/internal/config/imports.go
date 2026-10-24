package config

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

// FixtureCoverage is operator-owned demo configuration, never request input.
// Explicit scope and file digest prevent arbitrary CSV uploads gaining proof.
type FixtureCoverage struct {
	CompanyID          string  `json:"company_id"`
	AccountID          string  `json:"account_id"`
	FileSHA256         string  `json:"file_sha256"`
	SourceCompanyID    string  `json:"source_company_id"`
	FromDate           string  `json:"from_date"`
	ThroughDate        string  `json:"through_date"`
	CutoffDate         string  `json:"cutoff_date"`
	CutoffBalanceMinor int64   `json:"cutoff_balance_minor"`
	CutoffRowNumbers   []int64 `json:"cutoff_row_numbers"`
}

func validFixtureDate(raw string) bool {
	date, err := time.Parse(time.DateOnly, raw)
	return err == nil && date.Year() >= 1 && date.Year() <= 9999 && date.Format(time.DateOnly) == raw
}

func LoadFixtureCoverage(path string, enabled bool) ([]FixtureCoverage, error) {
	if !enabled {
		if path != "" {
			return nil, errors.New("fixture coverage path requires explicit demo enablement")
		}
		return nil, nil
	}
	if path == "" {
		return nil, errors.New("enabled fixture coverage requires a manifest")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("cannot open fixture coverage manifest")
	}
	defer func() { _ = file.Close() }()
	payload, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil || len(payload) > 1<<20 {
		return nil, errors.New("invalid fixture coverage manifest size")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var entries []FixtureCoverage
	if decoder.Decode(&entries) != nil || decoder.Decode(new(any)) != io.EOF || len(entries) == 0 || len(entries) > 1000 {
		return nil, errors.New("invalid fixture coverage manifest")
	}
	seen := make(map[string]bool)
	for _, entry := range entries {
		if err := validateFixtureCoverage(entry); err != nil {
			return nil, err
		}
		key := entry.CompanyID + "/" + entry.AccountID + "/" + entry.FileSHA256 + "/" + entry.SourceCompanyID
		if seen[key] {
			return nil, errors.New("duplicate fixture coverage manifest entry")
		}
		seen[key] = true
	}
	return entries, nil
}

func validateFixtureCoverage(entry FixtureCoverage) error {
	invalid := errors.New("invalid fixture coverage manifest entry")
	for _, raw := range []string{entry.CompanyID, entry.AccountID} {
		id, err := uuid.Parse(raw)
		if err != nil || id == uuid.Nil || id.String() != raw {
			return invalid
		}
	}
	digest, err := hex.DecodeString(entry.FileSHA256)
	if err != nil || len(digest) != 32 || strings.ToLower(entry.FileSHA256) != entry.FileSHA256 {
		return invalid
	}
	if strings.TrimSpace(entry.SourceCompanyID) == "" || len(entry.SourceCompanyID) > 128 {
		return invalid
	}
	for _, raw := range []string{entry.FromDate, entry.ThroughDate, entry.CutoffDate} {
		if !validFixtureDate(raw) {
			return invalid
		}
	}
	if entry.FromDate > entry.CutoffDate || entry.CutoffDate > entry.ThroughDate || len(entry.CutoffRowNumbers) == 0 || len(entry.CutoffRowNumbers) > 300000 {
		return invalid
	}
	rows := make(map[int64]bool)
	for _, row := range entry.CutoffRowNumbers {
		if row < 2 || row > 300001 || rows[row] {
			return invalid
		}
		rows[row] = true
	}
	return nil
}
