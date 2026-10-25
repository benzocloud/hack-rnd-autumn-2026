//go:build integration

package app

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
	ledgerdomain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	authpb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/auth/v1"
	forecastHandler "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/forecasting"
	importsHandler "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/imports"
	ledgerHandler "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/testutil/pki"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/grpcserver"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/mtls"
)

// Auth and the deterministic predictor are test doubles. HTTP API, Core
// transport/import parser, snapshot/jobs and PostgreSQL are real; this test
// does not claim a real Auth login, browser interaction or Python ML inference.
type importsHTTPAuth struct {
	authpb.UnimplementedAuthServiceServer
	company string
}

func (a importsHTTPAuth) AuthorizeCompany(ctx context.Context, r *authpb.AuthorizeCompanyRequest) (*authpb.AuthorizeCompanyResponse, error) {
	if identity := mtls.Identity(ctx); identity != mtls.API && identity != mtls.Core {
		return nil, status.Error(codes.PermissionDenied, "unexpected service identity")
	}
	cookie := r.GetBrowser().GetCookieToken()
	if write := r.GetWrite(); write != nil {
		cookie = write.GetBrowser().GetCookieToken()
		if write.CsrfToken != strings.Repeat("c", 24) {
			return nil, status.Error(codes.PermissionDenied, "CSRF denied")
		}
	}
	if cookie != "live-session" || r.CompanyId != a.company {
		return nil, status.Error(codes.PermissionDenied, "scope denied")
	}
	return &authpb.AuthorizeCompanyResponse{ActorId: "auth-owned-user", Role: authpb.CompanyRole_COMPANY_ROLE_OWNER}, nil
}

func importsHTTPListener(t *testing.T) net.Listener {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}

