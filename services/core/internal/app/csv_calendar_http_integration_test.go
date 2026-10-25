//go:build integration && mlintegration

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
	authpb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/auth/v1"
	forecastHandler "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/forecasting"
	importsHandler "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/imports"
	ledgerHandler "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/ledger"
	paymentsHandler "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/payments"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/grpcserver"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/mtls"
)

// Ordinary CSV upload/preview/commit supplies every fact and EOD without an
// operator manifest or SQL coverage proof. Auth alone is an explicit test double;
// API, Core, PostgreSQL and Python Predict calls use real mTLS services.
func TestOrdinaryCSVCalendarHTTPPostgresPythonLifecycle(t *testing.T) {
	binary := os.Getenv("CORE_API_TEST_BINARY")
	if binary == "" {
		t.Skip("set CORE_API_TEST_BINARY built from this worktree")
	}
	f := newManualBalanceFixture(t)
	today, err := time.Parse(time.DateOnly, string(f.today))
	if err != nil {
		t.Fatal(err)
	}
	cutoffTime := today.AddDate(0, 0, -1)
	cutoff := cutoffTime.Format(time.DateOnly)
	imports, err := newImports(f.app.db, config.Postgres{QueryTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
		defer cancel()
		for _, table := range []string{"core_forecast_jobs", "core_financial_snapshots", "core_payment_matches", "core_expected_payments", "core_payment_reconciliation_groups", "core_transactions", "core_balances", "core_history_coverage", "core_imports"} {
			if _, err := f.app.db.ExecContext(ctx, "DELETE FROM "+table+" WHERE company_id=$1", f.scope.CompanyID); err != nil {
				t.Error(err)
			}
		}
	})
	model := pythonFinancialModel(t, f)
	jobs := newForecastJobs(f.app.db, config.Postgres{QueryTimeout: 5 * time.Second}, model)
	auth := importsHTTPAuth{company: f.scope.CompanyID.String()}
	authTLS, authCA := importsHTTPAuthTLS(t, f)
	authServer := grpc.NewServer(grpc.Creds(credentials.NewTLS(authTLS)))
	authpb.RegisterAuthServiceServer(authServer, auth)
	authListener := importsHTTPListener(t)
	go func() { _ = authServer.Serve(authListener) }()
	t.Cleanup(authServer.Stop)
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
	if err := importsHandler.Register(server, imports, f.app.Companies, auth); err != nil {
		t.Fatal(err)
	}
	listener := loopbackListener(t)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	apiListener := loopbackListener(t)
	address := apiListener.Addr().String()
	_ = apiListener.Close()
	identity := f.ca.Files(t, mtls.API)
	client := &http.Client{Timeout: 5 * time.Second}
	base := "http://" + address
	spool := t.TempDir()
	if err := os.Chmod(spool, 0o700); err != nil {
		t.Fatal(err)
	}
	start := func() func() {
		ctx, cancel := context.WithCancel(t.Context())
		command := exec.CommandContext(ctx, binary)
		command.Env = []string{"API_ADDRESS=" + address, "API_ORIGIN=https://localhost", "API_AUTH_ADDRESS=" + authListener.Addr().String(), "API_AUTH_CA=" + authCA, "API_AUTH_CERT=" + identity.Certificate, "API_AUTH_KEY=" + identity.Key, "API_CORE_ADDRESS=" + listener.Addr().String(), "API_CORE_SERVER_NAME=saldo-core", "API_CORE_CA=" + identity.CA, "API_IMPORT_SPOOL_DIR=" + spool}
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
	callRaw := func(method, path, contentType, key string, body []byte, want int) map[string]json.RawMessage {
		t.Helper()
		request, err := http.NewRequestWithContext(t.Context(), method, base+path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Origin", "https://localhost")
		request.Header.Set("Content-Type", contentType)
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
	call := func(method, path, body, key string, want int) map[string]json.RawMessage {
		t.Helper()
		return callRaw(method, path, "application/json", key, []byte(body), want)
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
	// The two recorded operation days span ninety calendar days. The other 88
	// remain unknown input days; the model's limited mean is checked independently.
	csv := fmt.Sprintf("\ufeffcompany_id;operation_id;operation_date;amount;direction;counterparty_inn;counterparty_name;payment_purpose;balance_eod\nC001;FIRST;%s;90.00;Поступление;0012345678;Клиент;Первая операция;1000.00\nC001;LAST;%s;45.00;Списание;0012345678;Поставщик;Последняя операция;955.00\n", cutoffTime.AddDate(0, 0, -89).Format(time.DateOnly), cutoff)
	var upload bytes.Buffer
	writer := multipart.NewWriter(&upload)
	file, err := writer.CreateFormFile("file", "ordinary.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(file, csv); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{"adapter": "dataset_v1", "source_company_id": "C001", "account_id": f.scope.AccountID.String(), "currency": "RUB"} {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	acceptedImport := callRaw(http.MethodPost, company+"/imports", writer.FormDataContentType(), "csv-calendar-upload", upload.Bytes(), 202)
	if worked, err := imports.ProcessOne(t.Context()); err != nil || !worked {
		t.Fatal("import worker", worked, err)
	}
	importURL := text(acceptedImport, "status_url")
	preview := call(http.MethodGet, importURL, "", "", 200)
	if text(preview, "status") != "preview_ready" {
		t.Fatal(preview)
	}
	previewBody := object(preview["preview"])
	if string(previewBody["history_coverage"]) != "null" {
		t.Fatal("ordinary preview acquired coverage proof", previewBody)
	}
	applied := call(http.MethodPost, importURL+"/commit", `{"version":"`+text(previewBody, "preview_version")+`","expected_data_version":"`+text(previewBody, "base_data_version")+`","acknowledge_warnings":true}`, "csv-calendar-commit", 200)
	if text(applied, "status") != "applied" || string(applied["history_coverage"]) != "null" {
		t.Fatal(applied)
	}
	assertCount(t, f.app, "SELECT count(*) FROM core_transactions WHERE company_id=$1", f.scope, 2)
	assertCount(t, f.app, "SELECT count(*) FROM core_history_coverage WHERE company_id=$1", f.scope, 0)
	forecast := func(key, profile string, residualIn, residualOut int64, knownDirection string, remaining int64, dueDay int, knownID string) map[string]json.RawMessage {
		t.Helper()
		accepted := call(http.MethodPost, company+"/forecasts", `{"cutoff_date":"`+cutoff+`","horizon_days":30}`, key, 202)
		if worked, err := jobs.ProcessOne(t.Context()); err != nil || !worked {
			t.Fatal("forecast worker", worked, err)
		}
		ready := call(http.MethodGet, text(accepted, "status_url"), "", "", 200)
		if text(ready, "status") != "succeeded" {
			t.Fatalf("forecast did not succeed: status=%s error=%s", ready["status"], ready["error"])
		}
		input := object(ready["input"])
		modelContext := object(input["model_context"])
		if text(modelContext, "model_version") != profile || text(modelContext, "configuration_id") != profile || string(input["history_coverage"]) != "null" {
			t.Fatal("wrong frozen profile", input)
		}
		assessment := object(input["history_assessment"])
		if string(assessment["operation_days"]) != "2" {
			t.Fatal("unknown days became recorded days", assessment)
		}
		if profile == "mean_90_csv_v1" {
			if text(input, "calendar_knowledge_mode") != "unavailable" {
				t.Fatal(input)
			}
		} else {
			if text(modelContext, "preprocessing_version") != "csv_observed_operations_calendar_v1" || text(input, "calendar_knowledge_mode") != "current" || text(input, "calendar_as_of_date") != string(f.today) || string(input["no_post_cutoff_movements"]) != "true" {
				t.Fatal(input)
			}
		}
		result := object(ready["result"])
		probability := object(object(result["metrics"])["gap_probability"])
		if string(probability["value"]) != "null" || text(probability, "unavailable_reason") != "not_calibrated" {
			t.Fatal(probability)
		}
		var points []map[string]json.RawMessage
		if json.Unmarshal(result["daily_points"], &points) != nil || len(points) != 30 {
			t.Fatal(result)
		}
		balance := int64(95500)
		for n, point := range points {
			inflow, outflow := residualIn, residualOut
			if n+1 == dueDay {
				if knownDirection == "inflow" {
					inflow += remaining
				} else {
					outflow += remaining
				}
			}
			balance += inflow - outflow
			if minor(point["inflow"]) != inflow || minor(point["outflow"]) != outflow || minor(point["balance"]) != balance || text(point, "date") != cutoffTime.AddDate(0, 0, n+1).Format(time.DateOnly) {
				t.Fatalf("point %d: %s", n+1, point)
			}
		}
		var components, occurrences []map[string]json.RawMessage
		if json.Unmarshal(result["components"], &components) != nil || json.Unmarshal(result["effective_occurrences"], &occurrences) != nil {
			t.Fatal(result)
		}
		count := 0
		for _, component := range components {
			if string(component["kind"]) == `"known_payment"` {
				count++
				if text(component, "payment_id") != knownID || minor(component["remaining_amount"]) != remaining {
					t.Fatal(component)
				}
			}
		}
		if remaining > 0 && (count != 1 || len(occurrences) != 1) || remaining == 0 && (count != 0 || len(occurrences) != 0) {
			t.Fatal("known occurrence count", count, occurrences)
		}
		return ready
	}
	baseline := forecast("csv-calendar-baseline", "mean_90_csv_v1", 100, 50, "inflow", 0, 0, "")
	group := call(http.MethodPost, company+"/reconciliation-groups", `{"cutoff_date":"`+cutoff+`","horizon_start":"`+cutoffTime.AddDate(0, 0, 1).Format(time.DateOnly)+`","horizon_end":"`+cutoffTime.AddDate(0, 0, 30).Format(time.DateOnly)+`","kind":"additional","additional":{"confirmed":true}}`, "csv-calendar-group", 201)
	due := cutoffTime.AddDate(0, 0, 2).Format(time.DateOnly)
	payment := call(http.MethodPost, company+"/payments", `{"direction":"inflow","amount":{"amount_minor":"900","currency":"RUB"},"scheduled_date":"`+due+`","counterparty_name":"Клиент","description":"Дополнительное обязательство","reconciliation_group_id":"`+text(group, "id")+`"}`, "csv-calendar-payment", 201)
	paymentID := text(payment, "id")
	path := company + "/payments/" + paymentID
	const calendarProfile = "mean_90_csv_calendar_v1"
	first := forecast("csv-calendar-first", calendarProfile, 100, 50, "inflow", 900, 2, paymentID)
	var frozenSnapshot string
	snapshotID := text(object(first["input"]), "snapshot_id")
	if err := f.app.db.GetContext(t.Context(), &frozenSnapshot, "SELECT payload::text FROM core_financial_snapshots WHERE company_id=$1 AND id=$2", f.scope.CompanyID, snapshotID); err != nil {
		t.Fatal(err)
	}
	payment = call(http.MethodPatch, path, `{"version":"`+text(payment, "version")+`","amount":{"amount_minor":"1500","currency":"RUB"}}`, "", 200)
	forecast("csv-calendar-amount", calendarProfile, 100, 50, "inflow", 1500, 2, paymentID)
	due = cutoffTime.AddDate(0, 0, 4).Format(time.DateOnly)
	payment = call(http.MethodPatch, path, `{"version":"`+text(payment, "version")+`","scheduled_date":"`+due+`"}`, "", 200)
	forecast("csv-calendar-date", calendarProfile, 100, 50, "inflow", 1500, 4, paymentID)
	facts := call(http.MethodGet, company+"/transactions?direction=inflow&limit=100", "", "", 200)
	var factItems []map[string]json.RawMessage
	if json.Unmarshal(facts["items"], &factItems) != nil || len(factItems) != 1 {
		t.Fatal(facts)
	}
	fact := factItems[0]
	matchBody := func(amount string) string {
		return `{"transaction_id":"` + text(fact, "id") + `","payment_version":"` + text(payment, "version") + `","transaction_version":"` + text(fact, "version") + `","amount":{"amount_minor":"` + amount + `","currency":"RUB"}}`
	}
	matched := call(http.MethodPost, path+"/matches", matchBody("40"), "csv-calendar-match-first", 201)
	payment = call(http.MethodGet, path, "", "", 200)
	forecast("csv-calendar-partial", calendarProfile, 100, 50, "inflow", 1460, 4, paymentID)
	call(http.MethodDelete, path+"/matches/"+text(matched, "id")+"?payment_version="+text(payment, "version"), "", "", 204)
	payment = call(http.MethodGet, path, "", "", 200)
	forecast("csv-calendar-unmatch", calendarProfile, 100, 50, "inflow", 1500, 4, paymentID)
	call(http.MethodPost, path+"/matches", matchBody("70"), "csv-calendar-match-second", 201)
	payment = call(http.MethodGet, path, "", "", 200)
	forecast("csv-calendar-rematch", calendarProfile, 100, 50, "inflow", 1430, 4, paymentID)
	call(http.MethodDelete, path+"?version="+text(payment, "version"), "", "", 204)
	forecast("csv-calendar-cancel", calendarProfile, 100, 50, "inflow", 0, 4, "")
	// Replacement mapping survives a full match, date outside the horizon and
	// cancellation. Only explicit group revocation restores the historical mean.
	outflows := call(http.MethodGet, company+"/transactions?direction=outflow&limit=100", "", "", 200)
	if json.Unmarshal(outflows["items"], &factItems) != nil || len(factItems) != 1 {
		t.Fatal(outflows)
	}
	outflowFact := factItems[0]
	replacement := call(http.MethodPost, company+"/reconciliation-groups", `{"cutoff_date":"`+cutoff+`","horizon_start":"`+cutoffTime.AddDate(0, 0, 1).Format(time.DateOnly)+`","horizon_end":"`+cutoffTime.AddDate(0, 0, 30).Format(time.DateOnly)+`","kind":"replacement","replacement":{"complete_for_horizon":true,"historical_operations":[{"transaction_id":"`+text(outflowFact, "id")+`","version":"`+text(outflowFact, "version")+`"}]}}`, "csv-calendar-replacement", 201)
	replacementID := text(replacement, "id")
	createReplacement := func(key string, day int) map[string]json.RawMessage {
		t.Helper()
		return call(http.MethodPost, company+"/payments", `{"direction":"outflow","amount":{"amount_minor":"4500","currency":"RUB"},"scheduled_date":"`+cutoffTime.AddDate(0, 0, day).Format(time.DateOnly)+`","counterparty_name":"Поставщик","description":"Полная замена потока","reconciliation_group_id":"`+replacementID+`"}`, key, 201)
	}
	replacementPayment := createReplacement("csv-calendar-replacement-payment", 5)
	replacementPaymentID := text(replacementPayment, "id")
	replacementPath := company + "/payments/" + replacementPaymentID
	forecast("csv-calendar-replacement-forecast", calendarProfile, 100, 0, "outflow", 4500, 5, replacementPaymentID)
	call(http.MethodPost, replacementPath+"/matches", `{"transaction_id":"`+text(outflowFact, "id")+`","payment_version":"`+text(replacementPayment, "version")+`","transaction_version":"`+text(outflowFact, "version")+`","amount":{"amount_minor":"4500","currency":"RUB"}}`, "csv-calendar-full-match", 201)
	forecast("csv-calendar-full-match-forecast", calendarProfile, 100, 0, "outflow", 0, 5, "")
	outside := createReplacement("csv-calendar-outside-payment", 31)
	forecast("csv-calendar-outside-forecast", calendarProfile, 100, 0, "outflow", 0, 5, "")
	outsidePath := company + "/payments/" + text(outside, "id")
	inside := call(http.MethodPatch, outsidePath, `{"version":"`+text(outside, "version")+`","scheduled_date":"`+cutoffTime.AddDate(0, 0, 5).Format(time.DateOnly)+`"}`, "", 200)
	call(http.MethodDelete, outsidePath+"?version="+text(inside, "version"), "", "", 204)
	forecast("csv-calendar-replacement-cancel", calendarProfile, 100, 0, "outflow", 0, 5, "")
	call(http.MethodPost, company+"/reconciliation-groups/"+replacementID+"/revoke", `{"version":"`+text(replacement, "version")+`"}`, "csv-calendar-revoke", 200)
	last := forecast("csv-calendar-revoked-forecast", calendarProfile, 100, 50, "inflow", 0, 0, "")
	for _, original := range []map[string]json.RawMessage{baseline, first} {
		old := call(http.MethodGet, company+"/forecasts/"+text(original, "id"), "", "", 200)
		if string(old["is_stale"]) != "true" || !bytes.Equal(old["result"], original["result"]) || !bytes.Equal(old["input"], original["input"]) {
			t.Fatal("old forecast changed", old)
		}
	}
	var afterSnapshot string
	if err := f.app.db.GetContext(t.Context(), &afterSnapshot, "SELECT payload::text FROM core_financial_snapshots WHERE company_id=$1 AND id=$2", f.scope.CompanyID, snapshotID); err != nil || afterSnapshot != frozenSnapshot {
		t.Fatal("frozen snapshot changed", err)
	}
	assertCount(t, f.app, "SELECT count(*) FROM core_transactions WHERE company_id=$1", f.scope, 2)
	assertCount(t, f.app, "SELECT count(*) FROM core_history_coverage WHERE company_id=$1", f.scope, 0)

	// Recreate the Core application and job reader against the stored schema;
	// this read cannot rely on the creating worker's in-memory calendar.
	reopened, _ := testApplication(t)
	restartedJobs := newForecastJobs(reopened.db, config.Postgres{QueryTimeout: 5 * time.Second}, model)
	lastID, err := uuid.Parse(text(last, "id"))
	if err != nil {
		t.Fatal(err)
	}
	stored, err := restartedJobs.Read(t.Context(), f.scope, lastID)
	if err != nil || stored.Status != "succeeded" || stored.Model.Version != calendarProfile || stored.Result == nil || len(stored.Result.Days) != 30 || stored.Result.Days[29].Balance != 97000 {
		t.Fatal("Core restart failed to decode CSV calendar result", stored, err)
	}
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
