//go:build integration

package app

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
)

func TestExpenseScenarioDurabilityReplayAndIsolation(t *testing.T) {
	f := newManualBalanceFixture(t)
	cutoff, revision := seedForecastHistory(t, f, true)
	cfg := config.Postgres{QueryTimeout: 5 * time.Second}
	jobs := newForecastJobs(f.app.db, cfg, forecastModelFunc(deterministicForecast))
	accepted, err := jobs.Create(t.Context(), forecasting.CreateForecastCommand{Scope: f.scope, ActorID: "actor", IdempotencyKey: uuid.NewString(), Cutoff: cutoff, Today: f.today, HorizonDays: 30})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jobs.ProcessOne(t.Context()); err != nil {
		t.Fatal(err)
	}
	base, err := jobs.Read(t.Context(), f.scope, accepted.ID)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(base.Result)
	scenarios := newScenarios(f.app.db, cfg, jobs)
	cmd := forecasting.ScenarioCommand{Scope: f.scope, ActorID: "actor", IdempotencyKey: uuid.NewString(), Name: "Purchase", BaseForecastID: accepted.ID, Changes: []forecasting.ExpenseChange{{ID: uuid.New(), Date: base.Result.Days[2].Date, AmountMinor: 500, Description: "equipment"}}}
	view, err := scenarios.Create(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := scenarios.Create(t.Context(), cmd)
	if err != nil || replay.Record.ID != view.Record.ID {
		t.Fatal("replay", err)
	}
	cmd.Name = "changed"
	if _, err := scenarios.Create(t.Context(), cmd); !errors.Is(err, forecasting.ErrJobConflict) {
		t.Fatal("body conflict", err)
	}
	restarted := newScenarios(f.app.db, cfg, jobs)
	derived, err := restarted.Forecast(t.Context(), f.scope, view.Record.ForecastID)
	if err != nil || derived.Result.Days[1].Balance != base.Result.Days[1].Balance || derived.Result.Days[2].Balance != base.Result.Days[2].Balance-500 || derived.ScenarioID == nil {
		t.Fatal("derived forecast", err)
	}
	listed, err := restarted.List(t.Context(), forecasting.ScenarioListCommand{Scope: f.scope, ActorID: "actor", Limit: 1})
	if err != nil || len(listed.Items) != 1 {
		t.Fatal("list", err)
	}
	foreign := f.scope
	foreign.CompanyID = uuid.New()
	if _, err := restarted.Read(t.Context(), foreign, view.Record.ID); !errors.Is(err, forecasting.ErrNotFound) {
		t.Fatal("cross-company read", err)
	}
	after, err := jobs.Read(t.Context(), f.scope, accepted.ID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(after.Result)
	if string(before) != string(encoded) {
		t.Fatal("base forecast mutated")
	}
	assertRevision(t, f.app, f.scope, revision)
	if _, err := f.app.db.ExecContext(t.Context(), "DELETE FROM core_scenarios WHERE company_id=$1", f.scope.CompanyID); err != nil {
		t.Fatal(err)
	}
}
