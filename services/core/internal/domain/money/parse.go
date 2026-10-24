package money

import (
	"strconv"
)

// Parse accepts the decimal integer syntax of the REST contract, including -0.
// No floats, whitespace, plus signs, leading zeroes or exponent notation are accepted.
func Parse(text, currency string) (Money, error) {
	if currency != RUB {
		return Money{}, ErrCurrency
	}
	digits := text
	if len(digits) > 0 && digits[0] == '-' {
		digits = digits[1:]
	}
	if len(digits) == 0 || (len(digits) > 1 && digits[0] == '0') {
		return Money{}, ErrAmount
	}
	for _, digit := range digits {
		if digit < '0' || digit > '9' {
			return Money{}, ErrAmount
		}
	}
	minor, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return Money{}, ErrAmount
	}
	return New(minor, currency)
}

func ParsePositive(text, currency string) (Money, error) {
	m, err := ParseNonnegative(text, currency)
	if err != nil {
		return Money{}, err
	}
	if err := m.Positive(); err != nil {
		return Money{}, err
	}
	return m, nil
}

func ParseNonnegative(text, currency string) (Money, error) {
	if len(text) > 0 && text[0] == '-' {
		return Money{}, ErrAmount
	}
	m, err := Parse(text, currency)
	if err != nil {
		return Money{}, err
	}
	if err := m.Nonnegative(); err != nil {
		return Money{}, err
	}
	return m, nil
}
