//go:build integration && mlintegration

package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/credentials"

	ledgerHandler "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/ledger"
	paymentsHandler "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/payments"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/grpcserver"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/mtls"
)

// Auth is explicitly isolated by httpForecastAuth; financial requests use a real
// API process, API→Core mTLS and the marked disposable PostgreSQL database.
func TestPaymentHTTPAPILifecyclePersistsAudit(t *testing.T) {
	binary := os.Getenv("CORE_API_TEST_BINARY")
	if binary == "" {
		t.Skip("set CORE_API_TEST_BINARY built from this worktree")
	}
	f := newManualBalanceFixture(t)
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
	call := func(method, path, body, key, csrf string, want int) map[string]json.RawMessage {
		t.Helper()
		request, err := http.NewRequestWithContext(t.Context(), method, base+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Origin", "https://localhost")
		request.Header.Set("Content-Type", "application/json")
		if csrf != "" {
			request.Header.Set("X-CSRF-Token", csrf)
		}
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
		if response.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("financial response must not be cached")
		}
		if want == 204 {
			if len(data) != 0 {
				t.Fatal("204 response contains body")
			}
			return nil
		}
		var result map[string]json.RawMessage
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	text := func(result map[string]json.RawMessage, key string) string {
		t.Helper()
		var value string
		if err := json.Unmarshal(result[key], &value); err != nil {
			t.Fatal(key, err)
		}
		return value
	}
	remaining := func(result map[string]json.RawMessage) string {
		t.Helper()
		var value map[string]json.RawMessage
		if err := json.Unmarshal(result["remaining_amount"], &value); err != nil {
			t.Fatal(err)
		}
		return text(value, "amount_minor")
	}
	csrf := strings.Repeat("c", 24)
	company := "/api/v1/companies/" + f.scope.CompanyID.String()
	payments := company + "/payments"
	original := `{"direction":"inflow","amount":{"amount_minor":"100","currency":"RUB"},"scheduled_date":"` + string(f.today) + `","counterparty_name":"Customer","description":"Known receipt"}`
	call(http.MethodPost, payments, original, "http-payment-create", "", 403)
	created := call(http.MethodPost, payments, original, "http-payment-create", csrf, 201)
	payment := payments + "/" + text(created, "id")
	if remaining(created) != "100" || text(created, "version") != "1" {
		t.Fatal(created)
	}
	edit := `{"version":"` + text(created, "version") + `","amount":{"amount_minor":"150","currency":"RUB"}}`
	edited := call(http.MethodPatch, payment, edit, "", csrf, 200)
	if remaining(edited) != "150" {
		t.Fatal(edited)
	}
	call(http.MethodPatch, payment, edit, "", csrf, 409)
	date, err := time.Parse(time.DateOnly, string(f.today))
	if err != nil {
		t.Fatal(err)
	}
	due := date.AddDate(0, 0, 2).Format(time.DateOnly)
	edited = call(http.MethodPatch, payment, `{"version":"`+text(edited, "version")+`","scheduled_date":"`+due+`"}`, "", csrf, 200)
	if text(edited, "scheduled_date") != due {
		t.Fatal(edited)
	}
	factual := call(http.MethodPost, company+"/transactions", `{"operation_date":"`+string(f.today)+`","direction":"inflow","amount":{"amount_minor":"100","currency":"RUB"},"counterparty_name":"Customer","payment_purpose":"Paid receipt","expected_data_version":"4"}`, "http-payment-fact", csrf, 201)
	factID := text(factual, "id")
	matchBody := func(version, amount string) string {
		return `{"transaction_id":"` + factID + `","payment_version":"` + version + `","transaction_version":"` + text(factual, "version") + `","amount":{"amount_minor":"` + amount + `","currency":"RUB"}}`
	}
	firstMatch := call(http.MethodPost, payment+"/matches", matchBody(text(edited, "version"), "40"), "http-payment-match-one", csrf, 201)
	current := call(http.MethodGet, payment, "", "", csrf, 200)
	if remaining(current) != "110" {
		t.Fatal(current)
	}
	call(http.MethodDelete, payment+"/matches/"+text(firstMatch, "id")+"?payment_version="+text(current, "version"), "", "", csrf, 204)
	current = call(http.MethodGet, payment, "", "", csrf, 200)
	if remaining(current) != "150" {
		t.Fatal(current)
	}
	secondBody := matchBody(text(current, "version"), "70")
	secondMatch := call(http.MethodPost, payment+"/matches", secondBody, "http-payment-match-two", csrf, 201)
	current = call(http.MethodGet, payment, "", "", csrf, 200)
	if remaining(current) != "80" {
		t.Fatal(current)
	}
	call(http.MethodPost, payment+"/matches", matchBody(text(current, "version"), "31"), "http-payment-overmatch", csrf, 409)
	call(http.MethodDelete, payment+"?version="+text(current, "version"), "", "", csrf, 204)
	cancelled := call(http.MethodGet, payment, "", "", csrf, 200)
	if text(cancelled, "status") != "cancelled" || remaining(cancelled) != "80" {
		t.Fatal(cancelled)
	}
	call(http.MethodDelete, payment+"/matches/"+text(secondMatch, "id")+"?payment_version="+text(cancelled, "version"), "", "", csrf, 409)
	history := call(http.MethodGet, payment+"/matches", "", "", csrf, 200)
	var matches []map[string]json.RawMessage
	if json.Unmarshal(history["items"], &matches) != nil || len(matches) != 2 || string(matches[0]["removed_at"]) == "null" || string(matches[1]["removed_at"]) != "null" {
		t.Fatal(history)
	}
	replay := call(http.MethodPost, payments, original, "http-payment-create", csrf, 201)
	if text(replay, "id") != text(created, "id") || text(replay, "version") != "1" || remaining(replay) != "100" {
		t.Fatal("create replay returned current state", replay)
	}
	matchedReplay := call(http.MethodPost, payment+"/matches", secondBody, "http-payment-match-two", csrf, 201)
	if text(matchedReplay, "id") != text(secondMatch, "id") {
		t.Fatal("match replay changed allocation")
	}
	call(http.MethodPost, payments, strings.Replace(original, "Known receipt", "Different receipt", 1), "http-payment-create", csrf, 409)
	other := newScope(t, f.app)
	foreign, err := f.app.Payments.CreatePayment(t.Context(), paymentCommand(t, other, 1))
	if err != nil {
		t.Fatal(err)
	}
	call(http.MethodGet, payments+"/"+foreign.PaymentID.String(), "", "", csrf, 404)
	call(http.MethodGet, payments+"/"+uuid.NewString(), "", "", csrf, 404)
	active := call(http.MethodGet, payments, "", "", csrf, 200)
	var activeItems []json.RawMessage
	if json.Unmarshal(active["items"], &activeItems) != nil || len(activeItems) != 0 {
		t.Fatal(active)
	}
	assertCount(t, f.app, "SELECT count(*) FROM core_transactions WHERE company_id=$1", f.scope, 1)
	assertCount(t, f.app, "SELECT count(*) FROM core_payment_matches WHERE company_id=$1", f.scope, 2)
	// Restart the API process and reload the stored payment/history through mTLS.
	stop()
	stop = start()
	reloaded := call(http.MethodGet, payment, "", "", csrf, 200)
	if text(reloaded, "status") != "cancelled" || remaining(reloaded) != "80" || text(reloaded, "version") != text(cancelled, "version") {
		t.Fatal(reloaded)
	}
	history = call(http.MethodGet, payment+"/matches", "", "", csrf, 200)
	if json.Unmarshal(history["items"], &matches) != nil || len(matches) != 2 {
		t.Fatal(history)
	}
}
