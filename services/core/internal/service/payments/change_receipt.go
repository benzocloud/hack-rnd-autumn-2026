package payments

import (
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
)

type changeReceipt struct {
	Schema               int
	Data                 domain.PaymentData
	Amount, Remaining    int64
	Status               domain.Status
	Version, DataVersion int64
}

func newChangeReceipt(r ChangePaymentResult) changeReceipt {
	return changeReceipt{Schema: 1, Data: r.Payment.Data(), Amount: r.Payment.Data().Amount.Minor(), Remaining: r.Payment.Remaining().Minor(), Status: r.Payment.Status(), Version: r.Payment.Version(), DataVersion: r.DataVersion}
}
func (r changeReceipt) result() (ChangePaymentResult, error) {
	if r.Schema != 1 || r.DataVersion < 1 {
		return ChangePaymentResult{}, ErrUnavailable
	}
	amount, err := money.New(r.Remaining, money.RUB)
	if err != nil {
		return ChangePaymentResult{}, ErrUnavailable
	}
	r.Data.Amount, err = money.New(r.Amount, money.RUB)
	if err != nil {
		return ChangePaymentResult{}, ErrUnavailable
	}
	payment, err := domain.RestoreExpectedPayment(r.Data, amount, r.Status, r.Version)
	if err != nil {
		return ChangePaymentResult{}, ErrUnavailable
	}
	return ChangePaymentResult{Payment: payment, DataVersion: r.DataVersion}, nil
}
