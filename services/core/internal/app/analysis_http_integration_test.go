//go:build integration && mlintegration

package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
)

func monthlyCSVInput(cutoff time.Time) string {
	var csv strings.Builder
	_, _ = fmt.Fprintf(&csv, "company_id;operation_id;operation_date;amount;direction;counterparty_inn;counterparty_name;payment_purpose;balance_eod\nC001;FIRST;%s;90.00;Поступление;0012345678;Клиент;Первая операция;1000.00\n", cutoff.AddDate(0, 0, -89).Format(time.DateOnly))
	for n := 2; n >= 0; n-- {
		month := time.Date(cutoff.Year(), cutoff.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -n, 0)
		_, _ = fmt.Fprintf(&csv, "C001;RENT%d;%s;9.00;Списание;0098765432;Арендодатель;Аренда помещения;%d.00\n", n, month.AddDate(0, 0, min(5, cutoff.Day())-1).Format(time.DateOnly), (100000-int64(3-n)*900)/100)
	}
	_, _ = fmt.Fprintf(&csv, "C001;LAST;%s;45.00;Списание;0012345678;Поставщик;Уплата налогов ЕНП;928.00\n", cutoff.Format(time.DateOnly))
	return csv.String()
}

func checkMonthlyAnalysisHTTP(t *testing.T, f *manualBalanceFixture, company, cutoff string, jobs forecastJobs, call func(string, string, string, string, int) map[string]json.RawMessage, stop func(), start func() func()) {
	t.Helper()
	object := func(raw json.RawMessage) map[string]json.RawMessage {
		t.Helper()
		var v map[string]json.RawMessage
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	text := func(v map[string]json.RawMessage, key string) string {
		t.Helper()
		var s string
		if err := json.Unmarshal(v[key], &s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	minor := func(raw json.RawMessage) int64 {
		t.Helper()
		v, err := strconv.ParseInt(text(object(raw), "amount_minor"), 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	var revision int64
	if err := f.app.db.GetContext(t.Context(), &revision, "SELECT data_version FROM core_accounts WHERE company_id=$1 AND id=$2", f.scope.CompanyID, f.scope.AccountID); err != nil {
		t.Fatal(err)
	}
	forecast := func(key string, patternCount int, manualID string) map[string]json.RawMessage {
		t.Helper()
		accepted := call(http.MethodPost, company+"/forecasts", `{"cutoff_date":"`+cutoff+`","horizon_days":30}`, key, 202)
		if worked, err := jobs.ProcessOne(t.Context()); err != nil || !worked {
			t.Fatal(worked, err)
		}
		ready := call(http.MethodGet, text(accepted, "status_url"), "", "", 200)
		if text(ready, "status") != "succeeded" {
			t.Fatal(ready)
		}
		result := object(ready["result"])
		analysis := object(result["analysis"])
		var hypotheses, labels, points []map[string]json.RawMessage
		if json.Unmarshal(analysis["recurring_hypotheses"], &hypotheses) != nil || len(hypotheses) != patternCount || json.Unmarshal(analysis["classified_operations"], &labels) != nil || len(labels) != 5 || json.Unmarshal(result["daily_points"], &points) != nil || len(points) != 30 {
			t.Fatal(result)
		}
		if patternCount == 1 {
			h := hypotheses[0]
			var refs []map[string]json.RawMessage
			if text(h, "category_code") != "rent" || text(h, "rule_id") != "monthly_fit" || text(h, "status") != "hypothesis" || text(h, "expected_amount_minor") != "900" || string(h["confirmations"]) != "3" || json.Unmarshal(h["historical_operations"], &refs) != nil || len(refs) != 3 {
				t.Fatal(h)
			}
			known := map[string]string{}
			for _, label := range labels {
				known[text(label, "transaction_id")] = text(label, "version")
			}
			for _, ref := range refs {
				if known[text(ref, "transaction_id")] != text(ref, "version") {
					t.Fatal("foreign/stale evidence", ref)
				}
			}
		}
		if manualID != "" {
			found := false
			for _, label := range labels {
				if text(label, "transaction_id") == manualID {
					found = true
					if text(label, "category_source") != "manual" || text(label, "category_rule_id") != "" {
						t.Fatal(label)
					}
				}
			}
			if !found {
				t.Fatal("manual precedence lost")
			}
		}
		for n, point := range points {
			if minor(point["inflow"]) != 100 || minor(point["outflow"]) != 80 || minor(point["balance"]) != 92800+int64(n+1)*20 {
				t.Fatal("analysis changed money", n, point)
			}
		}
		assertRevision(t, f.app, f.scope, revision)
		assertCount(t, f.app, "SELECT count(*) FROM core_transactions WHERE company_id=$1", f.scope, 5)
		assertCount(t, f.app, "SELECT count(*) FROM core_expected_payments WHERE company_id=$1", f.scope, 0)
		assertCount(t, f.app, "SELECT count(*) FROM core_history_coverage WHERE company_id=$1", f.scope, 0)
		return ready
	}
	original := forecast("monthly-analysis-original", 1, "")
	originalInput := object(original["input"])
	var snapshotBefore, jobBefore string
	if err := f.app.db.GetContext(t.Context(), &snapshotBefore, "SELECT payload::text FROM core_financial_snapshots WHERE company_id=$1 AND id=$2", f.scope.CompanyID, text(originalInput, "snapshot_id")); err != nil {
		t.Fatal(err)
	}
	if err := f.app.db.GetContext(t.Context(), &jobBefore, "SELECT result::text FROM core_forecast_jobs WHERE company_id=$1 AND id=$2", f.scope.CompanyID, text(original, "id")); err != nil {
		t.Fatal(err)
	}
	facts := call(http.MethodGet, company+"/transactions?limit=100", "", "", 200)
	var factItems []map[string]json.RawMessage
	if json.Unmarshal(facts["items"], &factItems) != nil {
		t.Fatal(facts)
	}
	var rent map[string]json.RawMessage
	for _, fact := range factItems {
		if text(fact, "payment_purpose") == "Аренда помещения" {
			rent = fact
			break
		}
	}
	if rent == nil {
		t.Fatal("imported rent missing")
	}
	categories := call(http.MethodGet, company+"/categories?limit=100", "", "", 200)
	var items []map[string]json.RawMessage
	if json.Unmarshal(categories["items"], &items) != nil {
		t.Fatal(categories)
	}
	categoryID := ""
	for _, c := range items {
		if string(c["direction"]) == `"outflow"` {
			categoryID = text(c, "id")
			break
		}
	}
	if categoryID == "" {
		t.Fatal(categories)
	}
	categoryPath := company + "/transactions/" + text(rent, "id") + "/category"
	manual := call(http.MethodPatch, categoryPath, `{"version":"`+text(rent, "version")+`","category_id":"`+categoryID+`"}`, "", 200)
	revision++
	forecast("monthly-analysis-manual", 0, text(rent, "id"))
	call(http.MethodPatch, categoryPath, `{"version":"`+text(manual, "version")+`","category_id":null}`, "", 200)
	revision++
	cleared := forecast("monthly-analysis-clear", 1, "")
	// Same facts and money, explicit available-empty CURRENT calendar, no obligations.
	cutoffDate, _ := time.Parse(time.DateOnly, cutoff)
	call(http.MethodPost, company+"/reconciliation-groups", `{"cutoff_date":"`+cutoff+`","horizon_start":"`+cutoffDate.AddDate(0, 0, 1).Format(time.DateOnly)+`","horizon_end":"`+cutoffDate.AddDate(0, 0, 30).Format(time.DateOnly)+`","kind":"additional","additional":{"confirmed":true}}`, "monthly-analysis-additional", 201)
	revision++
	current := forecast("monthly-analysis-current", 1, "")
	if text(object(object(current["input"])["model_context"]), "model_version") != "mean_90_csv_calendar_analysis_v2" {
		t.Fatal(current)
	}
	for _, previous := range []map[string]json.RawMessage{original, cleared} {
		old := call(http.MethodGet, company+"/forecasts/"+text(previous, "id"), "", "", 200)
		if string(old["is_stale"]) != "true" || !bytes.Equal(old["result"], previous["result"]) || !bytes.Equal(old["input"], previous["input"]) {
			t.Fatal("old analysis changed")
		}
	}
	var snapshotAfter, jobAfter string
	if err := f.app.db.GetContext(t.Context(), &snapshotAfter, "SELECT payload::text FROM core_financial_snapshots WHERE company_id=$1 AND id=$2", f.scope.CompanyID, text(originalInput, "snapshot_id")); err != nil || snapshotBefore != snapshotAfter {
		t.Fatal("snapshot bytes changed", err)
	}
	if err := f.app.db.GetContext(t.Context(), &jobAfter, "SELECT result::text FROM core_forecast_jobs WHERE company_id=$1 AND id=$2", f.scope.CompanyID, text(original, "id")); err != nil || jobBefore != jobAfter {
		t.Fatal("saved bytes changed", err)
	}
	reopened, _ := testApplication(t)
	restarted := newForecastJobs(reopened.db, config.Postgres{QueryTimeout: 5 * time.Second}, nil)
	id, err := uuid.Parse(text(current, "id"))
	if err != nil {
		t.Fatal(err)
	}
	saved, err := restarted.Read(t.Context(), f.scope, id)
	if err != nil || saved.Result == nil || saved.Result.Analysis == nil || len(saved.Result.Analysis.Hypotheses) != 1 {
		t.Fatal("Core read/restart", saved, err)
	}
	stop()
	newStop := start()
	defer newStop()
	reload := call(http.MethodGet, company+"/forecasts/"+text(current, "id"), "", "", 200)
	if !bytes.Equal(reload["result"], current["result"]) {
		t.Fatal("API restart changed analysis")
	}
}
