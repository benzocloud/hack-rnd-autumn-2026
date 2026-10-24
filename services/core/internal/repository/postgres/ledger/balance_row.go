package ledger

import (
	"time"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

type balanceRow struct {
	Date       string             `db:"balance_date"`
	Amount     int64              `db:"amount_minor"`
	Currency   string             `db:"currency"`
	Kind       domain.BalanceKind `db:"source_kind"`
	System     string             `db:"source_system"`
	CompanyID  string             `db:"source_company_id"`
	RecordedAt time.Time          `db:"recorded_at"`
	Revision   int64              `db:"created_data_version"`
}

func (r balanceRow) observation(scope domain.Scope) (domain.BalanceObservation, error) {
	amount, err := money.New(r.Amount, r.Currency)
	if err != nil {
		return domain.BalanceObservation{}, err
	}
	return domain.NewBalanceObservation(scope, domain.Date(r.Date), amount, domain.BalanceSource{Kind: r.Kind, Namespace: domain.Namespace{System: r.System, CompanyID: r.CompanyID}, RecordedAt: r.RecordedAt})
}
