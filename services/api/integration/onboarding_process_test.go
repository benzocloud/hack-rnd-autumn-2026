//go:build onboardingintegration

package integration

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	clientauth "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/client/auth"
	coreclient "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/client/core"
	authv1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/auth/v1"
	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/testauth"
)

const onboardingPassword = "ephemeral-onboarding-password"

type onboardingProcess struct {
	cmd     *exec.Cmd
	done    chan error
	stopped bool
}

func (p *onboardingProcess) stop(t *testing.T) {
	t.Helper()
	if p == nil || p.stopped {
		return
	}
	p.stopped = true
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Error("signal test process failed")
	}
	select {
	case err := <-p.done:
		if err != nil {
			t.Error("test service exited unsuccessfully; inspect its private temporary log")
		}
	case <-time.After(8 * time.Second):
		_ = p.cmd.Process.Kill()
		<-p.done
		t.Error("test service did not shut down within its deadline")
	}
}

type onboardingStack struct {
	dir, apiRoot, origin string
	authDB, coreDB       *url.URL
	pki                  testauth.PKI
	addresses            map[string]string
	programs             map[string]string
	envs                 map[string][]string
	processes            map[string]*onboardingProcess
	proxy                *onboardingProxy
	emails               []string
}

func onboardingEnvironment(values map[string]string) []string {
	env := make([]string, 0)
	for _, item := range os.Environ() {
		name, _, _ := strings.Cut(item, "=")
		if !strings.HasPrefix(name, "AUTH_") && !strings.HasPrefix(name, "CORE_") && !strings.HasPrefix(name, "API_") && !strings.HasPrefix(name, "PG") && !strings.HasPrefix(name, "GOOSE_") {
			env = append(env, item)
		}
	}
	for key, value := range values {
		env = append(env, key+"="+value)
	}
	return env
}

// psql keeps DSNs in the child environment, never command arguments or reports.
// Callers validate both the loopback database name and its server marker first.
func onboardingSQL(t *testing.T, database *url.URL, query string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "psql", "-X", "-qAt", "-v", "ON_ERROR_STOP=1")
	password, _ := database.User.Password()
	cmd.Env = onboardingEnvironment(map[string]string{
		"PGDATABASE": strings.TrimPrefix(database.Path, "/"), "PGHOST": database.Hostname(), "PGPORT": database.Port(),
		"PGUSER": database.User.Username(), "PGPASSWORD": password, "PGSSLMODE": "disable", "PGCONNECT_TIMEOUT": "3",
	})
	cmd.Stdin = strings.NewReader(query)
	output, err := cmd.Output()
	if err != nil {
		t.Fatal("test database query failed (connection details suppressed)")
	}
	return strings.TrimSpace(string(output))
}

func onboardingDatabase(t *testing.T, name, databaseName, marker string) *url.URL {
	t.Helper()
	database, err := url.Parse(os.Getenv(name))
	if err != nil || database == nil || database.Scheme != "postgres" || database.Hostname() != "127.0.0.1" || database.Port() == "" || database.Path != "/"+databaseName || database.User == nil || database.User.Username() == "" || database.Query().Get("sslmode") != "disable" || database.RawQuery != "sslmode=disable" || database.Fragment != "" {
		t.Fatalf("%s must explicitly select the disposable loopback %s database with sslmode=disable", name, databaseName)
	}
	if onboardingSQL(t, database, "SELECT coalesce(current_setting('saldo.test_database',true),'');") != marker {
		t.Fatal("refusing writes: test database marker missing")
	}
	return database
}

func onboardingAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}

