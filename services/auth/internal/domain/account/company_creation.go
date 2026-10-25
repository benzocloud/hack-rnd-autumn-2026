package account

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	ErrCompanySpec = errors.New("invalid company spec")
	ErrConflict    = errors.New("auth record conflict")
	ErrLeaseLost   = errors.New("company creation lease lost")
)

const (
	CreationPrepared    = "PREPARED"
	CreationCompleted   = "COMPLETED"
	CreationQuarantined = "QUARANTINED"
)

// CompanySpec is Auth's normalized durable payload, independent of Core's domain.
// Empty INN represents absent/null, never an empty JSON string.
type CompanySpec struct{ Name, INN, Currency, Timezone string }

func (s CompanySpec) Normalize() (CompanySpec, error) {
	s.Name = strings.TrimSpace(s.Name)
	if err := s.Validate(); err != nil {
		return CompanySpec{}, err
	}
	return s, nil
}

func (s CompanySpec) Validate() error {
	if s.Name == "" || strings.TrimSpace(s.Name) != s.Name || !utf8.ValidString(s.Name) || utf8.RuneCountInString(s.Name) > 160 || strings.ContainsRune(s.Name, 0) || s.Currency != "RUB" || s.Timezone != "Europe/Moscow" {
		return ErrCompanySpec
	}
	if s.INN != "" {
		if len(s.INN) != 10 && len(s.INN) != 12 {
			return ErrCompanySpec
		}
		for _, digit := range s.INN {
			if digit < '0' || digit > '9' {
				return ErrCompanySpec
			}
		}
	}
	return nil
}

// Fingerprint follows v1: fixed order, Go JSON escaping, null INN and no LF.
func (s CompanySpec) Fingerprint() (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	var inn *string
	if s.INN != "" {
		value := s.INN
		inn = &value
	}
	payload := struct {
		Name     string  `json:"name"`
		INN      *string `json:"inn"`
		Currency string  `json:"currency"`
		Timezone string  `json:"timezone"`
	}{s.Name, inn, s.Currency, s.Timezone}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

// CompanyCreation retains initiator identity even after the user is deleted.
// Recovery metadata is storage state; browser requests never supply it.
type CompanyCreation struct {
	ID, CompanyID, InitiatorID          uuid.UUID
	Key, RequestSHA256, State           string
	Spec                                CompanySpec
	CreatedAt, UpdatedAt, NextAttemptAt time.Time
	Attempts                            int64
	LeaseToken                          uuid.UUID
	LeaseExpiresAt                      *time.Time
	AccountID                           uuid.UUID
	CommittedAt                         *time.Time
}
