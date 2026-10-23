package money_test

import (
	"errors"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	"math"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		text string
		want int64
	}{
		{"-9223372036854775808", math.MinInt64}, {"9223372036854775807", math.MaxInt64},
		{"-0", 0}, {"0", 0}, {"1", 1}, {"-1", -1},
	} {
		t.Run(tc.text, func(t *testing.T) {
			t.Parallel()
			got, err := money.Parse(tc.text, money.RUB)
			if err != nil || got.Minor() != tc.want || got.Currency() != money.RUB {
				t.Fatalf("Parse(%q) = %v, %v", tc.text, got, err)
			}
			roundtrip, err := money.Parse(got.Decimal(), money.RUB)
			if err != nil || roundtrip != got {
				t.Fatalf("roundtrip = %v, %v", roundtrip, err)
			}
		})
	}
	for _, text := range []string{"", "-", "+1", "01", "-01", " 1", "1\n", "1.0", "1e3", "１",
		"9223372036854775808", "-9223372036854775809"} {
		if _, err := money.Parse(text, money.RUB); !errors.Is(err, money.ErrAmount) {
			t.Errorf("Parse(%q): expected invalid amount, got %v", text, err)
		}
	}
	if _, err := money.New(1, "USD"); !errors.Is(err, money.ErrCurrency) {
		t.Fatalf("unexpected currency result: %v", err)
	}
	if err := (money.Money{}).Validate(); !errors.Is(err, money.ErrCurrency) {
		t.Fatalf("zero value accepted: %v", err)
	}
}
