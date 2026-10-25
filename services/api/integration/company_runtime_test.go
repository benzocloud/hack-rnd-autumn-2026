//go:build onboarding

package integration

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/testauth"
)

var challengeIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

const postgresImage = "postgres:18.6@sha256:edb0c08dc33befd0262ab3b5cf1c4f3f8cfd8b09e88c05b45adee29d6761ddb6"

func TestRealCompanyOnboardingChain(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatal("docker is required for the real Auth, Core and API chain")
	}
	root := repoRoot(t)
	bin := t.TempDir()
	pki, err := testauth.GeneratePKI(filepath.Join(t.TempDir(), "pki"))
	if err != nil {
		t.Fatal(err)
	}
	buildBinary(t, filepath.Join(root, "services/auth"), filepath.Join(bin, "auth"), "./cmd/auth")
	buildBinary(t, filepath.Join(root, "services/auth"), filepath.Join(bin, "auth-migrate"), "./cmd/migrate")
	buildBinary(t, filepath.Join(root, "services/core"), filepath.Join(bin, "core"), "./cmd/core")
	buildBinary(t, filepath.Join(root, "services/api"), filepath.Join(bin, "api"), "./cmd/api")

	port := dockerPostgres(t)
	psql(t, "postgres", "CREATE DATABASE auth_onboarding_test")
	psql(t, "postgres", "CREATE DATABASE core_onboarding_test")
	psql(t, "auth_onboarding_test", "ALTER DATABASE auth_onboarding_test SET saldo.test_database = 'saldo-auth-integration'")
	psql(t, "core_onboarding_test", "ALTER DATABASE core_onboarding_test SET saldo.test_database = 'saldo-core-integration'")
	applyCoreSchema(t, root)
	authURL := fmt.Sprintf("postgres://onboarding_test:onboarding-test@127.0.0.1:%s/auth_onboarding_test?sslmode=disable", port)
	runChecked(t, filepath.Join(bin, "auth-migrate"), filepath.Join(root, "services/auth"), []string{"AUTH_ENV=local", "AUTH_DATABASE_URL=" + authURL, "AUTH_QUERY_TIMEOUT=5s", "AUTH_STARTUP_TIMEOUT=5s"})

	authHTTP := reservedPort(t)
	authGRPC := reservedPort(t)
	coreGRPC := reservedPort(t)
	apiPort := reservedPort(t)
	origin := "https://127.0.0.1:" + apiPort
	codeKey := bytes.Repeat([]byte{7}, 32)
	encodedKey := base64.RawURLEncoding.EncodeToString(codeKey)
	authEnv := []string{
		"AUTH_ENV=local", "AUTH_DATABASE_URL=" + authURL, "AUTH_HTTP_ADDRESS=127.0.0.1:" + authHTTP,
		"AUTH_GRPC_ADDRESS=127.0.0.1:" + authGRPC, "AUTH_GRPC_CERT_FILE=" + pki.AuthCert, "AUTH_GRPC_KEY_FILE=" + pki.AuthKey, "AUTH_GRPC_CA_FILE=" + pki.CA,
		"AUTH_CODE_HMAC_KEY=" + encodedKey, "AUTH_CSRF_HMAC_KEY=" + encodedKey,
		"AUTH_CORE_ADDRESS=127.0.0.1:" + coreGRPC, "AUTH_CORE_SERVER_NAME=localhost", "AUTH_CORE_TIMEOUT=2s",
		"AUTH_QUERY_TIMEOUT=5s", "AUTH_STARTUP_TIMEOUT=5s",
	}
	coreEnv := []string{
		"CORE_POSTGRES_HOST=127.0.0.1", "CORE_POSTGRES_PORT=" + port, "CORE_POSTGRES_USER=onboarding_test", "CORE_POSTGRES_PASSWORD=onboarding-test",
		"CORE_POSTGRES_DATABASE=core_onboarding_test", "CORE_POSTGRES_SSL_MODE=disable",
		"CORE_GRPC_ADDRESS=127.0.0.1:" + coreGRPC, "CORE_AUTH_ADDRESS=127.0.0.1:" + authGRPC, "CORE_AUTH_SERVER_NAME=localhost", "CORE_AUTH_TIMEOUT=2s",
		"CORE_MTLS_CA_FILE=" + pki.CA, "CORE_MTLS_CERT_FILE=" + pki.CoreCert, "CORE_MTLS_KEY_FILE=" + pki.CoreKey,
	}
	apiEnv := []string{
		"API_ADDRESS=127.0.0.1:" + apiPort, "API_ORIGIN=" + origin, "API_AUTH_ADDRESS=127.0.0.1:" + authGRPC,
		"API_AUTH_CA=" + pki.CA, "API_AUTH_CERT=" + pki.APICert, "API_AUTH_KEY=" + pki.APIKey,
		"API_HTTP_CERT=" + pki.HTTPCert, "API_HTTP_KEY=" + pki.HTTPKey,
		"API_CORE_ADDRESS=127.0.0.1:" + coreGRPC, "API_CORE_SERVER_NAME=localhost",
		"API_CORE_CA=" + pki.CA, "API_CORE_CERT=" + pki.APICert, "API_CORE_KEY=" + pki.APIKey,
		"API_RPC_TIMEOUT=3s", "API_SLOW_TIMEOUT=10s",
	}
	stopCore := startProcess(t, filepath.Join(bin, "core"), filepath.Join(root, "services/core"), coreEnv, "127.0.0.1:"+coreGRPC)
	defer stopCore()
	stopAuth := startProcess(t, filepath.Join(bin, "auth"), filepath.Join(root, "services/auth"), authEnv, "127.0.0.1:"+authHTTP, "127.0.0.1:"+authGRPC)
	defer stopAuth()
	stopAPI := startProcess(t, filepath.Join(bin, "api"), filepath.Join(root, "services/api"), apiEnv, "127.0.0.1:"+apiPort)
	defer stopAPI()

	client := httpsClient(t, pki.CA)
	waitReady(t, client, origin+"/health/ready")
	session := loginUser(t, client, origin, "owner@example.test", codeKey)
	created := session.call(t, http.MethodPost, "/api/v1/companies", `{"name":"Кофейня Онбординг","inn":null,"currency":"RUB","timezone":"Europe/Moscow"}`, map[string]string{"Idempotency-Key": "onboarding-key-1"})
	companyID, idOK := created.body["id"].(string)
	if created.code != 201 || !idOK || companyID == "" || nested(created.body, "membership", "role") != "owner" {
		t.Fatalf("create: %d %v", created.code, created.body)
	}
	replay := session.call(t, http.MethodPost, "/api/v1/companies", `{"name":"Кофейня Онбординг","inn":null,"currency":"RUB","timezone":"Europe/Moscow"}`, map[string]string{"Idempotency-Key": "onboarding-key-1"})
	if replay.code != 201 || replay.body["id"] != companyID {
		t.Fatalf("replay: %d %v", replay.code, replay.body)
	}
	conflict := session.call(t, http.MethodPost, "/api/v1/companies", `{"name":"Другая компания","inn":null,"currency":"RUB","timezone":"Europe/Moscow"}`, map[string]string{"Idempotency-Key": "onboarding-key-1"})
	if conflict.code != 409 {
		t.Fatalf("conflict: %d %v", conflict.code, conflict.body)
	}
	noCSRF := session.call(t, http.MethodPost, "/api/v1/companies", `{"name":"Кофейня Онбординг","inn":null,"currency":"RUB","timezone":"Europe/Moscow"}`, map[string]string{"Idempotency-Key": "onboarding-key-2", "X-CSRF-Token": ""})
	if noCSRF.code != http.StatusForbidden {
		t.Fatalf("missing csrf: %d %v", noCSRF.code, noCSRF.body)
	}
	account := session.call(t, http.MethodGet, "/api/v1/companies/"+companyID+"/account", "", nil)
	if account.code != 200 || account.body["balance"] != nil || account.body["transaction_source"] != nil || account.body["data_version"] != "1" {
		t.Fatalf("account: %d %v", account.code, account.body)
	}
	if got := psqlValue(t, "core_onboarding_test", "SELECT count(*) FROM core_transactions"); got != "0" {
		t.Fatalf("new account has transactions: %s", got)
	}
	if got := psqlValue(t, "core_onboarding_test", "SELECT count(*) FROM core_balances"); got != "0" {
		t.Fatalf("new account has balances: %s", got)
	}
	stopAuth()
	stopAuth = startProcess(t, filepath.Join(bin, "auth"), filepath.Join(root, "services/auth"), authEnv, "127.0.0.1:"+authHTTP, "127.0.0.1:"+authGRPC)
	waitReady(t, client, origin+"/health/ready")
	read := session.call(t, http.MethodGet, "/api/v1/companies/"+companyID, "", nil)
	if read.code != 200 || nested(read.body, "membership", "role") != "owner" {
		t.Fatalf("company after auth restart: %d %v", read.code, read.body)
	}
	colleague := loginUser(t, httpsClient(t, pki.CA), origin, "colleague@example.test", codeKey)
	foreign := colleague.call(t, http.MethodGet, "/api/v1/companies/"+companyID, "", nil)
	if foreign.code != http.StatusForbidden && foreign.code != http.StatusNotFound {
		t.Fatalf("another user: %d %v", foreign.code, foreign.body)
	}
}