func newOnboardingStack(t *testing.T) *onboardingStack {
	t.Helper()
	s := &onboardingStack{dir: t.TempDir(), addresses: map[string]string{}, programs: map[string]string{}, envs: map[string][]string{}, processes: map[string]*onboardingProcess{}}
	s.authDB = onboardingDatabase(t, "API_ONBOARDING_AUTH_DATABASE_URL", "saldo_onboarding_auth_test", "saldo-auth-integration")
	s.coreDB = onboardingDatabase(t, "API_ONBOARDING_CORE_DATABASE_URL", "saldo_onboarding_core_test", "saldo-core-integration")
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	s.apiRoot = filepath.Dir(root)
	s.pki, err = testauth.GeneratePKI(filepath.Join(s.dir, "pki"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"auth", "core", "api", "proxy", "auth-health"} {
		s.addresses[name] = onboardingAddress(t)
	}
	s.origin = "https://" + s.addresses["api"]
	suffix := hex.EncodeToString(randomOnboardingBytes())[:12]
	s.emails = []string{"owner-" + suffix + "@example.test", "other-" + suffix + "@example.test"}
	for _, item := range []struct{ name, module, command string }{{"auth", "auth", "auth"}, {"migrate", "auth", "migrate"}, {"core", "core", "core"}, {"api", "api", "api"}} {
		binary := filepath.Join(s.dir, item.name)
		cmd := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "./cmd/"+item.command)
		cmd.Dir = filepath.Join(s.apiRoot, "..", item.module)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %s", item.name, output)
		}
		s.programs[item.name] = binary
	}
	s.envs["auth"] = onboardingEnvironment(map[string]string{
		"AUTH_ENV": "local", "AUTH_DATABASE_URL": s.authDB.String(), "AUTH_HTTP_ADDRESS": s.addresses["auth-health"],
		"AUTH_GRPC_ADDRESS": s.addresses["auth"], "AUTH_GRPC_CERT_FILE": s.pki.AuthCert, "AUTH_GRPC_KEY_FILE": s.pki.AuthKey, "AUTH_GRPC_CA_FILE": s.pki.CA,
		"AUTH_CODE_HMAC_KEY": base64.RawURLEncoding.EncodeToString(randomOnboardingBytes()), "AUTH_CSRF_HMAC_KEY": base64.RawURLEncoding.EncodeToString(randomOnboardingBytes()),
		"AUTH_DEMO_EMAIL_VERIFICATION": "true", "AUTH_DEMO_TEST_EMAILS": strings.Join(s.emails, ","),
		"AUTH_CORE_ADDRESS": s.addresses["proxy"], "AUTH_CORE_SERVER_NAME": "localhost", "AUTH_CORE_TIMEOUT": "1s",
	})
	password, _ := s.coreDB.User.Password()
	if password == "" {
		t.Fatal("Core test DSN requires an explicit synthetic password")
	}
	s.envs["core"] = onboardingEnvironment(map[string]string{
		"CORE_POSTGRES_HOST": s.coreDB.Hostname(), "CORE_POSTGRES_PORT": s.coreDB.Port(), "CORE_POSTGRES_USER": s.coreDB.User.Username(), "CORE_POSTGRES_PASSWORD": password,
		"CORE_POSTGRES_DATABASE": strings.TrimPrefix(s.coreDB.Path, "/"), "CORE_POSTGRES_SSL_MODE": "disable",
		"CORE_GRPC_ADDRESS": s.addresses["core"], "CORE_AUTH_ADDRESS": s.addresses["auth"], "CORE_AUTH_SERVER_NAME": "localhost", "CORE_AUTH_TIMEOUT": "1s",
		"CORE_MTLS_CA_FILE": s.pki.CA, "CORE_MTLS_CERT_FILE": s.pki.CoreCert, "CORE_MTLS_KEY_FILE": s.pki.CoreKey,
	})
	s.envs["api"] = onboardingEnvironment(map[string]string{
		"API_ADDRESS": s.addresses["api"], "API_ORIGIN": s.origin, "API_HTTP_CERT": s.pki.HTTPCert, "API_HTTP_KEY": s.pki.HTTPKey,
		"API_AUTH_ADDRESS": s.addresses["auth"], "API_AUTH_CA": s.pki.CA, "API_AUTH_CERT": s.pki.APICert, "API_AUTH_KEY": s.pki.APIKey,
		"API_CORE_ADDRESS": s.addresses["proxy"], "API_CORE_SERVER_NAME": "localhost", "API_RPC_TIMEOUT": "2s",
	})
	migrate := exec.CommandContext(t.Context(), s.programs["migrate"])
	migrate.Env = s.envs["auth"]
	if err := migrate.Run(); err != nil {
		t.Fatal("Auth test migration failed")
	}
	goose := os.Getenv("GOOSE")
	if goose == "" {
		goose = "goose"
	}
	version, err := exec.CommandContext(t.Context(), goose, "-version").Output()
	if err != nil || !strings.Contains(string(version), "v3.29.0") {
		t.Fatal("install pinned goose v3.29.0")
	}
	migrate = exec.CommandContext(t.Context(), goose, "-dir", filepath.Join(s.apiRoot, "..", "core", "migrations"), "up")
	migrate.Env = onboardingEnvironment(map[string]string{"GOOSE_DRIVER": "postgres", "GOOSE_DBSTRING": s.coreDB.String()})
	if err := migrate.Run(); err != nil {
		t.Fatal("Core test migration failed")
	}
	s.proxy = startOnboardingProxy(t, s.pki, s.addresses["proxy"], s.addresses["core"])
	t.Cleanup(func() {
		for _, name := range []string{"api", "auth", "core"} {
			s.processes[name].stop(t)
		}
	})
	s.start(t, "auth")
	s.start(t, "core")
	s.start(t, "api")
	s.waitReady(t)
	return s
}

