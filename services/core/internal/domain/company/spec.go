package company

import (
	"strings"
	"unicode/utf8"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

// Spec is the normalized durable Auth payload. Empty INN means JSON null;
// Core validates, but never renormalizes the already fingerprinted Auth command.
type Spec struct{ Name, INN, Currency, Timezone string }

func (s Spec) Validate() error {
	if ValidateName(s.Name) != nil || s.Currency != money.RUB || s.Timezone != "Europe/Moscow" {
		return ErrInvalid
	}
	return ValidateINN(s.INN)
}

func ValidateName(name string) error {
	if strings.TrimSpace(name) != name || name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 160 || strings.IndexByte(name, 0) >= 0 {
		return ErrInvalid
	}
	return nil
}

// Empty INN is the domain representation of an absent optional identifier.
func ValidateINN(inn string) error {
	if inn != "" {
		if len(inn) != 10 && len(inn) != 12 {
			return ErrInvalid
		}
		for _, digit := range inn {
			if digit < '0' || digit > '9' {
				return ErrInvalid
			}
		}
	}
	return nil
}
