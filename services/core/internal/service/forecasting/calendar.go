package forecasting

import (
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
)

// activateCalendar never moves overdue dates or labels today's remaining as
// historical knowledge. A past scenario needs a separately supported profile.
func activateCalendar(c domain.Calendar, cutoff, today ledger.Date, facts []domain.Fact) (domain.Calendar, error) {
	if !c.Captured {
		return c, nil
	}

	at, _ := time.Parse(time.DateOnly, string(cutoff))
	start := ledger.Date(at.AddDate(0, 0, 1).Format(time.DateOnly))
	end := ledger.Date(at.AddDate(0, 0, 30).Format(time.DateOnly))
	needsCalendar := len(c.Groups) > 0
	for _, p := range c.Payments {
		if (p.Status() == payments.Planned || p.Status() == payments.PartiallyMatched) && p.Data().ScheduledDate <= end {
			needsCalendar = true
		}
	}

	if cutoff != today && ledger.Date(at.AddDate(0, 0, 1).Format(time.DateOnly)) != today || c.LatestFactDate > cutoff {
		if needsCalendar {
			return domain.Calendar{}, ErrHorizon
		}
		return c, nil
	}
	compatible := make([]domain.ReconciliationGroup, 0)
	byID := map[uuid.UUID]domain.ReconciliationGroup{}
	operations := map[uuid.UUID]int64{}
	for _, fact := range facts {
		operations[fact.Transaction.Data().ID] = fact.Transaction.Version()
	}
	covered := map[uuid.UUID]bool{}
	for _, g := range c.Groups {
		if g.Status != "active" || g.Cutoff != cutoff || g.HorizonStart != start || g.HorizonEnd != end {
			continue
		}
		if g.ID == uuid.Nil || g.Version < 1 || g.ConfirmedBy == "" || g.ConfirmedAt.IsZero() || g.Additional != (len(g.Operations) == 0) {
			return domain.Calendar{}, ErrHorizon
		}
		for _, op := range g.Operations {
			if covered[op.ID] || operations[op.ID] != op.Version {
				return domain.Calendar{}, ErrHorizon
			}
			covered[op.ID] = true
		}
		byID[g.ID] = g
		compatible = append(compatible, g)
	}
	for _, p := range c.Payments {
		d := p.Data()
		if p.Status() != payments.Planned && p.Status() != payments.PartiallyMatched {
			continue
		}
		if d.ScheduledDate > end {
			continue
		}
		if d.ScheduledDate < start {
			return domain.Calendar{}, ErrHorizon
		}
		if _, ok := byID[d.ReconciliationGroupID]; !ok {
			return domain.Calendar{}, ErrHorizon
		}
	}
	c.Groups = compatible
	c.KnowledgeMode = "current"
	c.Reason = ""
	c.AsOf = today
	c.NoPostCutoffMovements = true
	c.ExcludedPayments = 0
	return c, nil
}
func activeCalendar(c domain.Calendar) bool { return c.Captured && c.KnowledgeMode == "current" }