type apiResult struct {
	code int
	body map[string]any
}

type browserSession struct {
	client *http.Client
	origin string
	csrf   string
}

func (s browserSession) call(t *testing.T, method, path, body string, headers map[string]string) apiResult {
	t.Helper()
	request, err := http.NewRequest(method, s.origin+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", s.origin)
	request.Header.Set("X-CSRF-Token", s.csrf)
	for key, value := range headers {
		if value == "" {
			request.Header.Del(key)
			continue
		}
		request.Header.Set(key, value)
	}
	response, err := s.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if len(bytes.TrimSpace(payload)) > 0 {
		if err := json.Unmarshal(payload, &decoded); err != nil {
			t.Fatalf("status %d not JSON: %s", response.StatusCode, payload)
		}
	}
	return apiResult{code: response.StatusCode, body: decoded}
}

func loginUser(t *testing.T, client *http.Client, origin, email string, codeKey []byte) browserSession {
	t.Helper()
	session := browserSession{client: client, origin: origin}
	csrf := session.call(t, http.MethodGet, "/api/v1/auth/csrf", "", nil)
	if csrf.code != 200 {
		t.Fatalf("csrf: %d %v", csrf.code, csrf.body)
	}
	session.csrf = csrf.body["csrf_token"].(string)
	consent := session.call(t, http.MethodGet, "/api/v1/auth/personal-data-consent", "", nil)
	if consent.code != 200 {
		t.Fatalf("consent: %d %v", consent.code, consent.body)
	}
	registerBody, _ := json.Marshal(map[string]any{
		"email": email, "password": "correct horse battery staple", "display_name": "Владелец",
		"personal_data_consent": map[string]any{"accepted": true, "document_version": consent.body["document_version"], "document_sha256": consent.body["document_sha256"]},
	})
	registered := session.call(t, http.MethodPost, "/api/v1/auth/register", string(registerBody), nil)
	challengeID, challengeOK := registered.body["challenge_id"].(string)
	if registered.code != 202 || !challengeOK || !challengeIDPattern.MatchString(challengeID) {
		t.Fatalf("register: %d %v", registered.code, registered.body)
	}
	code := verificationCode(t, challengeID, codeKey)
	wrong := "000000"
	if wrong == code {
		wrong = "000001"
	}
	rejected := session.call(t, http.MethodPost, "/api/v1/auth/verify-email", fmt.Sprintf(`{"challenge_id":%q,"code":%q}`, challengeID, wrong), nil)
	if rejected.code < 400 || rejected.code >= 500 {
		t.Fatalf("wrong verification code: %d %v", rejected.code, rejected.body)
	}
	verified := session.call(t, http.MethodPost, "/api/v1/auth/verify-email", fmt.Sprintf(`{"challenge_id":%q,"code":%q}`, challengeID, code), nil)
	if verified.code != 200 {
		t.Fatalf("verify: %d %v", verified.code, verified.body)
	}
	logged := session.call(t, http.MethodPost, "/api/v1/auth/login", fmt.Sprintf(`{"email":%q,"password":"correct horse battery staple"}`, email), nil)
	if logged.code != 200 {
		t.Fatalf("login: %d %v", logged.code, logged.body)
	}
	refreshed := session.call(t, http.MethodGet, "/api/v1/auth/csrf", "", nil)
	if refreshed.code != 200 {
		t.Fatalf("session csrf: %d %v", refreshed.code, refreshed.body)
	}
	session.csrf = refreshed.body["csrf_token"].(string)
	return session
}

func httpsClient(t *testing.T, caFile string) *http.Client {
	t.Helper()
	pem, err := os.ReadFile(caFile)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		t.Fatal("test CA rejected")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Timeout: 20 * time.Second, Jar: jar, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool}}}
}

