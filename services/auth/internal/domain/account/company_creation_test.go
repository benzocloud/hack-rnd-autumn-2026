package account

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestCompanySpecSharedFingerprintFixtures(t *testing.T) {
	data, err := os.ReadFile("../../../../../testdata/company-bootstrap-fingerprint-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name              string
		Input, Normalized CompanySpec
		CanonicalJSON     string `json:"canonical_json"`
		SHA256            string
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 5 {
		t.Fatal("shared fixtures missing")
	}
	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			spec, err := tt.Input.Normalize()
			if err != nil || spec != tt.Normalized {
				t.Fatal(spec, err)
			}
			digest := sha256.Sum256([]byte(tt.CanonicalJSON))
			if hex.EncodeToString(digest[:]) != tt.SHA256 {
				t.Fatal("fixture canonical bytes/hash differ")
			}
			hash, err := spec.Fingerprint()
			if err != nil || hash != tt.SHA256 {
				t.Fatalf("fingerprint %s, want %s: %v", hash, tt.SHA256, err)
			}
		})
	}
}

func TestCompanySpecInvalid(t *testing.T) {
	for name, edit := range map[string]func(*CompanySpec){
		"empty":          func(s *CompanySpec) { s.Name = " \t" },
		"too long":       func(s *CompanySpec) { s.Name = strings.Repeat("я", 161) },
		"invalid UTF8":   func(s *CompanySpec) { s.Name = "x\xff" },
		"NUL":            func(s *CompanySpec) { s.Name = "x\x00" },
		"currency":       func(s *CompanySpec) { s.Currency = "USD" },
		"timezone":       func(s *CompanySpec) { s.Timezone = "UTC" },
		"INN length":     func(s *CompanySpec) { s.INN = "123" },
		"INN digits":     func(s *CompanySpec) { s.INN = "１２３４５６７８９０" },
		"INN whitespace": func(s *CompanySpec) { s.INN = " 123456789" },
	} {
		t.Run(name, func(t *testing.T) {
			spec := CompanySpec{Name: "Company", Currency: "RUB", Timezone: "Europe/Moscow"}
			edit(&spec)
			if _, err := spec.Normalize(); !errors.Is(err, ErrCompanySpec) {
				t.Fatal(err)
			}
			if _, err := spec.Fingerprint(); !errors.Is(err, ErrCompanySpec) {
				t.Fatal(err)
			}
		})
	}
}
