package company

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func validSpec() Spec {
	return Spec{Name: "Кофейня", Currency: "RUB", Timezone: "Europe/Moscow"}
}
func TestFingerprintAuthCanonicalJSON(t *testing.T) {
	for _, tc := range []struct {
		spec Spec
		json string
	}{
		{validSpec(), `{"name":"Кофейня","inn":null,"currency":"RUB","timezone":"Europe/Moscow"}`},
		{Spec{Name: "A<&>\nB", INN: "001234567890", Currency: "RUB", Timezone: "Europe/Moscow"}, `{"name":"A\u003c\u0026\u003e\nB","inn":"001234567890","currency":"RUB","timezone":"Europe/Moscow"}`},
	} {
		got, err := Fingerprint(tc.spec)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256([]byte(tc.json))
		if got != hex.EncodeToString(digest[:]) {
			t.Fatalf("Auth fingerprint differs: %s", got)
		}
	}
}
func TestSpecValidation(t *testing.T) {
	for name, edit := range map[string]func(*Spec){
		"blank": func(s *Spec) { s.Name = "" }, "not normalized": func(s *Spec) { s.Name = " name " },
		"unicode blank": func(s *Spec) { s.Name = "\u00a0name" }, "too long": func(s *Spec) { s.Name = strings.Repeat("Я", 161) },
		"invalid utf8": func(s *Spec) { s.Name = string([]byte{0xff}) }, "NUL": func(s *Spec) { s.Name = "na\x00me" },
		"INN length": func(s *Spec) { s.INN = "123" }, "INN unicode": func(s *Spec) { s.INN = "１２３４５６７８９０" },
		"currency": func(s *Spec) { s.Currency = "USD" }, "timezone": func(s *Spec) { s.Timezone = "UTC" },
	} {
		t.Run(name, func(t *testing.T) {
			s := validSpec()
			edit(&s)
			if _, err := Fingerprint(s); !errors.Is(err, ErrInvalid) {
				t.Fatal(err)
			}
		})
	}
	s := validSpec()
	s.Name = strings.Repeat("Я", 160)
	s.INN = "0012345678"
	if s.Validate() != nil {
		t.Fatal("valid unicode name/INN rejected")
	}
}
func TestProfileAndImmutableReceipt(t *testing.T) {
	id := uuid.MustParse("019a0c65-9800-7000-8000-000000000001")
	account := uuid.MustParse("019a0c65-9800-7000-8000-000000000002")
	at := time.Date(2026, 10, 24, 12, 0, 0, 123456789, time.FixedZone("MSK", 3*3600))
	p, err := NewProfile(id, account, validSpec(), at)
	if err != nil {
		t.Fatal(err)
	}
	if p.CreatedAt.Location() != time.UTC || p.CreatedAt.Nanosecond() != 123456000 || p.Version != 1 {
		t.Fatal(p)
	}
	hash, _ := Fingerprint(p.Spec)
	receipt := BootstrapRecord{CommandID: uuid.New(), CompanyID: id, AccountID: account, Spec: p.Spec, RequestSHA256: hash, CommittedAt: p.CreatedAt}
	if receipt.Validate() != nil {
		t.Fatal(receipt)
	}
	p.Spec.Name = "Renamed"
	if receipt.Spec.Name == p.Spec.Name || receipt.Validate() != nil {
		t.Fatal("receipt changed with profile")
	}
	receipt.RequestSHA256 = strings.Repeat("0", 64)
	if !errors.Is(receipt.Validate(), ErrReceipt) {
		t.Fatal("tampered receipt accepted")
	}
	if _, err = NewProfile(id, id, validSpec(), at); !errors.Is(err, ErrInvalid) {
		t.Fatal("same entity ids accepted")
	}
	if _, err = NewProfile(id, account, validSpec(), time.Time{}); !errors.Is(err, ErrInvalid) {
		t.Fatal("zero timestamp accepted")
	}
}