func importsHTTPAuthTLS(t *testing.T, f *manualBalanceFixture) (*tls.Config, string) {
	t.Helper()
	// API Auth dialing verifies its actual target hostname. Core's shared PKI
	// helper provides service DNS names only; this test server also needs IP SAN.
	config := pki.Server(t, f.ca.Files(t, mtls.Auth))
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := url.Parse(mtls.Auth)
	if err != nil {
		t.Fatal(err)
	}
	certificate := &x509.Certificate{SerialNumber: big.NewInt(1), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, URIs: []*url.URL{identity},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, certificate, certificate, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	config.Certificates = []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}
	root := filepath.Join(t.TempDir(), "auth-test-ca.pem")
	if err := os.WriteFile(root, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	return config, root
}

func TestImportsHTTPAPIWithPostgres(t *testing.T) {
	binary := os.Getenv("CORE_API_TEST_BINARY")
	if binary == "" {
		t.Skip("set CORE_API_TEST_BINARY built from current API sources")
	}
	f := newManualBalanceFixture(t)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
		defer cancel()
		for _, table := range []string{"core_forecast_jobs", "core_financial_snapshots", "core_transactions", "core_history_coverage", "core_imports"} {
			if _, err := f.app.db.ExecContext(ctx, "DELETE FROM "+table+" WHERE company_id=$1", f.scope.CompanyID); err != nil {
				t.Error(err)
			}
		}
	})
	// Ninety facts on consecutive days make the coverage assumption explicit;
	// no SQL inserts supply history, balance, source binding or proof.
	start := time.Date(2026, time.July, 3, 0, 0, 0, 0, time.UTC)
	cutoff := start.AddDate(0, 0, 89).Format(time.DateOnly)
	var csvBuilder strings.Builder
	csvBuilder.WriteString("\ufeffcompany_id;operation_id;operation_date;amount;direction;counterparty_inn;counterparty_name;payment_purpose;balance_eod\n")
	for day := range 90 {
		date := start.AddDate(0, 0, day).Format(time.DateOnly)
		balance := 10000 + (day+1)*100
		_, _ = fmt.Fprintf(&csvBuilder, "C001;op%d;%s;1.00;Поступление;0012345678;Контрагент;Выручка;%d.00\n", day+1, date, balance/100)
	}
	csvBuilder.WriteString("C002;other;2026-09-30;3.00;Поступление;;Other;Other;3.00\n")
	csv := csvBuilder.String()
	digest := sha256.Sum256([]byte(csv))
	manifest := config.FixtureCoverage{CompanyID: f.scope.CompanyID.String(), AccountID: f.scope.AccountID.String(), FileSHA256: hex.EncodeToString(digest[:]), SourceCompanyID: "C001", FromDate: start.Format(time.DateOnly), ThroughDate: cutoff, CutoffDate: cutoff, CutoffBalanceMinor: 19000, CutoffRowNumbers: []int64{91}}
	imports, err := newImports(f.app.db, config.Postgres{QueryTimeout: 5 * time.Second}, manifest)
	if err != nil {
		t.Fatal(err)
	}
	var predictedInput *forecasting.FinancialModelInput
	model := forecastModelFunc(func(ctx context.Context, input forecasting.FinancialModelInput) (forecasting.FinancialModelResult, error) {
		captured := input
		predictedInput = &captured
		result, err := deterministicForecast(ctx, input)
		if input.Assessment != nil {
			result.Limitations = append([]string{}, input.Assessment.Limitations...)
		}
		return result, err
	})
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
	coreServer, err := grpcserver.New(credentials.NewTLS(tlsConfig))
	if err != nil {
		t.Fatal(err)
	}
	if err := ledgerHandler.Register(coreServer, f.app.Ledger, f.app.Companies, auth); err != nil {
		t.Fatal(err)
	}
	if err := importsHandler.Register(coreServer, imports, f.app.Companies, auth); err != nil {
		t.Fatal(err)
	}
	if err := forecastHandler.Register(coreServer, jobs, f.app.Companies, auth); err != nil {
		t.Fatal(err)
	}
	coreListener := importsHTTPListener(t)
	go func() { _ = coreServer.Serve(coreListener) }()
	t.Cleanup(coreServer.Stop)
	apiListener := importsHTTPListener(t)
	address := apiListener.Addr().String()
	_ = apiListener.Close()
	identity := f.ca.Files(t, mtls.API)
	spool := t.TempDir()
	if err := os.Chmod(spool, 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	command := exec.CommandContext(ctx, binary)
	command.Env = []string{
		"API_ADDRESS=" + address, "API_ORIGIN=https://localhost", "API_AUTH_ADDRESS=" + authListener.Addr().String(),
		"API_AUTH_CA=" + authCA, "API_AUTH_CERT=" + identity.Certificate, "API_AUTH_KEY=" + identity.Key,
		"API_CORE_ADDRESS=" + coreListener.Addr().String(), "API_CORE_SERVER_NAME=saldo-core", "API_CORE_CA=" + identity.CA, "API_IMPORT_SPOOL_DIR=" + spool,
	}
	var apiOutput bytes.Buffer
	command.Stdout, command.Stderr = &apiOutput, &apiOutput
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = command.Wait() })
	client := &http.Client{Timeout: 5 * time.Second}
	base := "http://" + address
	deadline := time.Now().Add(5 * time.Second)
	for {
		response, err := client.Get(base + "/health/live")
		if err == nil {
			_ = response.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("API did not listen", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	company := "/api/v1/companies/" + f.scope.CompanyID.String()
	call := func(method, path, contentType, key string, body []byte, want int) map[string]json.RawMessage {
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
		var result map[string]json.RawMessage
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", "statement.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(file, csv); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"adapter": "dataset_v1", "source_company_id": "C001", "account_id": f.scope.AccountID.String(), "currency": "RUB"} {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	accepted := call(http.MethodPost, company+"/imports", writer.FormDataContentType(), "http-import-123", body.Bytes(), http.StatusAccepted)
	var id, statusURL string
	if json.Unmarshal(accepted["id"], &id) != nil || json.Unmarshal(accepted["status_url"], &statusURL) != nil || id == "" {
		t.Fatal("missing durable acceptance", accepted)
	}
	queued := call(http.MethodGet, statusURL, "", "", nil, http.StatusOK)
	if string(queued["status"]) != `"queued"` {
		t.Fatal("upload applied instead of queuing", queued)
	}
	// A fresh service instance reads the accepted job/chunks from PostgreSQL.
	restarted, err := newImports(f.app.db, config.Postgres{QueryTimeout: 5 * time.Second}, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if worked, err := restarted.ProcessOne(t.Context()); err != nil || !worked {
		t.Fatal("durable worker did not process accepted input", worked, err)
	}
	preview := call(http.MethodGet, statusURL, "", "", nil, http.StatusOK)
	if string(preview["status"]) != `"preview_ready"` {
		t.Fatal("preview did not survive service reconstruction", preview)
	}
	var details struct {
		Rows     int64 `json:"row_count"`
		Excluded int64 `json:"excluded_other_company_row_count"`
		New      int64 `json:"new_transaction_count"`
		Warnings int64 `json:"warning_count"`
		Blockers int64 `json:"blocking_issue_count"`
	}
	if json.Unmarshal(preview["preview"], &details) != nil || details.Rows != 91 || details.Excluded != 1 || details.New != 90 || details.Warnings < 1 || details.Blockers != 0 {
		t.Fatal("incorrect CSV preview", string(preview["preview"]))
	}
	var previewCoverage struct {
		HistoryCoverage *struct {
			Kind        string `json:"kind"`
			DataVersion string `json:"data_version"`
		} `json:"history_coverage"`
	}
	if json.Unmarshal(preview["preview"], &previewCoverage) != nil || previewCoverage.HistoryCoverage == nil || previewCoverage.HistoryCoverage.Kind != "dataset_fixture_assumption" || previewCoverage.HistoryCoverage.DataVersion != "1" {
		t.Fatal("preview hid its fixture coverage proposal", preview)
	}
	replay := call(http.MethodPost, company+"/imports", writer.FormDataContentType(), "http-import-123", body.Bytes(), http.StatusAccepted)
	if string(replay["id"]) != string(accepted["id"]) {
		t.Fatal("replay created a second import", replay)
	}
	conflicting := bytes.Replace(body.Bytes(), []byte("1.00"), []byte("1.01"), 1)
	call(http.MethodPost, company+"/imports", writer.FormDataContentType(), "http-import-123", conflicting, http.StatusConflict)
	listed := call(http.MethodGet, company+"/imports?limit=1", "", "", nil, http.StatusOK)
	var items []json.RawMessage
	if json.Unmarshal(listed["items"], &items) != nil || len(items) != 1 || string(listed["next_cursor"]) != "null" {
		t.Fatal("import list did not return the durable job", listed)
	}
	issueCount, cursor := int64(0), ""
	for {
		issues := call(http.MethodGet, statusURL+"/issues?limit=1&cursor="+url.QueryEscape(cursor), "", "", nil, http.StatusOK)
		if json.Unmarshal(issues["items"], &items) != nil || len(items) != 1 {
			t.Fatal("preview warnings unavailable", issues)
		}
		issueCount += int64(len(items))
		if string(issues["next_cursor"]) == "null" {
			break
		}
		var next string
		if json.Unmarshal(issues["next_cursor"], &next) != nil || next == "" || next == cursor || issueCount > details.Warnings {
			t.Fatal("invalid issue pagination", issues)
		}
		cursor = next
	}
	if issueCount != details.Warnings {
		t.Fatal("not all preview warnings were returned", issueCount, details.Warnings)
	}
	for _, table := range []string{"core_transactions", "core_balances", "core_source_bindings"} {
		var count int
		if err := f.app.db.GetContext(t.Context(), &count, "SELECT count(*) FROM "+table+" WHERE company_id=$1", f.scope.CompanyID); err != nil || count != 0 {
			t.Fatal("preview mutated financial state", table, count, err)
		}
	}
	var revision int64
	if err := f.app.db.GetContext(t.Context(), &revision, "SELECT data_version FROM core_accounts WHERE id=$1", f.scope.AccountID); err != nil || revision != 1 {
		t.Fatal("preview changed account revision", revision, err)
	}
	var bound bool
	if err := f.app.db.GetContext(t.Context(), &bound, "SELECT factual_kind IS NOT NULL OR source_system IS NOT NULL OR source_company_id IS NOT NULL FROM core_accounts WHERE id=$1", f.scope.AccountID); err != nil || bound {
		t.Fatal("preview acquired the factual source binding", bound, err)
	}
	entries, err := os.ReadDir(spool)
	if err != nil || len(entries) != 0 {
		t.Fatal("API spool was not cleaned", len(entries), err)
	}
	// Commit the same preview through HTTP. All durable financial state and
	// coverage proof must originate from this authorized import use case.
	var previewBody struct {
		Version     string `json:"preview_version"`
		DataVersion string `json:"base_data_version"`
		CanCommit   bool   `json:"can_commit"`
	}
	if json.Unmarshal(preview["preview"], &previewBody) != nil || !previewBody.CanCommit {
		t.Fatal("valid import cannot commit", preview)
	}
	commitBody, err := json.Marshal(map[string]any{"version": previewBody.Version, "expected_data_version": previewBody.DataVersion, "acknowledge_warnings": true})
	if err != nil {
		t.Fatal(err)
	}
	applied := call(http.MethodPost, statusURL+"/commit", "application/json", "http-commit-123", commitBody, http.StatusOK)
	if string(applied["status"]) != `"applied"` {
		t.Fatal(applied)
	}
	var appliedCoverage struct {
		AppliedDataVersion string `json:"applied_data_version"`
		HistoryCoverage    *struct {
			Kind        string `json:"kind"`
			DataVersion string `json:"data_version"`
		} `json:"history_coverage"`
		Preview struct {
			HistoryCoverage *struct {
				Kind        string `json:"kind"`
				DataVersion string `json:"data_version"`
			} `json:"history_coverage"`
		} `json:"preview"`
	}
	if json.Unmarshal(applied["history_coverage"], &appliedCoverage.HistoryCoverage) != nil || appliedCoverage.HistoryCoverage == nil || appliedCoverage.HistoryCoverage.Kind != "dataset_fixture_assumption" || json.Unmarshal(applied["applied_data_version"], &appliedCoverage.AppliedDataVersion) != nil || json.Unmarshal(applied["preview"], &appliedCoverage.Preview) != nil || appliedCoverage.Preview.HistoryCoverage == nil || appliedCoverage.Preview.HistoryCoverage.DataVersion != appliedCoverage.AppliedDataVersion {
		t.Fatal("applied coverage was not returned through import and preview", applied)
	}
	repeatedCommit := call(http.MethodPost, statusURL+"/commit", "application/json", "http-commit-123", commitBody, http.StatusOK)
	if string(repeatedCommit["applied_data_version"]) != string(applied["applied_data_version"]) {
		t.Fatal("commit replay changed revision", repeatedCommit)
	}
	categories := call(http.MethodGet, company+"/categories?limit=100", "", "", nil, http.StatusOK)
	if json.Unmarshal(categories["items"], &items) != nil || len(items) == 0 {
		t.Fatal("import did not seed readable categories", categories)
	}
	transactions := call(http.MethodGet, company+"/transactions?limit=100&from=2026-07-03&to=2026-09-30", "", "", nil, http.StatusOK)
	if json.Unmarshal(transactions["items"], &items) != nil || len(items) != 90 {
		t.Fatal("committed facts not readable", transactions)
	}
	for _, item := range items {
		var transaction struct {
			Source struct {
				Kind   string `json:"kind"`
				System string `json:"source_system"`
				Import string `json:"import_id"`
			} `json:"source"`
		}
		if json.Unmarshal(item, &transaction) != nil || transaction.Source.Kind != "file" || transaction.Source.System != "dataset_v1" || transaction.Source.Import != id {
			t.Fatal("lost imported provenance", string(item))
		}
	}
	var firstTransaction struct {
		ID      string `json:"id"`
		Version string `json:"version"`
	}
	if len(items) == 0 || json.Unmarshal(items[0], &firstTransaction) != nil || firstTransaction.ID == "" || firstTransaction.Version != "1" {
		t.Fatal("imported operation version is unavailable", items)
	}
	var categoryItems []struct {
		ID           string `json:"id"`
		SemanticKind string `json:"semantic_kind"`
	}
	if json.Unmarshal(categories["items"], &categoryItems) != nil {
		t.Fatal("category catalogue is malformed", categories)
	}
	var salesCategory string
	for _, category := range categoryItems {
		if category.SemanticKind == "sales_receipt" {
			salesCategory = category.ID
			break
		}
	}
	if salesCategory == "" {
		t.Fatal("import did not seed the sales category", categories)
	}
	categoryURL := company + "/transactions/" + firstTransaction.ID + "/category"
	categoryBody, err := json.Marshal(map[string]any{"version": firstTransaction.Version, "category_id": salesCategory})
	if err != nil {
		t.Fatal(err)
	}
	assigned := call(http.MethodPatch, categoryURL, "application/json", "http-category-123", categoryBody, http.StatusOK)
	var assignedTransaction struct {
		Version    string `json:"version"`
		CategoryID string `json:"category_id"`
	}
	if json.Unmarshal(assigned["version"], &assignedTransaction.Version) != nil || json.Unmarshal(assigned["category_id"], &assignedTransaction.CategoryID) != nil || assignedTransaction.Version != "2" || assignedTransaction.CategoryID != salesCategory {
		t.Fatal("category update was not applied", assigned)
	}
	noOpBody, err := json.Marshal(map[string]any{"version": assignedTransaction.Version, "category_id": salesCategory})
	if err != nil {
		t.Fatal(err)
	}
	noOp := call(http.MethodPatch, categoryURL, "application/json", "http-category-noop-123", noOpBody, http.StatusOK)
	if string(noOp["version"]) != `"2"` {
		t.Fatal("setting the same category changed the transaction", noOp)
	}
	clearBody, err := json.Marshal(map[string]any{"version": assignedTransaction.Version, "category_id": nil})
	if err != nil {
		t.Fatal(err)
	}
	cleared := call(http.MethodPatch, categoryURL, "application/json", "http-category-clear-123", clearBody, http.StatusOK)
	if string(cleared["version"]) != `"3"` || string(cleared["category_id"]) != "null" {
		t.Fatal("category clear was not applied", cleared)
	}
	var accountVersion int64
	if err := f.app.db.GetContext(t.Context(), &accountVersion, "SELECT data_version FROM core_accounts WHERE company_id=$1 AND id=$2", f.scope.CompanyID, f.scope.AccountID); err != nil || accountVersion != 4 {
		t.Fatal("category mutations changed the wrong account revision", accountVersion, err)
	}
	forecastBody, err := json.Marshal(map[string]any{"cutoff_date": cutoff, "horizon_days": 30})
	if err != nil {
		t.Fatal(err)
	}
	forecastAccepted := call(http.MethodPost, company+"/forecasts", "application/json", "http-forecast-123", forecastBody, http.StatusAccepted)
	var forecastURL string
	if json.Unmarshal(forecastAccepted["status_url"], &forecastURL) != nil || forecastURL == "" {
		t.Fatal(forecastAccepted)
	}
	forecastQueued := call(http.MethodGet, forecastURL, "", "", nil, http.StatusOK)
	if string(forecastQueued["status"]) != `"queued"` {
		t.Fatal(forecastQueued)
	}
	// Reconstruct the worker around the same durable database, not the saved
	// in-process snapshot, and verify the imported facts reach the predictor.
	restartedJobs := newForecastJobs(f.app.db, config.Postgres{QueryTimeout: 5 * time.Second}, model)
	if worked, err := restartedJobs.ProcessOne(t.Context()); err != nil || !worked {
		t.Fatal(worked, err)
	}
	if predictedInput == nil || len(predictedInput.Operations) != 90 || len(predictedInput.Snapshot.History) != 90 || predictedInput.Coverage.Kind != "dataset_fixture_assumption" || predictedInput.Coverage.Reference != "dataset_fixture_assumption:"+manifest.FileSHA256 || predictedInput.Coverage.From != ledgerdomain.Date(manifest.FromDate) || predictedInput.Coverage.Through != ledgerdomain.Date(cutoff) || !slices.Contains(predictedInput.Coverage.Limitations, "dataset_coverage_assumed_not_bank_certified") {
		t.Fatal("predictor did not receive imported facts/fixture proof", predictedInput)
	}
	seenImportedIDs := make(map[string]bool)
	for _, operation := range predictedInput.Operations {
		source := operation.Data().Source
		if source.Kind != ledgerdomain.Imported || source.Namespace.System != "dataset_v1" || source.Namespace.CompanyID != "C001" || !strings.HasPrefix(source.ExternalID, "op") || seenImportedIDs[source.ExternalID] {
			t.Fatal("predictor received a fact outside the imported source", operation.Data())
		}
		seenImportedIDs[source.ExternalID] = true
	}
	ready := call(http.MethodGet, forecastURL, "", "", nil, http.StatusOK)
	reloaded := call(http.MethodGet, forecastURL, "", "", nil, http.StatusOK)
	if string(ready["status"]) != `"succeeded"` || !bytes.Equal(ready["result"], reloaded["result"]) {
		t.Fatal("stored forecast did not reload", ready, reloaded)
	}
	var forecastInput struct {
		Coverage struct {
			Kind        string   `json:"kind"`
			Reference   string   `json:"reference"`
			Version     string   `json:"data_version"`
			Limitations []string `json:"limitations"`
		} `json:"history_coverage"`
	}
	if json.Unmarshal(ready["input"], &forecastInput) != nil || forecastInput.Coverage.Kind != "dataset_fixture_assumption" || forecastInput.Coverage.Reference != "dataset_fixture_assumption:"+manifest.FileSHA256 || forecastInput.Coverage.Version != strconv.FormatInt(accountVersion, 10) || !slices.Contains(forecastInput.Coverage.Limitations, "dataset_coverage_assumed_not_bank_certified") {
		t.Fatal("HTTP lost typed coverage", ready)
	}
	var readiness struct {
		Status      string   `json:"status"`
		Limitations []string `json:"limitations"`
	}
	if json.Unmarshal(ready["readiness"], &readiness) != nil || readiness.Status != "limited" || !slices.Contains(readiness.Limitations, "dataset_coverage_assumed_not_bank_certified") {
		t.Fatal("fixture was presented as certified", ready)
	}
	// Simulate an existing import with no coverage. Reevaluation through the real
	// HTTP adapter must select the CSV profile without inserting a proof or facts.
	if _, err := f.app.db.ExecContext(t.Context(), "DELETE FROM core_history_coverage WHERE company_id=$1 AND account_id=$2", f.scope.CompanyID, f.scope.AccountID); err != nil {
		t.Fatal(err)
	}
	csvAccepted := call(http.MethodPost, company+"/forecasts", "application/json", "http-csv-forecast-123", forecastBody, http.StatusAccepted)
	var csvURL string
	if json.Unmarshal(csvAccepted["status_url"], &csvURL) != nil || csvURL == "" {
		t.Fatal(csvAccepted)
	}
	if worked, err := restartedJobs.ProcessOne(t.Context()); err != nil || !worked {
		t.Fatal(worked, err)
	}
	csvReady := call(http.MethodGet, csvURL, "", "", nil, http.StatusOK)
	csvReload := call(http.MethodGet, csvURL, "", "", nil, http.StatusOK)
	var csvInput struct {
		Coverage   any `json:"history_coverage"`
		Assessment *struct {
			RuleVersion string `json:"rule_version"`
			Version     string `json:"data_version"`
		} `json:"history_assessment"`
	}
	if string(csvReady["status"]) != `"succeeded"` || !bytes.Equal(csvReady["result"], csvReload["result"]) || json.Unmarshal(csvReady["input"], &csvInput) != nil || csvInput.Coverage != nil || csvInput.Assessment == nil || csvInput.Assessment.RuleVersion != "csv_history_quality_v1" || csvInput.Assessment.Version != strconv.FormatInt(accountVersion, 10) {
		t.Fatal("CSV HTTP projection lost its assessment", csvReady)
	}
	if predictedInput == nil || predictedInput.Assessment == nil || predictedInput.Snapshot.Model.Version != forecasting.CSVModelVersion || len(predictedInput.Snapshot.History) != 0 {
		t.Fatal("CSV input was relabeled covered", predictedInput)
	}
	assertCount(t, f.app, "SELECT count(*) FROM core_history_coverage WHERE company_id=$1", f.scope, 0)
	assertCount(t, f.app, "SELECT count(*) FROM core_transactions WHERE company_id=$1", f.scope, 90)
	assertRevision(t, f.app, f.scope, accountVersion)
	// Manual HTTP creation is permitted on this imported account, with the
	// original external namespace retained and an immutable replay response.
	duplicateBody, err := json.Marshal(map[string]any{"operation_date": start.Format(time.DateOnly), "direction": "inflow", "amount": map[string]string{"amount_minor": "100", "currency": "RUB"}, "counterparty_inn": "0012345678", "counterparty_name": "Контрагент", "payment_purpose": "Выручка", "expected_data_version": strconv.FormatInt(accountVersion, 10)})
	if err != nil {
		t.Fatal(err)
	}
	duplicate := call(http.MethodPost, company+"/transactions", "application/json", "http-manual-duplicate-123", duplicateBody, http.StatusConflict)
	if !bytes.Contains(duplicate["error"], []byte("TRANSACTION_DUPLICATE_SUSPECTED")) {
		t.Fatal("import duplicate not explained", duplicate)
	}
	manualBody, err := json.Marshal(map[string]any{"operation_date": start.AddDate(0, 0, 90).Format(time.DateOnly), "direction": "inflow", "amount": map[string]string{"amount_minor": "180", "currency": "RUB"}, "counterparty_name": "Manual client", "payment_purpose": "Manual supplement", "category_id": nil, "expected_data_version": strconv.FormatInt(accountVersion, 10)})
	if err != nil {
		t.Fatal(err)
	}
	manualCreated := call(http.MethodPost, company+"/transactions", "application/json", "http-manual-create-123", manualBody, http.StatusCreated)
	manualReplay := call(http.MethodPost, company+"/transactions", "application/json", "http-manual-create-123", manualBody, http.StatusCreated)
	if !bytes.Equal(manualCreated["id"], manualReplay["id"]) || !bytes.Equal(manualCreated["created_at"], manualReplay["created_at"]) || !bytes.Contains(manualCreated["source"], []byte(`"manual"`)) {
		t.Fatal("manual replay/provenance changed", manualCreated, manualReplay)
	}
	assertRevision(t, f.app, f.scope, accountVersion+1)
	assertCount(t, f.app, "SELECT count(*) FROM core_accounts WHERE company_id=$1 AND factual_kind='imported' AND source_company_id='C001'", f.scope, 1)
	assertCount(t, f.app, "SELECT count(*) FROM core_transactions WHERE company_id=$1", f.scope, 91)
	staleCSV := call(http.MethodGet, csvURL, "", "", nil, http.StatusOK)
	if string(staleCSV["is_stale"]) != "true" {
		t.Fatal("manual fact did not stale old forecast", staleCSV)
	}
	// A second uncommitted job independently exercises expiry. The already
	// applied import/forecast must remain unaffected by its cleanup.
	expiring := call(http.MethodPost, company+"/imports", writer.FormDataContentType(), "http-expiring-123", body.Bytes(), http.StatusAccepted)
	var expiringID string
	if json.Unmarshal(expiring["id"], &expiringID) != nil {
		t.Fatal(expiring)
	}
	if _, err := restarted.ProcessOne(t.Context()); err != nil {
		t.Fatal(err)
	}
	statusURL = company + "/imports/" + expiringID
	// Advance only this fixture's durable expiry, not OS/business clocks. The
	// actual worker cleanup must transition it and HTTP must omit failed preview.
	expired := time.Now().UTC().Add(-time.Minute)
	if _, err := f.app.db.ExecContext(t.Context(), "UPDATE core_imports SET expires_at=$2,payload=jsonb_set(payload,'{ExpiresAt}',to_jsonb($2::timestamptz)) WHERE id=$1", expiringID, expired); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.ProcessOne(t.Context()); err != nil {
		t.Fatal("expiry cleanup failed", err)
	}
	failed := call(http.MethodGet, statusURL, "", "", nil, http.StatusOK)
	var operationError struct {
		Code string `json:"code"`
	}
	if string(failed["status"]) != `"failed"` || string(failed["preview"]) != "null" || json.Unmarshal(failed["error"], &operationError) != nil || operationError.Code != "IMPORT_STATE_CONFLICT" {
		t.Fatal("expired preview was not represented as a failed import", failed)
	}
}
