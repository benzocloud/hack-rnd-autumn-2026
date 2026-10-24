package integration

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	clientauth "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/client/auth"
	authv1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/auth/v1"
	httpapi "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/handler/http"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/securetls"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/testauth"
)

type fixture struct {
	auth      *testauth.Server
	http      *httptest.Server
	client    *http.Client
	csrf      string
	grpc      *grpc.Server
	cancelled chan string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	pki, err := testauth.GeneratePKI(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tlsConfig, err := securetls.Load(pki.CA, pki.AuthCert, pki.AuthKey, "spiffe://saldo.local/api", true)
	if err != nil {
		t.Fatal(err)
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cancelled := make(chan string, 10)
	srv := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConfig)), grpc.UnaryInterceptor(func(ctx context.Context, request any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		response, callErr := next(ctx, request)
		if ctx.Err() != nil {
			cancelled <- info.FullMethod
		}
		return response, callErr
	}))
	f := &fixture{auth: testauth.New(), grpc: srv, cancelled: cancelled}
	authv1.RegisterAuthServiceServer(srv, f.auth)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(func() { srv.Stop(); _ = lis.Close() })
	client, conn, err := clientauth.Dial(lis.Addr().String(), pki.CA, pki.APICert, pki.APIKey, 150*time.Millisecond, 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	f.http = httptest.NewUnstartedServer(nil)
	origin := "https://" + f.http.Listener.Addr().String()
	f.http.Config.Handler = httpapi.New(client, origin, slog.New(slog.DiscardHandler), func(context.Context) bool { return true })
	cert, err := tls.LoadX509KeyPair(pki.HTTPCert, pki.HTTPKey)
	if err != nil {
		t.Fatal(err)
	}
	f.http.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}}
	f.http.StartTLS()
	t.Cleanup(f.http.Close)
	f.client = f.http.Client()
	f.client.Timeout = 2 * time.Second
	f.client.Jar, err = cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	f.refresh(t)
	return f
}
func (f *fixture) call(t *testing.T, method, path, body string, headers map[string]string) (int, map[string]any, *http.Response) {
	t.Helper()
	r, err := http.NewRequestWithContext(t.Context(), method, f.http.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", f.http.URL)
	r.Header.Set("X-CSRF-Token", f.csrf)
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	response, err := f.client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if len(data) > 0 {
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatalf("not JSON: %q", data)
		}
	}
	if response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("X-Request-ID") == "" {
		t.Fatal("missing safety headers")
	}
	if response.StatusCode != http.StatusNoContent && response.Header.Get("Content-Type") != "application/json" {
		t.Fatal("wrong content type")
	}
	return response.StatusCode, result, response
}
func (f *fixture) refresh(t *testing.T) {
	t.Helper()
	code, body, _ := f.call(t, "GET", "/api/v1/auth/csrf", "", nil)
	if code != 200 {
		t.Fatalf("csrf %d %v", code, body)
	}
	f.csrf = body["csrf_token"].(string)
}
func encoded(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func registration(t *testing.T) string {
	t.Helper()
	d := testauth.ConsentDocument()
	return encoded(t, map[string]any{"email": testauth.Email, "password": testauth.Password, "display_name": "API Demo", "personal_data_consent": map[string]any{"accepted": true, "document_version": d.GetDocumentVersion(), "document_sha256": d.GetDocumentSha256()}})
}
func loginBody() string { return `{"email":"demo@example.test","password":"example-password-only"}` }

func TestAllTenEndpointsOverMTLS(t *testing.T) {
	f := setup(t)
	cases := []struct {
		method, path, rpc, body string
		want                    int
		keys                    string
	}{
		{"GET", "/auth/personal-data-consent", "GetPersonalDataConsent", "", 200, "document_sha256,document_version,text"},
		{"GET", "/auth/csrf", "GetSecurityContext", "", 200, "csrf_token,expires_at"},
		{"POST", "/auth/register", "Register", registration(t), 202, "challenge_id,expires_at,message"},
		{"POST", "/auth/verify-email", "VerifyEmail", `{"challenge_id":"` + testauth.ChallengeID + `","code":"001234"}`, 200, "message"},
		{"POST", "/auth/resend-verification", "ResendVerification", `{"email":"demo@example.test"}`, 202, "challenge_id,expires_at,message"},
		{"POST", "/auth/forgot-password", "ForgotPassword", `{"email":"demo@example.test"}`, 202, "message"},
		{"POST", "/auth/login", "Login", loginBody(), 200, "expires_at,user"},
		{"GET", "/me", "GetCurrentAccount", "", 200, "display_name,email,email_verified,id"},
		{"POST", "/auth/logout", "Logout", "", 204, ""},
		{"POST", "/auth/reset-password", "ResetPassword", `{"token":"example-one-time-token","new_password":"changed-password-only"}`, 200, "message"},
	}
	for _, tc := range cases {
		t.Run(tc.rpc, func(t *testing.T) {
			f.refresh(t)
			before := len(f.auth.Requests(tc.rpc))
			code, body, response := f.call(t, tc.method, "/api/v1"+tc.path, tc.body, nil)
			if code != tc.want {
				t.Fatalf("got %d %v want %d", code, body, tc.want)
			}
			if len(f.auth.Requests(tc.rpc)) != before+1 {
				t.Fatal("request did not cross gRPC exactly once")
			}
			keys := strings.Split(tc.keys, ",")
			if tc.keys == "" {
				keys = nil
			}
			if len(body) != len(keys) {
				t.Fatalf("unexpected public fields %v", body)
			}
			for _, key := range keys {
				if _, ok := body[key]; !ok {
					t.Fatalf("missing %s", key)
				}
			}
			for _, c := range response.Cookies() {
				if !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.Domain != "" {
					t.Fatalf("unsafe cookie %+v", c)
				}
				if c.Value != "" && strings.Contains(encoded(t, body), c.Value) {
					t.Fatal("session token leaked")
				}
			}
			if tc.want == 204 && len(body) != 0 {
				t.Fatal("logout has body")
			}
			if tc.rpc == "Register" || tc.rpc == "ResendVerification" {
				if body["message"] != "Если подтверждение требуется, письмо с кодом будет отправлено." {
					t.Fatal("challenge acknowledgement contract")
				}
			}
			f.refresh(t)
			failure := status.Error(codes.Unavailable, "PRIVATE SESSION SECRET must not be public")
			f.auth.SetFailure(tc.rpc, failure)
			before = len(f.auth.Requests(tc.rpc))
			code, body, _ = f.call(t, tc.method, "/api/v1"+tc.path, tc.body, nil)
			f.auth.SetFailure(tc.rpc, nil)
			if code != 503 || body["error"].(map[string]any)["code"] != "DEPENDENCY_UNAVAILABLE" || strings.Contains(encoded(t, body), "PRIVATE") {
				t.Fatalf("unsafe dependency error %d %v", code, body)
			}
			if len(f.auth.Requests(tc.rpc)) != before+1 {
				t.Fatal("error request did not cross gRPC exactly once")
			}
		})
	}
	reg := f.auth.Requests("Register")[0].(*authv1.RegisterRequest)
	if reg.GetEmail() != testauth.Email || reg.GetPassword() != testauth.Password || !reg.GetPersonalDataConsent().GetAccepted() || reg.GetContext().GetClient().GetClientIp() != "127.0.0.1" {
		t.Fatalf("register mapping failed: %T", reg)
	}
	verify := f.auth.Requests("VerifyEmail")[0].(*authv1.VerifyEmailRequest)
	if verify.GetCode() != "001234" || verify.GetChallengeId() != testauth.ChallengeID {
		t.Fatal("leading zero/challenge mapping")
	}
}

func TestSecurityAndValidation(t *testing.T) {
	f := setup(t)
	tests := []struct {
		name, body, origin, csrf string
		want                     int
	}{
		{"foreign origin", loginBody(), "https://evil.test", f.csrf, 403}, {"null origin", loginBody(), "null", f.csrf, 403},
		{"missing origin", loginBody(), "", f.csrf, 403}, {"missing csrf", loginBody(), f.http.URL, "", 403},
		{"wrong csrf", loginBody(), f.http.URL, strings.Repeat("x", 43), 403},
		{"unknown field", `{"email":"demo@example.test","password":"example-password-only","role":"owner"}`, f.http.URL, f.csrf, 400},
		{"duplicate field", `{"email":"a","email":"b"}`, f.http.URL, f.csrf, 400},
		{"extra value", loginBody() + `{}`, f.http.URL, f.csrf, 400}, {"null", "null", f.http.URL, f.csrf, 400},
		{"short password", `{"email":"demo@example.test","password":"short"}`, f.http.URL, f.csrf, 422},
		{"wrong JSON type", `{"email":"demo@example.test","password":123456789012}`, f.http.URL, f.csrf, 422},
		{"wrong key case", `{"EMAIL":"demo@example.test","password":"example-password-only"}`, f.http.URL, f.csrf, 400},
		{"display email", `{"email":"Name <demo@example.test>","password":"example-password-only"}`, f.http.URL, f.csrf, 422},
		{"bad utf8", string([]byte{'{', '"', 0xff}), f.http.URL, f.csrf, 400},
		{"oversize", strings.Repeat(" ", 17000), f.http.URL, f.csrf, 400},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, body, _ := f.call(t, "POST", "/api/v1/auth/login", tc.body, map[string]string{"Origin": tc.origin, "X-CSRF-Token": tc.csrf, "X-User-ID": "forged", "X-Forwarded-For": "8.8.8.8"})
			if code != tc.want {
				t.Fatalf("%d %v", code, body)
			}
		})
	}
	// Origin absence may fall back to a same-origin Referer, while an explicit
	// hostile Origin must never fall back to a benign Referer.
	r, err := http.NewRequestWithContext(t.Context(), http.MethodPost, f.http.URL+"/api/v1/auth/login", strings.NewReader(loginBody()))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Referer", f.http.URL+"/login")
	r.Header.Set("X-CSRF-Token", f.csrf)
	r.Header.Set("Content-Type", "application/json")
	response, err := f.client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatal("referer fallback failed")
	}
	old := f.csrf
	f.refresh(t)
	if old == f.csrf {
		t.Fatal("login did not rotate CSRF")
	}
	code, body, _ := f.call(t, "POST", "/api/v1/auth/logout", "", map[string]string{"X-CSRF-Token": old})
	if code != 403 {
		t.Fatalf("stale csrf %d %v", code, body)
	}
	code, _, _ = f.call(t, "POST", "/api/v1/auth/logout", "", nil)
	if code != 204 {
		t.Fatal(code)
	}
	code, _, _ = f.call(t, "GET", "/api/v1/me", "", nil)
	if code != 401 {
		t.Fatal("logout session remains active")
	}
	for _, path := range []string{"/not-a-route", "/api/v1/auth/login/"} {
		code, _, _ := f.call(t, "GET", path, "", nil)
		if code != 404 {
			t.Fatalf("scope leak %s %d", path, code)
		}
	}
	code, _, _ = f.call(t, "GET", "/api/v1/auth/login", "", nil)
	if code != 405 {
		t.Fatal(code)
	}
}

