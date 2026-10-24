package testauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authv1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/auth/v1"
)

func fixtureWrite(t *testing.T, server *Server) *authv1.WriteContext {
	t.Helper()
	response, err := server.GetSecurityContext(t.Context(), &authv1.GetSecurityContextRequest{})
	if err != nil {
		t.Fatal(err)
	}
	return &authv1.WriteContext{
		Browser:   &authv1.BrowserContext{CookieToken: response.GetContext().GetCookieToken()},
		CsrfToken: response.GetContext().GetCsrfToken(),
		Client:    &authv1.ClientContext{ClientIp: "127.0.0.1", RequestId: "fixture-test"},
	}
}

func fixtureReason(t *testing.T, err error, code codes.Code, reason authv1.AuthErrorReason) {
	t.Helper()
	if status.Code(err) != code {
		t.Fatalf("status=%s want %s: %v", status.Code(err), code, err)
	}
	for _, detail := range status.Convert(err).Details() {
		if typed, ok := detail.(*authv1.AuthErrorDetail); ok {
			if typed.GetReason() != reason {
				t.Fatalf("reason=%s want %s", typed.GetReason(), reason)
			}
			return
		}
	}
	t.Fatal("missing typed AuthErrorDetail")
}

func TestSecurityContextsPreserveThenRotateExpired(t *testing.T) {
	server := New()
	first, err := server.GetSecurityContext(t.Context(), &authv1.GetSecurityContextRequest{})
	if err != nil {
		t.Fatal(err)
	}
	security := first.GetContext()
	for _, token := range []string{security.GetCookieToken(), security.GetCsrfToken()} {
		decoded, decodeErr := base64.RawURLEncoding.DecodeString(token)
		if decodeErr != nil || len(decoded) != 32 {
			t.Fatalf("expected 32 random bytes encoded as base64url: length=%d err=%v", len(decoded), decodeErr)
		}
	}
	remaining := time.Until(security.GetCookieExpiresAt().AsTime())
	if remaining < 9*time.Minute || remaining > 10*time.Minute {
		t.Fatalf("pre-auth lifetime=%s", remaining)
	}
	request := &authv1.GetSecurityContextRequest{Context: &authv1.BrowserContext{CookieToken: security.GetCookieToken()}}
	again, err := server.GetSecurityContext(t.Context(), request)
	if err != nil || again.GetContext().GetCookieToken() != security.GetCookieToken() || again.GetContext().GetCsrfToken() != security.GetCsrfToken() {
		t.Fatalf("live context was changed: %v", err)
	}
	server.mu.Lock()
	state := server.contexts[security.GetCookieToken()]
	state.expires = time.Now().Add(-time.Second)
	server.contexts[security.GetCookieToken()] = state
	server.mu.Unlock()
	rotated, err := server.GetSecurityContext(t.Context(), request)
	if err != nil || rotated.GetContext().GetCookieToken() == security.GetCookieToken() {
		t.Fatalf("expired context was reused: %v", err)
	}
	server.mu.Lock()
	_, stale := server.contexts[security.GetCookieToken()]
	server.mu.Unlock()
	if stale {
		t.Fatal("expired context retained")
	}
}