func randomOnboardingBytes() []byte {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		panic(err)
	}
	return value
}

func (s *onboardingStack) start(t *testing.T, name string) {
	t.Helper()
	log, err := os.CreateTemp(s.dir, name+"-*.log")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = log.Close() })
	cmd := exec.Command(s.programs[name])
	cmd.Env, cmd.Stdout, cmd.Stderr = s.envs[name], log, log
	if err := cmd.Start(); err != nil {
		t.Fatalf("start test %s failed", name)
	}
	p := &onboardingProcess{cmd: cmd, done: make(chan error, 1)}
	go func() { p.done <- cmd.Wait() }()
	s.processes[name] = p
}

func (s *onboardingStack) restart(t *testing.T) {
	t.Helper()
	for _, name := range []string{"api", "auth", "core"} {
		s.processes[name].stop(t)
	}
	for _, name := range []string{"auth", "core", "api"} {
		s.start(t, name)
	}
	s.waitReady(t)
}

type onboardingBrowser struct {
	client       *http.Client
	origin, csrf string
}

func (s *onboardingStack) browser(t *testing.T) *onboardingBrowser {
	t.Helper()
	ca, err := os.ReadFile(s.pki.CA)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		t.Fatal("invalid test HTTPS trust")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots}}
	t.Cleanup(transport.CloseIdleConnections)
	return &onboardingBrowser{origin: s.origin, client: &http.Client{Timeout: 5 * time.Second, Jar: jar, Transport: transport}}
}

func (s *onboardingStack) waitReady(t *testing.T) {
	t.Helper()
	b := s.browser(t)
	coreConn, err := coreclient.Dial(s.addresses["core"], "localhost", s.pki.CA, s.pki.AuthCert, s.pki.AuthKey, time.Second)
	if err != nil {
		t.Fatal("cannot initialize real Core readiness probe")
	}
	defer coreConn.Close()
	core := corev1.NewCompanyServiceClient(coreConn)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		// API transport readiness ends at the fault proxy. Independently require
		// the actual Core process/DB to return its missing-receipt business result.
		_, coreErr := core.GetCompanyCreationResult(t.Context(), &corev1.GetCompanyCreationResultRequest{CommandId: "ac124a91-bb12-4a2a-8011-14361bdb7ac0"})
		if status.Code(coreErr) != codes.NotFound {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, s.origin+"/health/ready", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := b.client.Do(request)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("real service chain readiness deadline; inspect private temporary logs")
}

func (b *onboardingBrowser) call(t *testing.T, method, path string, body any, want int, headers map[string]string) map[string]any {
	t.Helper()
	var data io.Reader
	if body != nil {
		value, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		data = strings.NewReader(string(value))
	}
	r, err := http.NewRequestWithContext(t.Context(), method, b.origin+"/api/v1"+path, data)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", b.origin)
	r.Header.Set("X-CSRF-Token", b.csrf)
	for key, value := range headers {
		r.Header.Set(key, value)
	}
	response, err := b.client.Do(r)
	if err != nil {
		t.Fatalf("HTTPS %s %s transport failed", method, path)
	}
	defer response.Body.Close()
	if response.StatusCode != want {
		t.Fatalf("HTTPS %s %s status=%d want=%d", method, path, response.StatusCode, want)
	}
	if response.Header.Get("X-Request-ID") == "" || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("missing response safety headers")
	}
	if want == http.StatusNoContent {
		return nil
	}
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal("invalid HTTPS JSON response")
	}
	return result
}

