package company

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func TestSharedAuthFingerprintFixtures(t *testing.T) {
	data, err := os.ReadFile("../../../../../testdata/company-bootstrap-fingerprint-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name          string
		Normalized    Spec
		CanonicalJSON string `json:"canonical_json"`
		SHA256        string
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 5 {
		t.Fatal("shared fixtures missing")
	}
	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			digest := sha256.Sum256([]byte(tt.CanonicalJSON))
			if hex.EncodeToString(digest[:]) != tt.SHA256 {
				t.Fatal("fixture canonical bytes/hash differ")
			}
			got, err := Fingerprint(tt.Normalized)
			if err != nil || got != tt.SHA256 {
				t.Fatalf("fingerprint %s, want %s: %v", got, tt.SHA256, err)
			}
		})
	}
}
