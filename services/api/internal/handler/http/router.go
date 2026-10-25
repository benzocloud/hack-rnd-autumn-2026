package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"

	"google.golang.org/grpc"

	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"

	authv1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/auth/v1"
)

type authClient interface {
	BeginCompanyCreation(context.Context, *authv1.BeginCompanyCreationRequest, ...grpc.CallOption) (*authv1.BeginCompanyCreationResponse, error)
	CompleteCompanyCreation(context.Context, *authv1.CompleteCompanyCreationRequest, ...grpc.CallOption) (*authv1.CompleteCompanyCreationResponse, error)
	ListMemberships(context.Context, *authv1.ListMembershipsRequest, ...grpc.CallOption) (*authv1.ListMembershipsResponse, error)
	GetPersonalDataConsent(context.Context, *authv1.GetPersonalDataConsentRequest, ...grpc.CallOption) (*authv1.GetPersonalDataConsentResponse, error)
	GetSecurityContext(context.Context, *authv1.GetSecurityContextRequest, ...grpc.CallOption) (*authv1.GetSecurityContextResponse, error)
	GetCurrentAccount(context.Context, *authv1.GetCurrentAccountRequest, ...grpc.CallOption) (*authv1.GetCurrentAccountResponse, error)
	Register(context.Context, *authv1.RegisterRequest, ...grpc.CallOption) (*authv1.RegisterResponse, error)
	Login(context.Context, *authv1.LoginRequest, ...grpc.CallOption) (*authv1.LoginResponse, error)
	VerifyEmail(context.Context, *authv1.VerifyEmailRequest, ...grpc.CallOption) (*authv1.VerifyEmailResponse, error)
	ResendVerification(context.Context, *authv1.ResendVerificationRequest, ...grpc.CallOption) (*authv1.ResendVerificationResponse, error)
	ForgotPassword(context.Context, *authv1.ForgotPasswordRequest, ...grpc.CallOption) (*authv1.ForgotPasswordResponse, error)
	ResetPassword(context.Context, *authv1.ResetPasswordRequest, ...grpc.CallOption) (*authv1.ResetPasswordResponse, error)
	Logout(context.Context, *authv1.LogoutRequest, ...grpc.CallOption) (*authv1.LogoutResponse, error)
}

type handler struct {
	auth   authClient
	core   CoreClients
	origin string
	logger *slog.Logger
	ready  func(context.Context) bool
}

func New(auth authClient, origin string, logger *slog.Logger, ready func(context.Context) bool) http.Handler {
	return NewWithCore(auth, origin, logger, ready, CoreClients{})
}

// CoreClients is the composing application's typed transport boundary.
type CoreClients struct {
	Scenarios   corev1.ScenariosServiceClient
	Comparisons interface {
		CompareForecasts(context.Context, *corev1.CompareForecastsRequest, ...grpc.CallOption) (*corev1.CompareForecastsResponse, error)
	}
	Payments    corev1.PaymentsServiceClient
	Imports     corev1.ImportsServiceClient
	ImportSpool *ImportSpool
	Companies   interface {
		CreateCompanyBootstrap(context.Context, *corev1.CreateCompanyBootstrapRequest, ...grpc.CallOption) (*corev1.CreateCompanyBootstrapResponse, error)
		ReadCompany(context.Context, *corev1.ReadCompanyRequest, ...grpc.CallOption) (*corev1.ReadCompanyResponse, error)
	}
	Ledger interface {
		GetDefaultAccount(context.Context, *corev1.GetDefaultAccountRequest, ...grpc.CallOption) (*corev1.GetDefaultAccountResponse, error)
		CreateManualTransaction(context.Context, *corev1.CreateManualTransactionRequest, ...grpc.CallOption) (*corev1.CreateManualTransactionResponse, error)
		ListCategories(context.Context, *corev1.ListCategoriesRequest, ...grpc.CallOption) (*corev1.ListCategoriesResponse, error)
		ListTransactions(context.Context, *corev1.ListTransactionsRequest, ...grpc.CallOption) (*corev1.ListTransactionsResponse, error)
		UpdateTransactionCategory(context.Context, *corev1.UpdateTransactionCategoryRequest, ...grpc.CallOption) (*corev1.UpdateTransactionCategoryResponse, error)
	}
	Forecasts interface {
		ListForecasts(context.Context, *corev1.ListForecastsRequest, ...grpc.CallOption) (*corev1.ListForecastsResponse, error)
		CreateForecast(context.Context, *corev1.CreateForecastRequest, ...grpc.CallOption) (*corev1.CreateForecastResponse, error)
		GetForecast(context.Context, *corev1.GetForecastRequest, ...grpc.CallOption) (*corev1.GetForecastResponse, error)
	}
}