func TestTimeoutCancellationAndOutage(t *testing.T) {
	f := setup(t)
	f.auth.SetDelay("GetPersonalDataConsent", time.Second)
	start := time.Now()
	code, _, _ := f.call(t, "GET", "/api/v1/auth/personal-data-consent", "", nil)
	if code != 503 || time.Since(start) > time.Second {
		t.Fatal("RPC deadline not propagated")
	}
	ctx, cancel := context.WithCancel(t.Context())
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, f.http.URL+"/api/v1/auth/personal-data-consent", nil)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		response, requestErr := f.client.Do(r)
		if response != nil {
			response.Body.Close()
		}
		done <- requestErr
	}()
	deadline := time.Now().Add(time.Second)
	for len(f.auth.Requests("GetPersonalDataConsent")) < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(f.auth.Requests("GetPersonalDataConsent")) < 2 {
		t.Fatal("cancel test did not reach RPC")
	}
	cancel()
	if <-done == nil {
		t.Fatal("HTTP cancellation ignored")
	}
	// Both the RPC deadline above and the cancelled HTTP request must cancel
	// the server-side gRPC context, not merely stop reading the HTTP response.
	for range 2 {
		select {
		case method := <-f.cancelled:
			if method != authv1.AuthService_GetPersonalDataConsent_FullMethodName {
				t.Fatal(method)
			}
		case <-time.After(time.Second):
			t.Fatal("gRPC server context was not cancelled")
		}
	}
	f.grpc.Stop()
	code, _, _ = f.call(t, "GET", "/api/v1/auth/csrf", "", nil)
	if code != 503 {
		t.Fatal("Auth outage accepted")
	}
}

