//go:build integration

package integration_test

import (
	"testing"
	"time"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/app"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/service/account"
)

func TestUnavailableCoreClientDoesNotBreakRegisterVerifyLogin(t *testing.T) {
	cfg, _, _ := accountFixture(t, true)
	cfg.CoreAddress = "127.0.0.1:1"
	cfg.CoreServerName = "core.saldo.test"
	cfg.CoreTimeout = 100 * time.Millisecond
	// Existing RPC fixture paths do not contain files; client initialization fails.
	application, err := app.New(t.Context(), cfg)
	if err != nil {
		t.Fatal("optional Core dependency broke Auth startup", err)
	}
	t.Cleanup(application.Close)
	challenge, browser := registerAccount(t, application.Account, "alice@example.test")
	if err := application.Account.VerifyEmail(t.Context(), account.VerifyEmailInput{Context: accountWrite(browser), ChallengeID: challenge.ID, Code: "000000"}); err != nil {
		t.Fatal(err)
	}
	login := loginAccount(t, application.Account, browser, "alice@example.test")
	if !login.User.EmailVerified {
		t.Fatal("verified login failed with unavailable Core")
	}
	if _, err := application.Account.GetCurrentAccount(t.Context(), login.Context.CookieToken); err != nil {
		t.Fatal(err)
	}
}
