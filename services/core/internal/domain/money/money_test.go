package money_test

import (
	"errors"
	"testing"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

func TestAmountConstraints(t *testing.T) {
	t.Parallel()
	for _, text := range []string{"0", "-0", "-1"} {
		if _, err := money.ParsePositive(text, money.RUB); !errors.Is(err, money.ErrAmount) {
			t.Errorf("positive %q: %v", text, err)
		}
	}
	for _, text := range []string{"-0", "-1"} {
		if _, err := money.ParseNonnegative(text, money.RUB); !errors.Is(err, money.ErrAmount) {
			t.Errorf("nonnegative %q: %v", text, err)
		}
	}
	if _, err := money.ParseNonnegative("0", money.RUB); err != nil {
		t.Fatal(err)
	}
	if _, err := money.ParsePositive("9223372036854775807", money.RUB); err != nil {
		t.Fatal(err)
	}
}
