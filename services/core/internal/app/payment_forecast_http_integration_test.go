//go:build integration && mlintegration

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/credentials"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
	forecastHandler "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/forecasting"
	ledgerHandler "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/ledger"
	paymentsHandler "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/payments"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/grpcserver"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/mtls"
)

// This is a covered-history CURRENT-calendar fixture, not proof that an ordinary
// CSV or a synthetic historical calendar can activate this financial profile.
func TestPaymentForecastHTTPWithPostgresPythonAndRestart(t *testing.T) {
	binary := os.Getenv("CORE_API_TEST_BINARY")
	if binary == "" {
		t.Skip("set CORE_API_TEST_BINARY built from this worktree")
	}
	f := newManualBalanceFixture(t)
	cutoff, _ := seedForecastHistory(t, f, true)
	model := pythonFinancialModel(t, f)
	jobs := newForecastJobs(f.app.db, config.Postgres{QueryTimeout: 5 * time.Second}, model)
	auth := httpForecastAuth{company: f.scope.CompanyID.String()}
	tlsConfig, err := mtls.Server(f.ca.Files(t, mtls.Core))
	if err != nil {
		t.Fatal(err)
	}
	server, err := grpcserver.New(credentials.NewTLS(tlsConfig))
	if err != nil {
		t.Fatal(err)
	}
	if err := ledgerHandler.Register(server, f.app.Ledger, f.app.Companies, auth); err != nil {
		t.Fatal(err)
	}
	if err := forecastHandler.Register(server, jobs, f.app.Companies, auth); err != nil {
		t.Fatal(err)
	}
	paymentsHandler.Register(server, f.app.Payments, f.app.Companies, auth)
	listener := loopbackListener(t)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	apiListener := loopbackListener(t)
	address := apiListener.Addr().String()
	_ = apiListener.Close()
	identity := f.ca.Files(t, mtls.API)
	client := &http.Client{Timeout: 5 * time.Second}
	base := "http://" + address
	start := func() func() {
		ctx, cancel := context.WithCancel(t.Context())
		command := exec.CommandContext(ctx, binary)
		command.Env = []string{"API_ADDRESS=" + address, "API_ORIGIN=https://localhost", "API_AUTH_ADDRESS=" + listener.Addr().String(), "API_AUTH_CA=" + identity.CA, "API_AUTH_CERT=" + identity.Certificate, "API_AUTH_KEY=" + identity.Key, "API_CORE_ADDRESS=" + listener.Addr().String(), "API_CORE_SERVER_NAME=saldo-core"}
		if err := command.Start(); err != nil {
			cancel()
			t.Fatal(err)
		}
		stop := func() { cancel(); _ = command.Wait() }
		deadline := time.Now().Add(5 * time.Second)
		for {
			response, err := client.Get(base + "/health/live")
			if err == nil {
				_ = response.Body.Close()
				break
			}
			if time.Now().After(deadline) {
				stop()
				t.Fatal("API did not listen", err)
			}
			time.Sleep(10 * time.Millisecond)
		}
		return stop
	}
	stop := start()
	t.Cleanup(func() { stop() })
	call := func(method, path, body, key string, want int) map[string]json.RawMessage {
		t.Helper()
		request, err := http.NewRequestWithContext(t.Context(), method, base+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Origin", "https://localhost")
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-CSRF-Token", strings.Repeat("c", 24))
		if key != "" {
			request.Header.Set("Idempotency-Key", key)
		}
		request.AddCookie(&http.Cookie{Name: "__Host-saldo_session", Value: "live-session"})
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
		if err != nil || response.StatusCode != want {
			t.Fatalf("%s %s: got %d want %d body=%s error=%v", method, path, response.StatusCode, want, data, err)
		}
		if want == 204 {
			return nil
		}
		var result map[string]json.RawMessage
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	text := func(object map[string]json.RawMessage, key string) string {
		t.Helper()
		var value string
		if err := json.Unmarshal(object[key], &value); err != nil {
			t.Fatal(key, err)
		}
		return value
	}
	object := func(raw json.RawMessage) map[string]json.RawMessage {
		t.Helper()
		var value map[string]json.RawMessage
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	minor := func(raw json.RawMessage) int64 {
		t.Helper()
		value := text(object(raw), "amount_minor")
		amount, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		return amount
	}
	company := "/api/v1/companies/" + f.scope.CompanyID.String()
	cutoffTime, err := time.Parse(time.DateOnly, string(cutoff))
	if err != nil {
		t.Fatal(err)
	}
	group := call(http.MethodPost, company+"/reconciliation-groups", `{"cutoff_date":"`+string(cutoff)+`","horizon_start":"`+cutoffTime.AddDate(0, 0, 1).Format(time.DateOnly)+`","horizon_end":"`+cutoffTime.AddDate(0, 0, 30).Format(time.DateOnly)+`","kind":"additional","additional":{"confirmed":true}}`, "http-current-group", 201)
	due := cutoffTime.AddDate(0, 0, 2).Format(time.DateOnly)
	payment := call(http.MethodPost, company+"/payments", `{"direction":"inflow","amount":{"amount_minor":"900","currency":"RUB"},"scheduled_date":"`+due+`","counterparty_name":"Customer","description":"Explicit additional receipt","reconciliation_group_id":"`+text(group, "id")+`"}`, "http-current-payment", 201)
	paymentID := text(payment, "id")
	path := company + "/payments/" + paymentID
	forecast := func(key string, remaining int64, dueDay int) map[string]json.RawMessage {
		t.Helper()
		accepted := call(http.MethodPost, company+"/forecasts", `{"cutoff_date":"`+string(cutoff)+`","horizon_days":30}`, key, 202)
		if worked, err := jobs.ProcessOne(t.Context()); err != nil || !worked {
			t.Fatal("worker", worked, err)
		}
		ready := call(http.MethodGet, text(accepted, "status_url"), "", "", 200)
		if text(ready, "status") != "succeeded" {
			t.Fatal(ready)
		}
		input := object(ready["input"])
		if text(input, "calendar_knowledge_mode") != "current" || text(input, "calendar_as_of_date") != string(f.today) || string(input["no_post_cutoff_movements"]) != "true" {
			t.Fatal(input)
		}
		result := object(ready["result"])
		var points []map[string]json.RawMessage
		if json.Unmarshal(result["daily_points"], &points) != nil || len(points) != 30 {
			t.Fatal(result)
		}
		balance := int64(300)
		for n, point := range points {
			inflow := int64(100)
			if n+1 == dueDay {
				inflow += remaining
			}
			balance += inflow - 200
			if minor(point["inflow"]) != inflow || minor(point["outflow"]) != 200 || minor(point["balance"]) != balance || text(point, "date") != cutoffTime.AddDate(0, 0, n+1).Format(time.DateOnly) {
				t.Fatalf("point %d: %s", n+1, point)
			}
		}
		var components, occurrences []map[string]json.RawMessage
		if json.Unmarshal(result["components"], &components) != nil || json.Unmarshal(result["effective_occurrences"], &occurrences) != nil {
			t.Fatal(result)
		}
		known := 0
		for _, component := range components {
			if string(component["kind"]) == `"known_payment"` {
				known++
				if text(component, "payment_id") != paymentID || minor(component["remaining_amount"]) != remaining {
					t.Fatal(component)
				}
			}
		}
		if remaining > 0 && (known != 1 || len(occurrences) != 1) || remaining == 0 && (known != 0 || len(occurrences) != 0) {
			t.Fatal("known occurrence count", known, occurrences)
		}
		return ready
	}
	first := forecast("http-current-forecast-one", 900, 2)
	var frozenSnapshot string
	snapshotID := text(object(first["input"]), "snapshot_id")
	if err := f.app.db.GetContext(t.Context(), &frozenSnapshot, "SELECT payload::text FROM core_financial_snapshots WHERE company_id=$1 AND id=$2", f.scope.CompanyID, snapshotID); err != nil {
		t.Fatal(err)
	}
	payment = call(http.MethodPatch, path, `{"version":"`+text(payment, "version")+`","amount":{"amount_minor":"1500","currency":"RUB"}}`, "", 200)
	forecast("http-current-forecast-amount", 1500, 2)
	due = cutoffTime.AddDate(0, 0, 4).Format(time.DateOnly)
	payment = call(http.MethodPatch, path, `{"version":"`+text(payment, "version")+`","scheduled_date":"`+due+`"}`, "", 200)
	forecast("http-current-forecast-date", 1500, 4)
	facts := call(http.MethodGet, company+"/transactions?direction=inflow&from="+string(cutoff)+"&to="+string(cutoff), "", "", 200)
	var factItems []map[string]json.RawMessage
	if json.Unmarshal(facts["items"], &factItems) != nil || len(factItems) != 1 {
		t.Fatal(facts)
	}
	fact := factItems[0]
	matchBody := func(amount string) string {
		return `{"transaction_id":"` + text(fact, "id") + `","payment_version":"` + text(payment, "version") + `","transaction_version":"` + text(fact, "version") + `","amount":{"amount_minor":"` + amount + `","currency":"RUB"}}`
	}
	matched := call(http.MethodPost, path+"/matches", matchBody("40"), "http-current-match-one", 201)
	payment = call(http.MethodGet, path, "", "", 200)
	if minor(payment["remaining_amount"]) != 1460 {
		t.Fatal(payment)
	}
	forecast("http-current-forecast-partial", 1460, 4)
	call(http.MethodDelete, path+"/matches/"+text(matched, "id")+"?payment_version="+text(payment, "version"), "", "", 204)
	payment = call(http.MethodGet, path, "", "", 200)
	forecast("http-current-forecast-unmatch", 1500, 4)
	call(http.MethodPost, path+"/matches", matchBody("70"), "http-current-match-two", 201)
	payment = call(http.MethodGet, path, "", "", 200)
	forecast("http-current-forecast-rematch", 1430, 4)
	call(http.MethodDelete, path+"?version="+text(payment, "version"), "", "", 204)
	last := forecast("http-current-forecast-cancel", 0, 4)
	old := call(http.MethodGet, company+"/forecasts/"+text(first, "id"), "", "", 200)
	if string(old["is_stale"]) != "true" || !bytes.Equal(old["result"], first["result"]) || !bytes.Equal(old["input"], first["input"]) {
		t.Fatal("old forecast changed", old)
	}
	var afterSnapshot string
	if err := f.app.db.GetContext(t.Context(), &afterSnapshot, "SELECT payload::text FROM core_financial_snapshots WHERE company_id=$1 AND id=$2", f.scope.CompanyID, snapshotID); err != nil || afterSnapshot != frozenSnapshot {
		t.Fatal("frozen snapshot changed", err)
	}
	assertCount(t, f.app, "SELECT count(*) FROM core_transactions WHERE company_id=$1", f.scope, 180)
	stop()
	stop = start()
	reloaded := call(http.MethodGet, company+"/forecasts/"+text(last, "id"), "", "", 200)
	if !bytes.Equal(reloaded["result"], last["result"]) {
		t.Fatal("restart changed saved result")
	}
	history := call(http.MethodGet, path+"/matches", "", "", 200)
	var matches []map[string]json.RawMessage
	if json.Unmarshal(history["items"], &matches) != nil || len(matches) != 2 || string(matches[0]["removed_at"]) == "null" || string(matches[1]["removed_at"]) != "null" {
		t.Fatal(history)
	}
}