func TestMappedBusinessFailures(t *testing.T) {
	f := setup(t)
	for _, tc := range []struct {
		path, body, reason string
		want               int
	}{
		{"login", `{"email":"demo@example.test","password":"wrong-password-only"}`, "INVALID_CREDENTIALS", 401},
		{"verify-email", `{"challenge_id":"` + testauth.ChallengeID + `","code":"999999"}`, "CODE_INVALID_OR_EXPIRED", 422},
		{"reset-password", `{"token":"invalid-one-time-token","new_password":"changed-password-only"}`, "TOKEN_INVALID_OR_EXPIRED", 422},
		{"register", strings.ReplaceAll(registration(t), testauth.ConsentVersion, "old-version"), "CONSENT_DOCUMENT_CHANGED", 409},
		{"register", `{"email":"demo@example.test","password":"example-password-only","display_name":"Demo"}`, "CONSENT_REQUIRED", 422},
	} {
		code, body, _ := f.call(t, "POST", "/api/v1/auth/"+tc.path, tc.body, nil)
		if code != tc.want || body["error"].(map[string]any)["code"] != tc.reason {
			t.Fatalf("%s %d %v", tc.path, code, body)
		}
	}
	// Raw passwords in request bodies are never echoed in the error envelope.
	code, body, _ := f.call(t, "POST", "/api/v1/auth/login", loginBody(), map[string]string{"Content-Type": "text/plain"})
	if code != 400 || bytes.Contains([]byte(encoded(t, body)), []byte(testauth.Password)) {
		t.Fatal("content type or secret handling")
	}
}
