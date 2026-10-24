package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	authv1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/auth/v1"
)

type verificationModeAuth struct {
	authClient
	mode string
}

func (a verificationModeAuth) GetSecurityContext(context.Context, *authv1.GetSecurityContextRequest, ...grpc.CallOption) (*authv1.GetSecurityContextResponse, error) {
	return &authv1.GetSecurityContextResponse{Context: &authv1.SecurityContext{CookieToken: strings.Repeat("k", 43), CsrfToken: strings.Repeat("c", 43), CookieExpiresAt: timestamppb.New(time.Now().Add(time.Minute))}, EmailVerificationMode: a.mode}, nil
}
func TestCSRFReturnsOnlyServerOwnedVerificationMode(t *testing.T) {
	for _, mode := range []string{"strict", "demo", "", "browser-selected-mode"} {
		t.Run(mode, func(t *testing.T) {
			handler := New(verificationModeAuth{mode: mode}, "https://example.test", slog.New(slog.DiscardHandler), nil)
			request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/csrf?email_verification_mode=demo", nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if mode == "browser-selected-mode" {
				if response.Code != 503 || len(response.Result().Cookies()) != 0 {
					t.Fatal("invalid upstream mode changed the browser context")
				}
				return
			}
			if response.Code != 200 {
				t.Fatal(response.Code)
			}
			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if mode != "" && body["email_verification_mode"] != mode {
				t.Fatal("browser selected verification mode")
			}
			if mode == "" && body["email_verification_mode"] != nil {
				t.Fatal("missing capability was invented")
			}
			if strings.Contains(response.Body.String(), strings.Repeat("k", 43)) || body["cookie_token"] != nil {
				t.Fatal("private cookie leaked in JSON")
			}
			if len(response.Result().Cookies()) != 1 || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("missing cookie or cache protection")
			}
		})
	}
}