func (b *onboardingBrowser) refresh(t *testing.T) {
	t.Helper()
	b.csrf = b.call(t, http.MethodGet, "/auth/csrf", nil, 200, nil)["csrf_token"].(string)
}

func (b *onboardingBrowser) register(t *testing.T, email string) string {
	t.Helper()
	b.refresh(t)
	consent := b.call(t, http.MethodGet, "/auth/personal-data-consent", nil, 200, nil)
	challenge := b.call(t, http.MethodPost, "/auth/register", map[string]any{"email": email, "password": onboardingPassword, "display_name": "Onboarding test", "personal_data_consent": map[string]any{"accepted": true, "document_version": consent["document_version"], "document_sha256": consent["document_sha256"]}}, 202, nil)
	// Existing server-only isolated .test allowlist profile; finances stay real.
	b.call(t, http.MethodPost, "/auth/verify-email", map[string]any{"challenge_id": challenge["challenge_id"], "code": "001234"}, 200, nil)
	return b.login(t, email)
}

func (b *onboardingBrowser) login(t *testing.T, email string) string {
	t.Helper()
	b.refresh(t)
	b.call(t, http.MethodPost, "/auth/login", map[string]any{"email": email, "password": onboardingPassword}, 200, nil)
	b.refresh(t)
	return b.call(t, http.MethodGet, "/me", nil, 200, nil)["id"].(string)
}

func (b *onboardingBrowser) writeContext(t *testing.T) *authv1.WriteContext {
	t.Helper()
	u, err := url.Parse(b.origin)
	if err != nil {
		t.Fatal(err)
	}
	for _, cookie := range b.client.Jar.Cookies(u) {
		if cookie.Name == "__Host-saldo_session" {
			return &authv1.WriteContext{Browser: &authv1.BrowserContext{CookieToken: cookie.Value}, CsrfToken: b.csrf}
		}
	}
	t.Fatal("missing live test browser session")
	return nil
}

func companyBody(name string) map[string]any {
	return map[string]any{"name": name, "inn": nil, "currency": "RUB", "timezone": "Europe/Moscow"}
}

