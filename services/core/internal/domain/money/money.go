package money

import (
	"strconv"
)

const RUB = "RUB"

type Money struct {
	minor    int64
	currency string
}

func New(minor int64, currency string) (Money, error) {
	if currency != RUB {
		return Money{}, ErrCurrency
	}
	return Money{minor: minor, currency: currency}, nil
}

func (m Money) Minor() int64 { return m.minor }

func (m Money) Currency() string { return m.currency }

func (m Money) Decimal() string { return strconv.FormatInt(m.minor, 10) }

func (m Money) Validate() error {
	if m.currency != RUB {
		return ErrCurrency
	}
	return nil
}

func (m Money) Positive() error {
	if err := m.Validate(); err != nil {
		return err
	}
	if m.minor <= 0 {
		return ErrAmount
	}
	return nil
}

func (m Money) Nonnegative() error {
	if err := m.Validate(); err != nil {
		return err
	}
	if m.minor < 0 {
		return ErrAmount
	}
	return nil
}