func waitReady(t *testing.T, client *http.Client, url string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		response, err := client.Get(url)
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
			last = fmt.Errorf("status %d", response.StatusCode)
		} else {
			last = err
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("service not ready: %v", last)
}

func startProcess(t *testing.T, binary, dir string, env []string, addresses ...string) func() {
	t.Helper()
	var last error
	for range 5 {
		if err := claimAddresses(addresses...); err != nil {
			last = err
			time.Sleep(20 * time.Millisecond)
			continue
		}
		stop, err := runProcess(t, binary, dir, env)
		if err == nil {
			return stop
		}
		last = err
		if !bindConflict(err) {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("could not bind %s: %v", binary, last)
	return nil
}

func claimAddresses(addresses ...string) error {
	listeners := make([]net.Listener, 0, len(addresses))
	for _, address := range addresses {
		listener, err := net.Listen("tcp", address)
		if err != nil {
			for _, open := range listeners {
				_ = open.Close()
			}
			return err
		}
		listeners = append(listeners, listener)
	}
	for _, listener := range listeners {
		_ = listener.Close()
	}
	return nil
}

func runProcess(t *testing.T, binary, dir string, env []string) (func(), error) {
	t.Helper()
	cmd := exec.Command(binary)
	cmd.Dir = dir
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH")}, env...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			select {
			case <-exited:
				return
			default:
			}
			if cmd.Process != nil {
				_ = cmd.Process.Signal(os.Interrupt)
			}
			select {
			case <-exited:
			case <-time.After(6 * time.Second):
				if cmd.Process != nil {
					_ = cmd.Process.Kill()
				}
				<-exited
			}
		})
	}
	select {
	case <-exited:
		return nil, fmt.Errorf("%s exited before its port was held: %s", binary, output.String())
	case <-time.After(400 * time.Millisecond):
	}
	t.Cleanup(func() {
		stop()
		if t.Failed() {
			t.Log(output.String())
		}
	})
	return stop, nil
}