func (s *onboardingStack) waitState(t *testing.T, key, want string) string {
	t.Helper()
	// Keys below are fixed test identifiers; no browser input enters this SQL.
	query := fmt.Sprintf("SELECT state || ':' || company_id FROM company_creation_commands WHERE idempotency_key='%s';", key)
	// A cancelled worker may intentionally leave the durable 60s lease intact.
	deadline := time.Now().Add(80 * time.Second)
	for time.Now().Before(deadline) {
		value := onboardingSQL(t, s.authDB, query)
		if strings.HasPrefix(value, want+":") {
			return strings.TrimPrefix(value, want+":")
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("command %s did not reach %s", key, want)
	return ""
}

func (s *onboardingStack) assertOneEmptyCompany(t *testing.T, company string) {
	t.Helper()
	for _, table := range []string{"core_companies", "core_accounts", "core_company_bootstraps"} {
		column := "company_id"
		if table == "core_companies" {
			column = "id"
		}
		if onboardingSQL(t, s.coreDB, fmt.Sprintf("SELECT count(*) FROM %s WHERE %s='%s';", table, column, company)) != "1" {
			t.Fatalf("%s count differs from one", table)
		}
	}
	if onboardingSQL(t, s.authDB, fmt.Sprintf("SELECT count(*) FROM memberships WHERE company_id='%s' AND role='owner';", company)) != "1" {
		t.Fatal("owner was not finalized exactly once")
	}
	for _, table := range []string{"core_transactions", "core_balances", "core_source_bindings", "core_history_coverage"} {
		if onboardingSQL(t, s.coreDB, fmt.Sprintf("SELECT count(*) FROM %s WHERE company_id='%s';", table, company)) != "0" {
			t.Fatalf("new account unexpectedly contains %s", table)
		}
	}
}

func TestRealCompanyOnboardingProcesses(t *testing.T) {
	s := newOnboardingStack(t)
	b := s.browser(t)
	ownerID := b.register(t, s.emails[0])
	key := "create-" + hex.EncodeToString(randomOnboardingBytes())[:16]
	body := companyBody("  ООО «Пустой счёт» <&>  ")
	headers := map[string]string{"Idempotency-Key": key}
	if len(b.call(t, http.MethodGet, "/companies", nil, 200, nil)["items"].([]any)) != 0 {
		t.Fatal("new user already has membership")
	}
	b.call(t, http.MethodPost, "/companies", body, 403, map[string]string{"Idempotency-Key": key, "X-CSRF-Token": ""})
	var group sync.WaitGroup
	ids := make(chan string, 2)
	for range 2 {
		group.Go(func() {
			value := b.call(t, http.MethodPost, "/companies", body, 201, headers)
			ids <- value["id"].(string)
		})
	}
	group.Wait()
	close(ids)
	company := ""
	for id := range ids {
		if company != "" && company != id {
			t.Fatal("concurrent same-key requests created two companies")
		}
		company = id
	}
	if company == "" {
		t.Fatal("double-click returned no company")
	}
	s.assertOneEmptyCompany(t, company)
	created := b.call(t, http.MethodPost, "/companies", body, 201, headers)
	if created["id"] != company || created["name"] != "ООО «Пустой счёт» <&>" || created["membership"].(map[string]any)["role"] != "owner" {
		t.Fatal("normalized company/owner replay mismatch")
	}
	b.call(t, http.MethodPost, "/companies", companyBody("Changed body"), 409, headers)
	account := b.call(t, http.MethodGet, "/companies/"+company+"/account", nil, 200, nil)
	if account["id"] != created["default_account_id"] || account["balance"] != nil || account["transaction_source"] != nil || account["data_version"] != "1" {
		t.Fatal("new default account must be empty and unbound, with its original ID/version")
	}
	s.restart(t)
	if b.call(t, http.MethodPost, "/companies", body, 201, headers)["id"] != company {
		t.Fatal("completed replay changed after all processes restarted")
	}
	b.call(t, http.MethodPost, "/auth/logout", nil, 204, nil)
	if b.login(t, s.emails[0]) != ownerID {
		t.Fatal("login identity changed")
	}
	if len(b.call(t, http.MethodGet, "/companies", nil, 200, nil)["items"].([]any)) != 1 {
		t.Fatal("owner membership not retained after login")
	}
	s.assertOneEmptyCompany(t, company)
	t.Log("real register/verify/login, double-click/replay/conflict, empty account, restart/logout/login passed")

	t.Run("CoreCommitLostReplyAndRestart", func(t *testing.T) {
		lostKey := "lost-" + hex.EncodeToString(randomOnboardingBytes())[:16]
		s.proxy.holdReceipt.Store(true)
		s.proxy.dropReply.Store(true)
		defer s.proxy.holdReceipt.Store(false)
		b.call(t, http.MethodPost, "/companies", companyBody("Lost reply"), 503, map[string]string{"Idempotency-Key": lostKey})
		lostCompany := s.waitState(t, lostKey, "PREPARED")
		if onboardingSQL(t, s.coreDB, fmt.Sprintf("SELECT count(*) FROM core_company_bootstraps WHERE company_id='%s';", lostCompany)) != "1" {
			t.Fatal("fault did not occur after actual Core commit")
		}
		if onboardingSQL(t, s.authDB, fmt.Sprintf("SELECT count(*) FROM memberships WHERE company_id='%s';", lostCompany)) != "0" {
			t.Fatal("membership granted without readable receipt")
		}
		b.call(t, http.MethodPost, "/auth/logout", nil, 204, nil)
		s.restart(t)
		s.proxy.holdReceipt.Store(false)
		if s.waitState(t, lostKey, "COMPLETED") != lostCompany {
			t.Fatal("recovery changed accepted company")
		}
		b.login(t, s.emails[0])
		if b.call(t, http.MethodPost, "/companies", companyBody("Lost reply"), 201, map[string]string{"Idempotency-Key": lostKey})["id"] != lostCompany {
			t.Fatal("retry after recovery created a duplicate")
		}
		s.assertOneEmptyCompany(t, lostCompany)
	})

	t.Run("PreparedBeforeBootstrapAndRestart", func(t *testing.T) {
		preparedKey := "prepared-" + hex.EncodeToString(randomOnboardingBytes())[:16]
		s.proxy.holdBootstrap.Store(true)
		defer s.proxy.holdBootstrap.Store(false)
		b.call(t, http.MethodPost, "/companies", companyBody("Before bootstrap"), 503, map[string]string{"Idempotency-Key": preparedKey})
		preparedCompany := s.waitState(t, preparedKey, "PREPARED")
		if onboardingSQL(t, s.coreDB, fmt.Sprintf("SELECT count(*) FROM core_company_bootstraps WHERE company_id='%s';", preparedCompany)) != "0" {
			t.Fatal("bootstrap transport fault did not prevent commit")
		}
		s.restart(t)
		s.proxy.holdBootstrap.Store(false)
		if s.waitState(t, preparedKey, "COMPLETED") != preparedCompany {
			t.Fatal("recovery changed prepared company")
		}
		s.assertOneEmptyCompany(t, preparedCompany)
	})

	t.Run("ForeignCommandPeersAndRevocation", func(t *testing.T) {
		other := s.browser(t)
		other.register(t, s.emails[1])
		apiAuth, conn, err := clientauth.Dial(s.addresses["auth"], s.pki.CA, s.pki.APICert, s.pki.APIKey, time.Second, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		command := onboardingSQL(t, s.authDB, fmt.Sprintf("SELECT id FROM company_creation_commands WHERE idempotency_key='%s';", key))
		_, err = apiAuth.CompleteCompanyCreation(t.Context(), &authv1.CompleteCompanyCreationRequest{Context: other.writeContext(t), CommandId: command})
		if status.Code(err) != codes.PermissionDenied {
			t.Fatal("foreign user completed another initiator's command")
		}
		_, err = apiAuth.AuthorizeCompanyBootstrap(t.Context(), &authv1.AuthorizeCompanyBootstrapRequest{CommandId: command, CompanyId: company})
		if status.Code(err) != codes.PermissionDenied {
			t.Fatal("API peer reached Core-only AuthorizeCompanyBootstrap")
		}
		coreConn, err := coreclient.Dial(s.addresses["core"], "localhost", s.pki.CA, s.pki.APICert, s.pki.APIKey, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer coreConn.Close()
		_, err = corev1.NewCompanyServiceClient(coreConn).GetCompanyCreationResult(t.Context(), &corev1.GetCompanyCreationResultRequest{CommandId: command})
		if status.Code(err) != codes.PermissionDenied {
			t.Fatal("API peer read Auth-only receipt")
		}
		other.call(t, http.MethodGet, "/companies/"+company, nil, 404, nil)
		// Negative authorization fixture only: the owner above was created through
		// HTTP, never inserted with SQL. A completed replay must not restore it.
		onboardingSQL(t, s.authDB, fmt.Sprintf("DELETE FROM memberships WHERE company_id='%s';", company))
		b.call(t, http.MethodGet, "/companies/"+company, nil, 404, nil)
		b.call(t, http.MethodPost, "/companies", body, 404, headers)
		if onboardingSQL(t, s.authDB, fmt.Sprintf("SELECT count(*) FROM memberships WHERE company_id='%s';", company)) != "0" {
			t.Fatal("completed replay restored revoked membership")
		}
	})
}
