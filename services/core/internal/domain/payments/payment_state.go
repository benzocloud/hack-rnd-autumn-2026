package payments

import (
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

func RestoreExpectedPayment(data PaymentData, remaining money.Money, status Status, version int64) (ExpectedPayment, error) {
	if err := validatePaymentState(data.Amount, remaining, status, version); err != nil {
		return ExpectedPayment{}, err
	}
	p, err := NewExpectedPayment(data)
	if err != nil {
		return ExpectedPayment{}, err
	}
	p.remaining, p.status, p.version = remaining, status, version
	return p, nil
}

func validatePaymentState(amount, remaining money.Money, status Status, version int64) error {
	if version < 1 {
		return ledger.ErrVersion
	}
	if err := remaining.Nonnegative(); err != nil {
		return err
	}
	if remaining.Minor() > amount.Minor() {
		return ErrAllocation
	}
	switch status {
	case Planned:
		if remaining != amount {
			return ErrState
		}
	case PartiallyMatched:
		if remaining.Minor() == 0 || remaining == amount {
			return ErrState
		}
	case Matched:
		if remaining.Minor() != 0 {
			return ErrState
		}
	case Cancelled:
		if remaining.Minor() == 0 {
			return ErrState
		}
	default:
		return ErrState
	}
	return nil
}