func bindConflict(err error) bool {
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "address already in use") || strings.Contains(text, "bind")
}

func buildBinary(t *testing.T, dir, output, pkg string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-o", output, pkg)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.27.3")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build %s: %v\n%s", pkg, err, out)
	}
}

func dockerPostgres(t *testing.T) string {
	t.Helper()
	name := "saldo-onboarding-chain-" + strings.ToLower(t.Name())
	_ = exec.Command("docker", "rm", "-f", name).Run()
	cmd := exec.Command("docker", "run", "--rm", "--name", name, "-e", "POSTGRES_USER=onboarding_test", "-e", "POSTGRES_PASSWORD=onboarding-test", "-e", "POSTGRES_DB=postgres", "-e", "POSTGRES_HOST_AUTH_METHOD=trust", "-p", "127.0.0.1::5432", "-d", postgresImage)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("postgres: %v\n%s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })
	port := strings.TrimSpace(string(mustOutput(t, "docker", "port", name, "5432/tcp")))
	port = port[strings.LastIndex(port, ":")+1:]
	deadline := time.Now().Add(40 * time.Second)
	ready := 0
	for time.Now().Before(deadline) {
		if exec.Command("docker", "exec", name, "psql", "-U", "onboarding_test", "-d", "postgres", "-c", "SELECT 1").Run() == nil {
			ready++
			if ready == 3 {
				return port
			}
		} else {
			ready = 0
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatal("postgres did not become ready")
	return ""
}

func applyCoreSchema(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, "services/core/migrations"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		raw, err := os.ReadFile(filepath.Join(root, "services/core/migrations", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
		up, _, _ := strings.Cut(strings.TrimPrefix(text, "-- +goose Up\n"), "\n-- +goose Down")
		psql(t, "core_onboarding_test", up)
	}
}

func psql(t *testing.T, database, sql string) {
	t.Helper()
	if strings.TrimSpace(psqlValue(t, database, sql)) == "" && strings.Contains(strings.ToLower(sql), "select") {
		t.Fatalf("empty SQL result for %s", sql)
	}
}

func psqlValue(t *testing.T, database, sql string) string {
	t.Helper()
	cmd := exec.Command("docker", "exec", "-i", "saldo-onboarding-chain-"+strings.ToLower(t.Name()), "psql", "-U", "onboarding_test", "-d", database, "-tA", "-v", "ON_ERROR_STOP=1")
	cmd.Stdin = strings.NewReader(sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sql: %v\n%s\n%s", err, sql, out)
	}
	return strings.TrimSpace(string(out))
}

func runChecked(t *testing.T, binary, dir string, env []string) {
	t.Helper()
	cmd := exec.Command(binary)
	cmd.Dir = dir
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH")}, env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", binary, err, out)
	}
}

func reservedPort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if closeErr := listener.Close(); err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
	return port
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("test file path unavailable")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../.."))
}

func verificationCode(t *testing.T, challengeID string, key []byte) string {
	t.Helper()
	row := psqlValue(t, "auth_onboarding_test", "SELECT user_id::text || ' ' || encode(code_hmac, 'hex') FROM verification_challenges WHERE id = '"+challengeID+"'")
	userID, encoded, ok := strings.Cut(row, " ")
	stored, err := hex.DecodeString(encoded)
	if !ok || !challengeIDPattern.MatchString(userID) || err != nil || len(stored) != sha256.Size {
		t.Fatalf("verification challenge %s has no recoverable code", challengeID)
	}
	for n := range 1000000 {
		code := fmt.Sprintf("%06d", n)
		if hmac.Equal(verificationMAC(key, challengeID, userID, code), stored) {
			return code
		}
	}
	t.Fatalf("stored verification HMAC does not match the test key")
	return ""
}

func verificationMAC(key []byte, challengeID, userID, code string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("email_verification\x00" + challengeID + "\x00" + userID + "\x00" + code))
	return mac.Sum(nil)
}

func mustOutput(t *testing.T, name string, args ...string) []byte {
	t.Helper()
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", name, err, out)
	}
	return out
}

func nested(body map[string]any, key, child string) string {
	value, _ := body[key].(map[string]any)
	text, _ := value[child].(string)
	return text
}
