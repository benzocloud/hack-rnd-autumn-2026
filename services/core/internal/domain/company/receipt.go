package company

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// BootstrapRecord retains the original spec after later profile edits.
type BootstrapRecord struct {
	CommandID, CompanyID, AccountID uuid.UUID
	RequestSHA256                   string
	Spec                            Spec
	CommittedAt                     time.Time
}

// Fingerprint follows the accepted Auth v1 field order and standard Go JSON
// escaping, with null for absent INN and no trailing LF.
func Fingerprint(spec Spec) (string, error) {
	if err := spec.Validate(); err != nil {
		return "", err
	}
	var inn *string
	if spec.INN != "" {
		value := spec.INN
		inn = &value
	}
	payload := struct {
		Name     string  `json:"name"`
		INN      *string `json:"inn"`
		Currency string  `json:"currency"`
		Timezone string  `json:"timezone"`
	}{spec.Name, inn, spec.Currency, spec.Timezone}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func (r BootstrapRecord) Validate() error {
	if r.CommandID == uuid.Nil || r.CompanyID == uuid.Nil || r.AccountID == uuid.Nil || r.CompanyID == r.AccountID || r.CommittedAt.IsZero() {
		return ErrReceipt
	}
	expected, err := Fingerprint(r.Spec)
	if err != nil || r.RequestSHA256 != expected {
		return ErrReceipt
	}
	return nil
}
