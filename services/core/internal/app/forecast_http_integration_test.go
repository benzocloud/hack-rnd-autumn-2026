//go:build integration && mlintegration

package app

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
	authpb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/auth/v1"
	companyHandler "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/company"
	forecastHandler "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/forecasting"
	ledgerHandler "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/grpcserver"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/mtls"
)

// This double isolates the financial HTTP adapter from the unfinished Auth
// deployment. Real Core/SQL/ML and API→Core certificate validation are exercised.
type httpForecastAuth struct{ company string }

func (a httpForecastAuth) AuthorizeCompany(_ context.Context, r *authpb.AuthorizeCompanyRequest) (*authpb.AuthorizeCompanyResponse, error) {
	cookie := r.GetBrowser().GetCookieToken()
	if r.GetWrite() != nil {
		cookie = r.GetWrite().GetBrowser().GetCookieToken()
		if r.GetWrite().CsrfToken != strings.Repeat("c", 24) {
			return nil, status.Error(codes.PermissionDenied, "CSRF denied")
		}
	}
	if cookie != "live-session" || r.CompanyId != a.company {
		return nil, status.Error(codes.PermissionDenied, "scope denied")
	}
	return &authpb.AuthorizeCompanyResponse{ActorId: "auth-owned-user", Role: authpb.CompanyRole_COMPANY_ROLE_OWNER}, nil
}

func loopbackListener(t *testing.T) net.Listener {
	t.Helper()
	l, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l
}

func TestForecastHTTPAPIWithPostgresAndPythonML(t *testing.T) {
	binary := os.Getenv("CORE_API_TEST_BINARY")
	if binary == "" {
		t.Skip("set CORE_API_TEST_BINARY built from the financial API worktree")
	}
	f := newManualBalanceFixture(t)
	cutoff, _ := seedForecastHistory(t, f, true)
	jobs := newForecastJobs(f.app.db, config.Postgres{QueryTimeout: 5 * time.Second}, pythonFinancialModel(t, f))
	auth := httpForecastAuth{company: f.scope.CompanyID.String()}
	tlsConfig, err := mtls.Server(f.ca.Files(t, mtls.Core))
	if err != nil {
		t.Fatal(err)
	}
	server, err := grpcserver.New(credentials.NewTLS(tlsConfig))
	if err != nil {
		t.Fatal(err)
	}
	companyHandler.Register(server, f.app.Companies, auth)
	if err := ledgerHandler.Register(server, f.app.Ledger, f.app.Companies, auth); err != nil {
		t.Fatal(err)
	}
	if err := forecastHandler.Register(server, jobs, f.app.Companies, auth); err != nil {
		t.Fatal(err)
	}
	coreListener := loopbackListener(t)
	go func() { _ = server.Serve(coreListener) }()
	t.Cleanup(server.Stop)
	apiListener := loopbackListener(t)
	address := apiListener.Addr().String()
	_ = apiListener.Close()
	identity := f.ca.Files(t, mtls.API)
	ctx, cancel := context.WithCancel(t.Context())
	command := exec.CommandContext(ctx, binary)
	// Auth RPCs are outside this adapter test. Its financial handlers call Core,
	// whose authorization dependency above is explicitly a test double.
	command.Env = []string{"API_ADDRESS=" + address, "API_ORIGIN=https://localhost", "API_AUTH_ADDRESS=" + coreListener.Addr().String(), "API_AUTH_CA=" + identity.CA, "API_AUTH_CERT=" + identity.Certificate, "API_AUTH_KEY=" + identity.Key, "API_CORE_ADDRESS=" + coreListener.Addr().String(), "API_CORE_SERVER_NAME=saldo-core"}
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
	request := func(method, path, body string, want int) map[string]json.RawMessage {
		t.Helper()
		r, err := http.NewRequestWithContext(t.Context(), method, base+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Origin", "https://localhost")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", strings.Repeat("c", 24))
		r.Header.Set("Idempotency-Key", "http-real-forecast-123")
		r.AddCookie(&http.Cookie{Name: "__Host-saldo_session", Value: "live-session"})
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
		if err != nil || response.StatusCode != want {
			t.Fatalf("%s %s: %d %s %v", method, path, response.StatusCode, data, err)
		}
		var result map[string]json.RawMessage
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	company := "/api/v1/companies/" + f.scope.CompanyID.String()
	request(http.MethodGet, company, "", 200)
	request(http.MethodGet, company+"/account", "", 200)
	body := `{"cutoff_date":"` + string(cutoff) + `","horizon_days":30}`
	accepted := request(http.MethodPost, company+"/forecasts", body, 202)
	var id, statusURL string
	if json.Unmarshal(accepted["id"], &id) != nil || json.Unmarshal(accepted["status_url"], &statusURL) != nil || id == "" {
		t.Fatal(accepted)
	}
	pending := request(http.MethodGet, statusURL, "", 200)
	if string(pending["status"]) != `"queued"` {
		t.Fatal(pending)
	}
	if worked, err := jobs.ProcessOne(t.Context()); err != nil || !worked {
		t.Fatal(worked, err)
	}
	ready := request(http.MethodGet, statusURL, "", 200)
	if string(ready["status"]) != `"succeeded"` || string(ready["result"]) == "null" {
		t.Fatal(ready)
	}
	replayed := request(http.MethodPost, company+"/forecasts", body, 202)
	if string(replayed["id"]) != string(accepted["id"]) {
		t.Fatal("HTTP replay changed job")
	}
	listed := request(http.MethodGet, company+"/forecasts", "", 200)
	var items []map[string]json.RawMessage
	if json.Unmarshal(listed["items"], &items) != nil || len(items) != 1 || string(items[0]["id"]) != string(accepted["id"]) {
		t.Fatal(listed)
	}
}