func TestLoginRotationAndRequestSnapshots(t *testing.T) {
	server := New()
	write := fixtureWrite(t, server)
	_, err := server.GetCurrentAccount(t.Context(), &authv1.GetCurrentAccountRequest{Context: write.GetBrowser()})
	fixtureReason(t, err, codes.Unauthenticated, authv1.AuthErrorReason_AUTH_ERROR_REASON_AUTH_REQUIRED)
	request := &authv1.LoginRequest{Context: write, Email: " DEMO@EXAMPLE.TEST ", Password: "incorrect-password"}
	_, err = server.Login(t.Context(), request)
	fixtureReason(t, err, codes.Unauthenticated, authv1.AuthErrorReason_AUTH_ERROR_REASON_INVALID_CREDENTIALS)
	request.Password = Password
	login, err := server.Login(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if login.GetUser().GetEmail() != Email || login.GetUser().GetEmailVerified() || login.GetContext().GetCookieToken() == write.GetBrowser().GetCookieToken() {
		t.Fatal("login did not create a fresh unverified synthetic session")
	}
	if time.Until(login.GetContext().GetCookieExpiresAt().AsTime()) < 23*time.Hour {
		t.Fatal("authenticated session has pre-auth lifetime")
	}
	_, err = server.Login(t.Context(), request)
	fixtureReason(t, err, codes.PermissionDenied, authv1.AuthErrorReason_AUTH_ERROR_REASON_CSRF_INVALID)
	me, err := server.GetCurrentAccount(t.Context(), &authv1.GetCurrentAccountRequest{Context: &authv1.BrowserContext{CookieToken: login.GetContext().GetCookieToken()}})
	if err != nil || me.GetUser().GetId() != login.GetUser().GetId() {
		t.Fatalf("live session lookup failed: %v", err)
	}
	request.Email = "mutated@example.test"
	snapshots := server.Requests("Login")
	if len(snapshots) != 3 {
		t.Fatalf("recorded %d calls, want 3", len(snapshots))
	}
	first, ok := snapshots[0].(*authv1.LoginRequest)
	if !ok || first.GetPassword() != "incorrect-password" || first.GetEmail() != " DEMO@EXAMPLE.TEST " {
		t.Fatal("incoming request snapshot changed after caller mutation")
	}
	first.Email = "changed-snapshot@example.test"
	snapshot, ok := server.Requests("Login")[0].(*authv1.LoginRequest)
	if !ok || snapshot.GetEmail() != " DEMO@EXAMPLE.TEST " {
		t.Fatal("returned snapshots alias server state")
	}
}

func TestConsentAndChallengeLifecycle(t *testing.T) {
	server := New()
	write := fixtureWrite(t, server)
	consent, err := server.GetPersonalDataConsent(t.Context(), &authv1.GetPersonalDataConsentRequest{})
	if err != nil {
		t.Fatal(err)
	}
	document := consent.GetDocument()
	if document.GetDocumentSha256() != fmt.Sprintf("%x", sha256.Sum256([]byte(document.GetText()))) || document.GetDocumentVersion() != ConsentVersion {
		t.Fatal("document identity does not describe exact fixture text")
	}
	register := &authv1.RegisterRequest{Context: write, Email: Email, Password: Password, DisplayName: "Synthetic User"}
	_, err = server.Register(t.Context(), register)
	fixtureReason(t, err, codes.InvalidArgument, authv1.AuthErrorReason_AUTH_ERROR_REASON_CONSENT_REQUIRED)
	register.PersonalDataConsent = &authv1.ConsentAcceptance{Accepted: true, DocumentVersion: "old-version", DocumentSha256: document.GetDocumentSha256()}
	_, err = server.Register(t.Context(), register)
	fixtureReason(t, err, codes.Aborted, authv1.AuthErrorReason_AUTH_ERROR_REASON_CONSENT_DOCUMENT_CHANGED)
	register.PersonalDataConsent.DocumentVersion = document.GetDocumentVersion()
	registered, err := server.Register(t.Context(), register)
	if err != nil || registered.GetChallenge().GetChallengeId() != ChallengeID {
		t.Fatalf("registration acknowledgement failed: %v", err)
	}
	resend, err := server.ResendVerification(t.Context(), &authv1.ResendVerificationRequest{Context: write, Email: Email})
	if err != nil || resend.GetChallenge().GetChallengeId() == ChallengeID {
		t.Fatalf("resend did not rotate challenge: %v", err)
	}
	_, err = server.VerifyEmail(t.Context(), &authv1.VerifyEmailRequest{Context: write, ChallengeId: ChallengeID, Code: Code})
	fixtureReason(t, err, codes.InvalidArgument, authv1.AuthErrorReason_AUTH_ERROR_REASON_CODE_INVALID_OR_EXPIRED)
	_, err = server.VerifyEmail(t.Context(), &authv1.VerifyEmailRequest{Context: write, ChallengeId: resend.GetChallenge().GetChallengeId(), Code: "999999"})
	fixtureReason(t, err, codes.InvalidArgument, authv1.AuthErrorReason_AUTH_ERROR_REASON_CODE_INVALID_OR_EXPIRED)
	for range 2 {
		_, err = server.VerifyEmail(t.Context(), &authv1.VerifyEmailRequest{Context: write, ChallengeId: resend.GetChallenge().GetChallengeId(), Code: Code})
		if err != nil {
			t.Fatal("correct-code replay must be safe", err)
		}
	}
	login, err := server.Login(t.Context(), &authv1.LoginRequest{Context: write, Email: Email, Password: Password})
	if err != nil || !login.GetUser().GetEmailVerified() {
		t.Fatalf("verification state was not reflected in profile: %v", err)
	}
}

func TestPasswordResetRevokesSessionsAndConsumesToken(t *testing.T) {
	server := New()
	write := fixtureWrite(t, server)
	login, err := server.Login(t.Context(), &authv1.LoginRequest{Context: write, Email: Email, Password: Password})
	if err != nil {
		t.Fatal(err)
	}
	write = &authv1.WriteContext{Browser: &authv1.BrowserContext{CookieToken: login.GetContext().GetCookieToken()}, CsrfToken: login.GetContext().GetCsrfToken()}
	_, err = server.ForgotPassword(t.Context(), &authv1.ForgotPasswordRequest{Context: write, Email: Email})
	if err != nil {
		t.Fatal(err)
	}
	reset := &authv1.ResetPasswordRequest{Context: write, Token: "invalid-reset-token", NewPassword: "new-synthetic-password"}
	_, err = server.ResetPassword(t.Context(), reset)
	fixtureReason(t, err, codes.InvalidArgument, authv1.AuthErrorReason_AUTH_ERROR_REASON_TOKEN_INVALID_OR_EXPIRED)
	reset.Token = ResetToken
	if _, err = server.ResetPassword(t.Context(), reset); err != nil {
		t.Fatal(err)
	}
	_, err = server.GetCurrentAccount(t.Context(), &authv1.GetCurrentAccountRequest{Context: write.GetBrowser()})
	fixtureReason(t, err, codes.Unauthenticated, authv1.AuthErrorReason_AUTH_ERROR_REASON_AUTH_REQUIRED)
	reset.Context = fixtureWrite(t, server)
	_, err = server.ResetPassword(t.Context(), reset)
	fixtureReason(t, err, codes.InvalidArgument, authv1.AuthErrorReason_AUTH_ERROR_REASON_TOKEN_INVALID_OR_EXPIRED)
	_, err = server.Login(t.Context(), &authv1.LoginRequest{Context: reset.Context, Email: Email, Password: Password})
	fixtureReason(t, err, codes.Unauthenticated, authv1.AuthErrorReason_AUTH_ERROR_REASON_INVALID_CREDENTIALS)
	login, err = server.Login(t.Context(), &authv1.LoginRequest{Context: reset.Context, Email: Email, Password: reset.GetNewPassword()})
	if err != nil {
		t.Fatal(err)
	}
	write = &authv1.WriteContext{Browser: &authv1.BrowserContext{CookieToken: login.GetContext().GetCookieToken()}, CsrfToken: login.GetContext().GetCsrfToken()}
	if _, err = server.Logout(t.Context(), &authv1.LogoutRequest{Context: write}); err != nil {
		t.Fatal(err)
	}
	_, err = server.GetCurrentAccount(t.Context(), &authv1.GetCurrentAccountRequest{Context: write.GetBrowser()})
	fixtureReason(t, err, codes.Unauthenticated, authv1.AuthErrorReason_AUTH_ERROR_REASON_AUTH_REQUIRED)
	if _, err = server.Logout(t.Context(), &authv1.LogoutRequest{Context: fixtureWrite(t, server)}); err != nil {
		t.Fatal("pre-auth logout must succeed", err)
	}
}

func TestEveryRPCPropagatesInjectedFailureAndRejectsMissingCSRF(t *testing.T) {
	server := New()
	calls := map[string]func() error{
		"GetPersonalDataConsent": func() error {
			_, err := server.GetPersonalDataConsent(t.Context(), &authv1.GetPersonalDataConsentRequest{})
			return err
		},
		"GetSecurityContext": func() error {
			_, err := server.GetSecurityContext(t.Context(), &authv1.GetSecurityContextRequest{})
			return err
		},
		"Register":    func() error { _, err := server.Register(t.Context(), &authv1.RegisterRequest{}); return err },
		"Login":       func() error { _, err := server.Login(t.Context(), &authv1.LoginRequest{}); return err },
		"VerifyEmail": func() error { _, err := server.VerifyEmail(t.Context(), &authv1.VerifyEmailRequest{}); return err },
		"ResendVerification": func() error {
			_, err := server.ResendVerification(t.Context(), &authv1.ResendVerificationRequest{})
			return err
		},
		"ForgotPassword": func() error {
			_, err := server.ForgotPassword(t.Context(), &authv1.ForgotPasswordRequest{})
			return err
		},
		"ResetPassword": func() error { _, err := server.ResetPassword(t.Context(), &authv1.ResetPasswordRequest{}); return err },
		"Logout":        func() error { _, err := server.Logout(t.Context(), &authv1.LogoutRequest{}); return err },
		"GetCurrentAccount": func() error {
			_, err := server.GetCurrentAccount(t.Context(), &authv1.GetCurrentAccountRequest{})
			return err
		},
	}
	for method, call := range calls {
		t.Run(method, func(t *testing.T) {
			server.SetFailure(method, status.Error(codes.Unavailable, "synthetic outage"))
			if err := call(); status.Code(err) != codes.Unavailable {
				t.Fatalf("injected failure was swallowed: %v", err)
			}
			server.SetFailure(method, nil)
			if method != "GetPersonalDataConsent" && method != "GetSecurityContext" && method != "GetCurrentAccount" {
				fixtureReason(t, call(), codes.PermissionDenied, authv1.AuthErrorReason_AUTH_ERROR_REASON_CSRF_INVALID)
			}
		})
	}
}

func TestDelayCancellationAndConcurrentRecording(t *testing.T) {
	server := New()
	server.SetDelay("GetPersonalDataConsent", time.Hour)
	ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
	defer cancel()
	_, err := server.GetPersonalDataConsent(ctx, &authv1.GetPersonalDataConsentRequest{})
	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("delay ignored deadline: %v", err)
	}
	server.SetDelay("GetPersonalDataConsent", 0)
	canceled, stop := context.WithCancel(t.Context())
	stop()
	_, err = server.GetPersonalDataConsent(canceled, &authv1.GetPersonalDataConsentRequest{})
	if status.Code(err) != codes.Canceled {
		t.Fatalf("canceled call succeeded: %v", err)
	}
	server.SetDelay("GetPersonalDataConsent", time.Millisecond)
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			response, callErr := server.GetPersonalDataConsent(t.Context(), &authv1.GetPersonalDataConsentRequest{})
			if callErr != nil || response.GetDocument().GetText() != ConsentText {
				t.Errorf("concurrent read failed: %v", callErr)
			}
		})
	}
	workers.Wait()
	if len(server.Requests("GetPersonalDataConsent")) != 10 {
		t.Fatal("concurrent calls were lost")
	}
}
