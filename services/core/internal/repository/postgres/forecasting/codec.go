package forecasting

import (
	"encoding/json"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
)

func encodeSnapshot(snapshot domain.FinancialSnapshot) ([]byte, error) {
	d := snapshot.Data()
	openingAmount := d.Opening.Observation.Amount().Minor()
	p := snapshotPayload{Opening: balancePayload{AmountMinor: &openingAmount,
		Currency: d.Opening.Observation.Amount().Currency(), Source: d.Opening.Observation.Source(), DataVersion: d.Opening.DataVersion},
		Calendar: encodeCalendar(d.Calendar), Facts: make([]factPayload, 0, len(d.Facts))}
	for _, fact := range d.Facts {
		t := fact.Transaction.Data()
		amount := t.Amount.Minor()
		p.Facts = append(p.Facts, factPayload{ID: t.ID, Date: t.Date, Direction: t.Direction,
			AmountMinor: &amount, Currency: t.Amount.Currency(), Source: t.Source, CategoryID: t.Category.ID,
			CounterpartyINN: t.CounterpartyINN, CounterpartyName: t.CounterpartyName, Purpose: t.Purpose,
			Version: fact.Transaction.Version(), DataVersion: fact.DataVersion})
	}
	return json.Marshal(p)
}

func decodeSnapshot(data domain.SnapshotData, version int, payload []byte) (domain.FinancialSnapshot, error) {
	if version != 1 && version != 2 {
		return domain.FinancialSnapshot{}, domain.ErrInvalid
	}
	var p snapshotPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return domain.FinancialSnapshot{}, err
	}
	if p.Opening.AmountMinor == nil || p.Facts == nil {
		return domain.FinancialSnapshot{}, domain.ErrInvalid
	}
	amount, err := money.New(*p.Opening.AmountMinor, p.Opening.Currency)
	if err != nil {
		return domain.FinancialSnapshot{}, err
	}
	observation, err := ledger.NewBalanceObservation(data.Scope, data.Cutoff, amount, p.Opening.Source)
	if err != nil {
		return domain.FinancialSnapshot{}, err
	}
	data.Opening = domain.OpeningBalance{Observation: observation, DataVersion: p.Opening.DataVersion}
	data.Calendar, err = decodeCalendar(p.Calendar)
	if err != nil {
		return domain.FinancialSnapshot{}, err
	}
	if version == 1 && data.Calendar.Captured || version == 2 && !data.Calendar.Captured {
		return domain.FinancialSnapshot{}, domain.ErrInvalid
	}
	data.Facts = make([]domain.Fact, 0, len(p.Facts))
	for _, f := range p.Facts {
		fact, err := f.restore(data.Scope)
		if err != nil {
			return domain.FinancialSnapshot{}, err
		}
		data.Facts = append(data.Facts, fact)
	}
	return domain.NewFinancialSnapshot(data)
}

func (f factPayload) restore(scope ledger.Scope) (domain.Fact, error) {
	if f.AmountMinor == nil {
		return domain.Fact{}, domain.ErrInvalid
	}
	amount, err := money.New(*f.AmountMinor, f.Currency)
	if err != nil {
		return domain.Fact{}, err
	}
	category := ledger.Category{}
	if f.CategoryID != uuid.Nil {
		category = ledger.Category{ID: f.CategoryID, CompanyID: scope.CompanyID}
	}
	record, err := ledger.RestoreTransaction(ledger.TransactionData{ID: f.ID, Scope: scope, Date: f.Date, Direction: f.Direction,
		Amount: amount, Source: f.Source, Category: category, CounterpartyINN: f.CounterpartyINN,
		CounterpartyName: f.CounterpartyName, Purpose: f.Purpose}, f.Version)
	return domain.Fact{Transaction: record, DataVersion: f.DataVersion}, err
}

func encodeCalendar(c domain.Calendar) calendarPayload {
	p := calendarPayload{ExcludedPayments: c.ExcludedPayments, Reason: c.Reason, Captured: c.Captured, Groups: c.Groups, KnowledgeMode: c.KnowledgeMode, AsOf: c.AsOf, LatestFactDate: c.LatestFactDate, NoPostCutoffMovements: c.NoPostCutoffMovements}
	for _, payment := range c.Payments {
		d := payment.Data()
		amount, remaining := d.Amount.Minor(), payment.Remaining().Minor()
		p.Payments = append(p.Payments, paymentPayload{Data: d, AmountMinor: &amount, RemainingMinor: &remaining, Currency: d.Amount.Currency(), Status: payment.Status(), Version: payment.Version()})
	}
	return p
}
func decodeCalendar(p calendarPayload) (domain.Calendar, error) {
	c := domain.Calendar{ExcludedPayments: p.ExcludedPayments, Reason: p.Reason, Captured: p.Captured, Groups: p.Groups, KnowledgeMode: p.KnowledgeMode, AsOf: p.AsOf, LatestFactDate: p.LatestFactDate, NoPostCutoffMovements: p.NoPostCutoffMovements}
	for _, record := range p.Payments {
		if record.AmountMinor == nil || record.RemainingMinor == nil {
			return domain.Calendar{}, domain.ErrInvalid
		}
		amount, err := money.New(*record.AmountMinor, record.Currency)
		if err != nil {
			return domain.Calendar{}, err
		}
		remaining, err := money.New(*record.RemainingMinor, record.Currency)
		if err != nil {
			return domain.Calendar{}, err
		}
		record.Data.Amount = amount
		payment, err := payments.RestoreExpectedPayment(record.Data, remaining, record.Status, record.Version)
		if err != nil {
			return domain.Calendar{}, err
		}
		c.Payments = append(c.Payments, payment)
	}
	return c, nil
}