func NewWithCore(auth authClient, origin string, logger *slog.Logger, ready func(context.Context) bool, core CoreClients) http.Handler {
	h := &handler{auth: auth, core: core, origin: origin, logger: logger, ready: ready}
	routes := map[string]struct {
		method string
		fn     http.HandlerFunc
	}{
		"/api/v1/auth/personal-data-consent": {"GET", h.consent}, "/api/v1/auth/csrf": {"GET", h.csrf}, "/api/v1/me": {"GET", h.me},
		"/api/v1/auth/register": {"POST", h.register}, "/api/v1/auth/login": {"POST", h.login}, "/api/v1/auth/verify-email": {"POST", h.verify},
		"/api/v1/auth/resend-verification": {"POST", h.resend}, "/api/v1/auth/forgot-password": {"POST", h.forgot},
		"/api/v1/auth/reset-password": {"POST", h.reset}, "/api/v1/auth/logout": {"POST", h.logout},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("X-Request-ID", rand.Text())
		w.Header().Set("X-Request-ID", r.Header.Get("X-Request-ID"))
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		rw := &responseWriter{ResponseWriter: w, status: 200}
		route := "unknown"
		defer func() {
			h.logger.InfoContext(r.Context(), "http request", "route", route, "status", rw.status, "request_id", r.Header.Get("X-Request-ID"))
		}()
		if r.URL.Path == "/health/live" || r.URL.Path == "/health/ready" {
			route = r.URL.Path
			if r.Method != http.MethodGet {
				writeError(rw, r, 405, "INVALID_REQUEST", nil)
				return
			}
			if r.URL.Path == "/health/ready" && !h.ready(r.Context()) {
				writeError(rw, r, 503, "DEPENDENCY_UNAVAILABLE", nil)
				return
			}
			writeJSON(rw, 200, map[string]string{"status": "ok"})
			return
		}
		if h.scenarioRoute(rw, r) {
			route = "/api/v1/companies/{company_id}/scenarios"
			return
		}
		if h.financialRoute(rw, r) {
			route = "/api/v1/companies/{company_id}"
			return
		}
		endpoint, ok := routes[r.URL.Path]
		if !ok {
			writeError(rw, r, 404, "NOT_FOUND", nil)
			return
		}
		route = r.URL.Path
		if r.Method != endpoint.method {
			rw.Header().Set("Allow", endpoint.method)
			writeError(rw, r, 405, "INVALID_REQUEST", nil)
			return
		}
		if r.Method == http.MethodPost && (!h.allowedOrigin(r) || len(r.Header.Values("X-CSRF-Token")) != 1 || len(r.Header.Get("X-CSRF-Token")) < 16 || len(r.Header.Get("X-CSRF-Token")) > 512) {
			writeError(rw, r, 403, "CSRF_INVALID", nil)
			return
		}
		endpoint.fn(rw, r)
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *handler) allowedOrigin(r *http.Request) bool {
	if values, exists := r.Header["Origin"]; exists {
		return len(values) == 1 && values[0] == h.origin
	}
	values := r.Header.Values("Referer")
	if len(values) != 1 {
		return false
	}
	u, err := url.Parse(values[0])
	return err == nil && u.User == nil && u.Scheme+"://"+u.Host == h.origin
}
func browser(r *http.Request) *authv1.BrowserContext {
	// Duplicate cookie names are ambiguous; never silently pick an authenticated one.
	cookies := r.CookiesNamed("__Host-saldo_session")
	if len(cookies) != 1 {
		return &authv1.BrowserContext{}
	}
	return &authv1.BrowserContext{CookieToken: cookies[0].Value}
}
func clientContext(r *http.Request) *authv1.ClientContext {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || net.ParseIP(ip) == nil {
		ip = ""
	}
	return &authv1.ClientContext{ClientIp: ip, RequestId: r.Header.Get("X-Request-ID")}
}
func writeContext(r *http.Request) *authv1.WriteContext {
	return &authv1.WriteContext{Browser: browser(r), Client: clientContext(r), CsrfToken: r.Header.Get("X-CSRF-Token")}
}
func setCookie(w http.ResponseWriter, token string, expiry time.Time) {
	http.SetCookie(w, &http.Cookie{Name: "__Host-saldo_session", Value: token, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: expiry.UTC()})
}
func clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: "__Host-saldo_session", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0).UTC()})
}
