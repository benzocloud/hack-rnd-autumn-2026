package money_test

import (
	"errors"
	"math"
	"math/big"
	"testing"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

func FuzzArithmetic(f *testing.F) {
	for _, pair := range [][2]int64{{math.MaxInt64, 1}, {math.MinInt64, -1}, {math.MinInt64, math.MinInt64},
		{0, math.MinInt64}, {math.MaxInt64, math.MinInt64}, {42, 7}, {-42, 7}} {
		f.Add(pair[0], pair[1])
	}
	f.Fuzz(func(t *testing.T, a, b int64) {
		left, err := money.New(a, money.RUB)
		if err != nil {
			t.Fatal(err)
		}
		right, err := money.New(b, money.RUB)
		if err != nil {
			t.Fatal(err)
		}
		for _, subtract := range []bool{false, true} {
			want := new(big.Int)
			var got money.Money
			if subtract {
				want.Sub(big.NewInt(a), big.NewInt(b))
				got, err = left.Sub(right)
			} else {
				want.Add(big.NewInt(a), big.NewInt(b))
				got, err = left.Add(right)
			}
			if !want.IsInt64() {
				if !errors.Is(err, money.ErrOverflow) {
					t.Fatalf("a=%d b=%d subtract=%v: overflow not rejected: %v", a, b, subtract, err)
				}
			} else if err != nil || got.Minor() != want.Int64() {
				t.Fatalf("a=%d b=%d subtract=%v: got %v, %v; want %v", a, b, subtract, got, err, want)
			}
		}
	})
}
