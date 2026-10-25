package forecasting

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
)

func TestCurrentCalendarActivationRequiresExplicitCompatibleGroup(t *testing.T) {
	scope := ledger.Scope{CompanyID: uuid.New(), AccountID: uuid.New()}
	amount, _ := money.New(100, money.RUB)
	g := domain.ReconciliationGroup{ID: uuid.New(), Scope: scope, Version: 1, Status: "active", Cutoff: "2026-10-24", HorizonStart: "2026-10-25", HorizonEnd: "2026-11-23", ConfirmedBy: "actor", ConfirmedAt: time.Now(), Additional: true}
	p, err := payments.NewExpectedPayment(payments.PaymentData{ID: uuid.New(), ReconciliationGroupID: g.ID, Scope: scope, Direction: ledger.Outflow, Amount: amount, ScheduledDate: "2026-10-27", Source: ledger.Source{Kind: ledger.Manual}, CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	c := domain.Calendar{Captured: true, KnowledgeMode: "unavailable", Reason: domain.HistoricalCalendarUnavailable, Payments: []payments.ExpectedPayment{p}, Groups: []domain.ReconciliationGroup{g}, LatestFactDate: "2026-10-24"}
	got, err := activateCalendar(c, "2026-10-24", "2026-10-25", nil)
	if err != nil || got.KnowledgeMode != "current" || got.AsOf != "2026-10-25" {
		t.Fatal(got, err)
	}
	for name, change := range map[string]func(*domain.Calendar){"missing group": func(c *domain.Calendar) { c.Groups = nil }, "revoked group": func(c *domain.Calendar) { c.Groups[0].Status = "revoked" }, "wrong horizon": func(c *domain.Calendar) { c.Groups[0].HorizonEnd = "2026-11-24" }, "post cutoff fact": func(c *domain.Calendar) { c.LatestFactDate = "2026-10-25" }} {
		t.Run(name, func(t *testing.T) {
			bad := c
			bad.Groups = append([]domain.ReconciliationGroup(nil), c.Groups...)
			change(&bad)
			if _, err := activateCalendar(bad, "2026-10-24", "2026-10-25", nil); !errors.Is(err, ErrHorizon) {
				t.Fatal(err)
			}
		})
	}
	if _, err := activateCalendar(c, "2026-10-24", "2026-10-26", nil); !errors.Is(err, ErrHorizon) {
		t.Fatal("today calendar silently used in historical forecast", err)
	}
	empty := domain.Calendar{Captured: true, KnowledgeMode: "unavailable", Reason: domain.HistoricalCalendarUnavailable}
	available, err := activateCalendar(empty, "2026-10-24", "2026-10-25", nil)
	if err != nil || available.KnowledgeMode != "current" {
		t.Fatal("available empty conflated with unavailable", err)
	}
	historical, err := activateCalendar(empty, "2026-10-24", "2026-10-26", nil)
	if err != nil || historical.KnowledgeMode != "unavailable" {
		t.Fatal("invented historical knowledge", err)
	}
}
