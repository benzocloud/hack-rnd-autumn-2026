//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/app"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/config"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/service/account"
)

const accountPassword = "correct horse battery staple"

type accountOperations interface {
	GetPersonalDataConsent(context.Context) (account.Document, error)
	GetSecurityContext(context.Context, string) (account.SecurityContext, error)
	Register(context.Context, account.RegisterInput) (account.Challenge, error)
	Login(context.Context, account.LoginInput) (account.LoginResult, error)
	VerifyEmail(context.Context, account.VerifyEmailInput) error
	ResendVerification(context.Context, account.WriteContext, string) (account.Challenge, error)
	Logout(context.Context, account.WriteContext) error
	GetCurrentAccount(context.Context, string) (account.User, error)
	AuthorizeCompany(context.Context, account.AuthorizeCompanyInput) (account.Authorization, error)
	ListMemberships(context.Context, string, int, string) (account.MembershipPage, error)
}

func accountConfiguration(t *testing.T, db *pgx.Conn, demo bool) config.Config {
	t.Helper()
	raw, err := url.Parse(os.Getenv("AUTH_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal("invalid scratch database URL")
	}
	var schema string
	if err := db.QueryRow(t.Context(), "SELECT current_schema()").Scan(&schema); err != nil {
		t.Fatal("cannot locate isolated Auth test schema")
	}
	query := raw.Query()
	query.Set("search_path", schema)
	raw.RawQuery = query.Encode()
	enabled := "false"
	if demo {
		enabled = "true"
	}
	env := map[string]string{
		"AUTH_ENV": "local", "AUTH_DATABASE_URL": raw.String(),
		"AUTH_QUERY_TIMEOUT": "5s", "AUTH_STARTUP_TIMEOUT": "5s",
		"AUTH_GRPC_ADDRESS":   "127.0.0.1:9092",
		"AUTH_GRPC_CERT_FILE": "unused-cert.pem", "AUTH_GRPC_KEY_FILE": "unused-key.pem", "AUTH_GRPC_CA_FILE": "unused-ca.pem",
		"AUTH_CODE_HMAC_KEY":           base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32)),
		"AUTH_CSRF_HMAC_KEY":           base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32)),
		"AUTH_DEMO_EMAIL_VERIFICATION": enabled,
		"AUTH_DEMO_TEST_EMAILS":        "alice@example.test,bob@example.test",
	}
	cfg, err := config.Load(func(name string) string { return env[name] })
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func openAccounts(t *testing.T, cfg config.Config) accountOperations {
	t.Helper()
	application, err := app.New(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	if application.Account == nil {
		t.Fatal("account service is unavailable with explicit RPC configuration")
	}
	return application.Account
}

func accountFixture(t *testing.T, demo bool) (config.Config, *pgx.Conn, accountOperations) {
	t.Helper()
	_, db := scratch(t)
	cfg := accountConfiguration(t, db, demo)
	if err := app.Migrate(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	return cfg, db, openAccounts(t, cfg)
}

func accountWrite(value account.SecurityContext) account.WriteContext {
	return account.WriteContext{CookieToken: value.CookieToken, CSRFToken: value.CSRFToken}
}

func registerAccount(t *testing.T, service accountOperations, email string) (account.Challenge, account.SecurityContext) {
	t.Helper()
	browser, err := service.GetSecurityContext(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	document, err := service.GetPersonalDataConsent(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := service.Register(t.Context(), account.RegisterInput{
		Context: accountWrite(browser), Email: email, Password: accountPassword, DisplayName: "Демо пользователь",
		ConsentAccepted: true, ConsentVersion: document.Version, ConsentSHA256: document.SHA256,
	})
	if err != nil {
		t.Fatal(err)
	}
	return challenge, browser
}

func loginAccount(t *testing.T, service accountOperations, browser account.SecurityContext, email string) account.LoginResult {
	t.Helper()
	login, err := service.Login(t.Context(), account.LoginInput{Context: accountWrite(browser), Email: email, Password: accountPassword})
	if err != nil {
		t.Fatal(err)
	}
	return login
}

func execAccountFixture(t *testing.T, db *pgx.Conn, sql string, args ...any) {
	t.Helper()
	if _, err := db.Exec(t.Context(), sql, args...); err != nil {
		t.Fatal("Auth test fixture SQL failed")
	}
}

// Only this test fixture writes a known verification MAC; application responses never expose codes.
func setKnownVerificationCode(t *testing.T, db *pgx.Conn, cfg config.Config, challenge uuid.UUID, code string) {
	t.Helper()
	var userID uuid.UUID
	if err := db.QueryRow(t.Context(), "SELECT user_id FROM verification_challenges WHERE id=$1", challenge).Scan(&userID); err != nil {
		t.Fatal("cannot read verification fixture")
	}
	mac := hmac.New(sha256.New, cfg.CodeHMACKey())
	_, _ = mac.Write([]byte("email_verification\x00" + challenge.String() + "\x00" + userID.String() + "\x00" + code))
	execAccountFixture(t, db, "UPDATE verification_challenges SET code_hmac=$2 WHERE id=$1", challenge, mac.Sum(nil))
}

func TestAccountDemoSessionsConsentAndCompanyScope(t *testing.T) {
	_, db, service := accountFixture(t, true)
	document, err := service.GetPersonalDataConsent(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	browser, err := service.GetSecurityContext(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	registration := account.RegisterInput{Context: accountWrite(browser), Email: "alice@example.test", Password: accountPassword,
		DisplayName: "Alice", ConsentVersion: document.Version, ConsentSHA256: document.SHA256}
	if _, err := service.Register(t.Context(), registration); !errors.Is(err, account.ErrConsentRequired) {
		t.Fatal("registration did not require separate consent")
	}
	registration.ConsentAccepted = true
	for _, malformed := range []struct{ version, hash string }{
		{"", document.SHA256},
		{strings.Repeat("v", 129), document.SHA256},
		{document.Version, "stale"},
		{document.Version, strings.Repeat("A", 64)},
		{document.Version, strings.Repeat("z", 64)},
	} {
		invalid := registration
		invalid.ConsentVersion, invalid.ConsentSHA256 = malformed.version, malformed.hash
		if _, err := service.Register(t.Context(), invalid); !errors.Is(err, account.ErrValidation) {
			t.Fatal("malformed consent metadata lost validation error")
		}
	}
	registration.ConsentSHA256 = strings.Repeat("0", 64)
	if _, err := service.Register(t.Context(), registration); !errors.Is(err, account.ErrConsentChanged) {
		t.Fatal("registration accepted stale consent evidence")
	}
	var count int
	if err := db.QueryRow(t.Context(), "SELECT count(*) FROM users").Scan(&count); err != nil || count != 0 {
		t.Fatal("rejected consent created an account")
	}
	registration.ConsentSHA256 = document.SHA256
	challenge, err := service.Register(t.Context(), registration)
	if err != nil {
		t.Fatal(err)
	}
	var passwordHash string
	if err := db.QueryRow(t.Context(), "SELECT credentials.password_hash FROM credentials JOIN users ON users.id=credentials.user_id WHERE users.email=$1", registration.Email).Scan(&passwordHash); err != nil || !strings.HasPrefix(passwordHash, "$argon2id$v=19$m=65536,t=3,p=1$") {
		t.Fatal("persisted password hash does not use the agreed Argon2id parameters")
	}
	if err := service.VerifyEmail(t.Context(), account.VerifyEmailInput{Context: accountWrite(browser), ChallengeID: challenge.ID, Code: "000000"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetCurrentAccount(t.Context(), browser.CookieToken); !errors.Is(err, account.ErrAuthRequired) {
		t.Fatal("email acknowledgement created an authenticated session")
	}
	wrong := account.LoginInput{Context: accountWrite(browser), Email: registration.Email, Password: "a wrong but long password"}
	if _, err := service.Login(t.Context(), wrong); !errors.Is(err, account.ErrInvalidCredentials) {
		t.Fatal("demo email mode bypassed password verification")
	}
	duplicate := registration
	duplicate.Password = "a replacement long password"
	inert, err := service.Register(t.Context(), duplicate)
	if err != nil || inert.ID == challenge.ID {
		t.Fatal("duplicate registration did not return a fresh inert challenge")
	}
	if err := db.QueryRow(t.Context(), "SELECT count(*) FROM consent_evidence").Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate registration changed consent history")
	}
	var storedVersion, storedHash, storedText string
	if err := db.QueryRow(t.Context(), "SELECT document_version,document_sha256,document_text FROM consent_evidence").Scan(&storedVersion, &storedHash, &storedText); err != nil || storedVersion != document.Version || storedHash != document.SHA256 || storedText != document.Text {
		t.Fatal("consent evidence did not preserve the server's exact active document")
	}
	alice := loginAccount(t, service, browser, registration.Email)
	if !alice.User.EmailVerified || alice.Context.CookieToken == browser.CookieToken || alice.Context.CSRFToken == browser.CSRFToken {
		t.Fatal("login did not rotate the real server context")
	}
	if _, err := service.GetCurrentAccount(t.Context(), browser.CookieToken); !errors.Is(err, account.ErrAuthRequired) {
		t.Fatal("rotated pre-auth cookie remained usable")
	}
	if _, err := service.Login(t.Context(), account.LoginInput{Context: accountWrite(alice.Context), Email: registration.Email, Password: duplicate.Password}); !errors.Is(err, account.ErrInvalidCredentials) {
		t.Fatal("duplicate registration replaced existing credentials")
	}
	bobChallenge, bobBrowser := registerAccount(t, service, "bob@example.test")
	beforeBob := loginAccount(t, service, bobBrowser, "bob@example.test")
	if err := service.VerifyEmail(t.Context(), account.VerifyEmailInput{Context: accountWrite(alice.Context), ChallengeID: bobChallenge.ID, Code: "123456"}); !errors.Is(err, account.ErrCodeInvalid) {
		t.Fatal("an authenticated actor verified another account")
	}
	if err := db.QueryRow(t.Context(), "SELECT failed_attempts FROM verification_challenges WHERE id=$1", bobChallenge.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("a foreign actor changed another account's challenge attempts")
	}
	if err := service.VerifyEmail(t.Context(), account.VerifyEmailInput{Context: accountWrite(beforeBob.Context), ChallengeID: bobChallenge.ID, Code: "987654"}); err != nil {
		t.Fatal(err)
	}
	bob := beforeBob
	company, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	foreignCompany, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	// Operator-seeded synthetic memberships exercise Auth scope without pretending Core bootstrap exists.
	execAccountFixture(t, db, "INSERT INTO memberships(company_id,user_id,role) VALUES($1,$2,'owner'),($1,$3,'financier'),($4,$3,'owner')", company, alice.User.ID, bob.User.ID, foreignCompany)
	ownerRead := account.AuthorizeCompanyInput{CookieToken: alice.Context.CookieToken, CompanyID: company, Permission: "financial_read"}
	auth, err := service.AuthorizeCompany(t.Context(), ownerRead)
	if err != nil || auth.ActorID != alice.User.ID || auth.Role != "owner" {
		t.Fatal("owner authorization did not come from the live server session")
	}
	ownerRead.CompanyID = foreignCompany
	if _, err := service.AuthorizeCompany(t.Context(), ownerRead); !errors.Is(err, account.ErrNotFound) {
		t.Fatal("company scope exposed a foreign company")
	}
	for _, tc := range []struct {
		name, permission, csrf string
		write                  bool
		want                   error
	}{
		{name: "read own company", permission: "financial_read"},
		{name: "real financial write", permission: "financial_write", write: true, csrf: bob.Context.CSRFToken},
		{name: "write without context", permission: "financial_write", want: account.ErrCSRFInvalid},
		{name: "write without csrf", permission: "financial_write", write: true, want: account.ErrCSRFInvalid},
		{name: "owner-only company update", permission: "update", write: true, csrf: bob.Context.CSRFToken, want: account.ErrForbidden},
		{name: "owner-only membership management", permission: "members_manage", write: true, csrf: bob.Context.CSRFToken, want: account.ErrForbidden},
		{name: "unknown permission", permission: "browser-selected-admin", want: account.ErrValidation},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.AuthorizeCompany(t.Context(), account.AuthorizeCompanyInput{CookieToken: bob.Context.CookieToken,
				CSRFToken: tc.csrf, CompanyID: company, Permission: tc.permission, Write: tc.write})
			if !errors.Is(err, tc.want) {
				t.Fatalf("unexpected permission result: %v", err)
			}
		})
	}
	ownerWrite := account.AuthorizeCompanyInput{CookieToken: alice.Context.CookieToken, CompanyID: company, Permission: "financial_write", Write: true, CSRFToken: browser.CSRFToken}
	if _, err := service.AuthorizeCompany(t.Context(), ownerWrite); !errors.Is(err, account.ErrCSRFInvalid) {
		t.Fatal("CSRF from the rotated context authorized a write")
	}
	ownerWrite.CSRFToken = alice.Context.CSRFToken
	if _, err := service.AuthorizeCompany(t.Context(), ownerWrite); err != nil {
		t.Fatal(err)
	}
	alicePage, err := service.ListMemberships(t.Context(), alice.Context.CookieToken, 1, "")
	if err != nil || len(alicePage.Items) != 1 || alicePage.Items[0].CompanyID != company {
		t.Fatal("membership list escaped the actor scope")
	}
	bobPage, err := service.ListMemberships(t.Context(), bob.Context.CookieToken, 1, "")
	if err != nil || bobPage.NextCursor == "" {
		t.Fatal("membership pagination omitted a real next page")
	}
	if _, err := service.ListMemberships(t.Context(), alice.Context.CookieToken, 1, bobPage.NextCursor); !errors.Is(err, account.ErrInvalidCursor) {
		t.Fatal("another actor reused a membership cursor")
	}
	if err := service.Logout(t.Context(), accountWrite(alice.Context)); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetCurrentAccount(t.Context(), alice.Context.CookieToken); !errors.Is(err, account.ErrAuthRequired) {
		t.Fatal("logout left the real server session active")
	}
	execAccountFixture(t, db, "UPDATE users SET credential_version=credential_version+1 WHERE id=$1", bob.User.ID)
	if _, err := service.GetCurrentAccount(t.Context(), bob.Context.CookieToken); !errors.Is(err, account.ErrAuthRequired) {
		t.Fatal("a stale credential version retained a live session")
	}
}

func TestAccountStrictCodeAndDemoProofIsolation(t *testing.T) {
	cfg, db, strict := accountFixture(t, false)
	challenge, browser := registerAccount(t, strict, "alice@example.test")
	setKnownVerificationCode(t, db, cfg, challenge.ID, "654321")
	input := account.VerifyEmailInput{Context: accountWrite(browser), ChallengeID: challenge.ID, Code: "000000"}
	if err := strict.VerifyEmail(t.Context(), input); !errors.Is(err, account.ErrCodeInvalid) {
		t.Fatal("default strict mode accepted a wrong six-digit code")
	}
	var attempts int
	if err := db.QueryRow(t.Context(), "SELECT failed_attempts FROM verification_challenges WHERE id=$1", challenge.ID).Scan(&attempts); err != nil || attempts != 1 {
		t.Fatal("wrong-code rejection rolled back the attempt counter")
	}
	input.Code = "654321"
	if err := strict.VerifyEmail(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	if err := strict.VerifyEmail(t.Context(), input); err != nil {
		t.Fatal("strict verification acknowledgement was not idempotent")
	}
	input.Code = "111111"
	if err := strict.VerifyEmail(t.Context(), input); !errors.Is(err, account.ErrCodeInvalid) {
		t.Fatal("consumed strict challenge accepted a different code")
	}
	demo := openAccounts(t, accountConfiguration(t, db, true))
	bobChallenge, bobBrowser := registerAccount(t, demo, "bob@example.test")
	if err := demo.VerifyEmail(t.Context(), account.VerifyEmailInput{Context: accountWrite(bobBrowser), ChallengeID: bobChallenge.ID, Code: "000007"}); err != nil {
		t.Fatal(err)
	}
	bob := loginAccount(t, demo, bobBrowser, "bob@example.test")
	strictBob, err := strict.GetCurrentAccount(t.Context(), bob.Context.CookieToken)
	if err != nil || strictBob.EmailVerified {
		t.Fatal("strict mode treated a persisted demo acknowledgement as real email proof")
	}
	company, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	execAccountFixture(t, db, "INSERT INTO memberships(company_id,user_id,role) VALUES($1,$2,'owner')", company, bob.User.ID)
	if _, err := strict.AuthorizeCompany(t.Context(), account.AuthorizeCompanyInput{CookieToken: bob.Context.CookieToken, CompanyID: company, Permission: "financial_read"}); !errors.Is(err, account.ErrEmailUnverified) {
		t.Fatal("turning off demo promoted mocked email proof into financial access")
	}
	replacement, err := strict.ResendVerification(t.Context(), accountWrite(bob.Context), "bob@example.test")
	if err != nil || replacement.ID == bobChallenge.ID {
		t.Fatal("strict verification could not replace a demo challenge")
	}
	setKnownVerificationCode(t, db, cfg, replacement.ID, "345678")
	if err := strict.VerifyEmail(t.Context(), account.VerifyEmailInput{Context: accountWrite(bob.Context), ChallengeID: replacement.ID, Code: "345678"}); err != nil {
		t.Fatal(err)
	}
	if verified, err := strict.GetCurrentAccount(t.Context(), bob.Context.CookieToken); err != nil || !verified.EmailVerified {
		t.Fatal("strict challenge did not establish its separate verification result")
	}
}

func TestAccountDemoRejectsInvalidChallengeStateAndCodeShape(t *testing.T) {
	_, db, service := accountFixture(t, true)
	challenge, browser := registerAccount(t, service, "alice@example.test")
	for _, code := range []string{"12345", "1234567", "12 345", "１２３４５６", "١٢٣٤٥٦"} {
		t.Run("malformed "+code, func(t *testing.T) {
			if err := service.VerifyEmail(t.Context(), account.VerifyEmailInput{Context: accountWrite(browser), ChallengeID: challenge.ID, Code: code}); !errors.Is(err, account.ErrValidation) {
				t.Fatal("demo accepted something other than six ASCII digits")
			}
		})
	}
	if err := service.VerifyEmail(t.Context(), account.VerifyEmailInput{Context: accountWrite(browser), ChallengeID: uuid.New(), Code: "123456"}); !errors.Is(err, account.ErrCodeInvalid) {
		t.Fatal("demo accepted an unknown challenge")
	}
	for _, tc := range []struct{ name, sql string }{
		{name: "expired", sql: "UPDATE verification_challenges SET created_at=now()-interval '11 minutes',expires_at=now()-interval '1 minute' WHERE id=$1"},
		{name: "revoked", sql: "UPDATE verification_challenges SET revoked_at=now() WHERE id=$1"},
		{name: "blocked", sql: "UPDATE verification_challenges SET failed_attempts=5 WHERE id=$1"},
		{name: "unknown key", sql: "UPDATE verification_challenges SET code_key_id='retired' WHERE id=$1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current, err := service.ResendVerification(t.Context(), accountWrite(browser), "alice@example.test")
			if err != nil {
				t.Fatal(err)
			}
			execAccountFixture(t, db, tc.sql, current.ID)
			if err := service.VerifyEmail(t.Context(), account.VerifyEmailInput{Context: accountWrite(browser), ChallengeID: current.ID, Code: "123456"}); !errors.Is(err, account.ErrCodeInvalid) {
				t.Fatal("demo ignored a challenge lifecycle guard")
			}
		})
	}
}

func TestAccountConcurrentDemoAcknowledgementsDoNotCreateSessions(t *testing.T) {
	_, db, service := accountFixture(t, true)
	challenge, browser := registerAccount(t, service, "alice@example.test")
	results := make(chan error, 2)
	for range 2 {
		go func() {
			results <- service.VerifyEmail(t.Context(), account.VerifyEmailInput{Context: accountWrite(browser), ChallengeID: challenge.ID, Code: "001234"})
		}()
	}
	var firstError error
	for range 2 {
		if err := <-results; err != nil && firstError == nil {
			firstError = err
		}
	}
	if firstError != nil {
		t.Fatal(firstError)
	}
	var consumed, sessions int
	if err := db.QueryRow(t.Context(), "SELECT count(*) FROM verification_challenges WHERE id=$1 AND consumed_at IS NOT NULL", challenge.ID).Scan(&consumed); err != nil || consumed != 1 {
		t.Fatal("concurrent acknowledgements did not consume one challenge")
	}
	if err := db.QueryRow(t.Context(), "SELECT count(*) FROM sessions").Scan(&sessions); err != nil || sessions != 0 {
		t.Fatal("verification acknowledgement created a session")
	}
}

func TestAccountVerificationBudgetSurvivesResendAndDemoSwitch(t *testing.T) {
	cfg, db, strict := accountFixture(t, false)
	challenge, browser := registerAccount(t, strict, "alice@example.test")
	for attempt := range 5 {
		if attempt > 0 {
			next, err := strict.ResendVerification(t.Context(), accountWrite(browser), "alice@example.test")
			if err != nil {
				t.Fatal(err)
			}
			challenge = next
		}
		setKnownVerificationCode(t, db, cfg, challenge.ID, "654321")
		if err := strict.VerifyEmail(t.Context(), account.VerifyEmailInput{Context: accountWrite(browser), ChallengeID: challenge.ID, Code: "000000"}); !errors.Is(err, account.ErrCodeInvalid) {
			t.Fatal("strict verification accepted a wrong code")
		}
	}
	var failures int
	if err := db.QueryRow(t.Context(), "SELECT verification_failures FROM users WHERE email='alice@example.test'").Scan(&failures); err != nil || failures != 5 {
		t.Fatal("resend reset the user's durable verification budget")
	}
	next, err := strict.ResendVerification(t.Context(), accountWrite(browser), "alice@example.test")
	if err != nil {
		t.Fatal(err)
	}
	setKnownVerificationCode(t, db, cfg, next.ID, "654321")
	input := account.VerifyEmailInput{Context: accountWrite(browser), ChallengeID: next.ID, Code: "654321"}
	if err := strict.VerifyEmail(t.Context(), input); !errors.Is(err, account.ErrCodeInvalid) {
		t.Fatal("a fresh challenge bypassed the exhausted user budget")
	}
	demo := openAccounts(t, accountConfiguration(t, db, true))
	if err := demo.VerifyEmail(t.Context(), input); !errors.Is(err, account.ErrCodeInvalid) {
		t.Fatal("enabling demo bypassed the exhausted user budget")
	}
	execAccountFixture(t, db, "UPDATE users SET verification_window_started_at=now()-interval '16 minutes' WHERE email='alice@example.test'")
	if err := strict.VerifyEmail(t.Context(), input); err != nil {
		t.Fatal("verification remained blocked after the user budget window expired")
	}
}

func TestAccountConsentRegistryRotationRejectsStaleAcceptance(t *testing.T) {
	cfg, db, service := accountFixture(t, true)
	original, err := service.GetPersonalDataConsent(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	_, _ = registerAccount(t, service, "alice@example.test")
	browser, err := service.GetSecurityContext(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	updated := account.Document{Text: original.Text + "\n\nТестовая редакция согласия.", Version: original.Version + "-rotation"}
	digest := sha256.Sum256([]byte(updated.Text))
	updated.SHA256 = hex.EncodeToString(digest[:])
	execAccountFixture(t, db, "UPDATE consent_documents SET document_version=$1,document_sha256=$2,document_text=$3 WHERE singleton", updated.Version, updated.SHA256, updated.Text)
	acceptance := account.RegisterInput{Context: accountWrite(browser), Email: "bob@example.test", Password: accountPassword, DisplayName: "Bob",
		ConsentAccepted: true, ConsentVersion: original.Version, ConsentSHA256: original.SHA256}
	if _, err := service.Register(t.Context(), acceptance); !errors.Is(err, account.ErrConsentChanged) {
		t.Fatal("stale consent created an account after the registry changed")
	}
	active, err := service.GetPersonalDataConsent(t.Context())
	if err != nil || active != updated {
		t.Fatal("consent RPC did not return the active database version")
	}
	restarted := openAccounts(t, cfg)
	active, err = restarted.GetPersonalDataConsent(t.Context())
	if err != nil || active != updated {
		t.Fatal("startup overwrote the active consent registry with the embedded bootstrap")
	}
	acceptance.ConsentVersion, acceptance.ConsentSHA256 = updated.Version, updated.SHA256
	if _, err := restarted.Register(t.Context(), acceptance); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []struct {
		email    string
		document account.Document
	}{{email: "alice@example.test", document: original}, {email: "bob@example.test", document: updated}} {
		t.Run(expected.email, func(t *testing.T) {
			var actual account.Document
			if err := db.QueryRow(t.Context(), "SELECT evidence.document_version,evidence.document_sha256,evidence.document_text FROM consent_evidence evidence JOIN users ON users.id=evidence.user_id WHERE users.email=$1", expected.email).Scan(&actual.Version, &actual.SHA256, &actual.Text); err != nil || actual != expected.document {
				t.Fatal("registration failed to preserve its own server-side consent snapshot")
			}
		})
	}
}
