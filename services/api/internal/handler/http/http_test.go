package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	authv1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/auth/v1"
)

func TestErrorMapping(t *testing.T) {
	for value, name := range authv1.AuthErrorReason_name {
		if value == 0 {
			continue
		}
		reason := strings.TrimPrefix(name, "AUTH_ERROR_REASON_")
		grpcCode, want := reasonStatus(reason)
		t.Run(reason, func(t *testing.T) {
			st, err := status.New(grpcCode, "SECRET").WithDetails(&authv1.AuthErrorDetail{Reason: authv1.AuthErrorReason(value), RetryAfterSeconds: 7, FieldViolations: []*authv1.FieldViolation{{Field: "email", Description: "SECRET"}, {Field: "password=SECRET", Description: "SECRET"}}})
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Header.Set("X-Request-ID", "safe-id")
			w := httptest.NewRecorder()
			rpcError(w, r, st.Err())
			if w.Code != want || strings.Contains(w.Body.String(), "SECRET") || !strings.Contains(w.Body.String(), reason) || !strings.Contains(w.Body.String(), "safe-id") {
				t.Fatal(w.Code, w.Body.String())
			}
			if want == 429 && w.Header().Get("Retry-After") != "7" {
				t.Fatal("missing retry hint")
			}
		})
	}
	for _, code := range []codes.Code{codes.Unavailable, codes.DeadlineExceeded, codes.Canceled, codes.PermissionDenied, codes.Unimplemented, codes.Internal, codes.Unknown} {
		w := httptest.NewRecorder()
		rpcError(w, httptest.NewRequest(http.MethodGet, "/", nil), status.Error(code, "SECRET"))
		want := 503
		if code == codes.Internal || code == codes.Unknown {
			want = 500
		}
		if w.Code != want || strings.Contains(w.Body.String(), "SECRET") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	for _, d := range []*authv1.AuthErrorDetail{{Reason: 999}, {Reason: authv1.AuthErrorReason_AUTH_ERROR_REASON_INVALID_CREDENTIALS}} {
		st, _ := status.New(codes.Internal, "SECRET").WithDetails(d)
		w := httptest.NewRecorder()
		rpcError(w, httptest.NewRequest(http.MethodGet, "/", nil), st.Err())
		if w.Code != 500 {
			t.Fatal(w.Code)
		}
	}
}

func TestValidationAndAmbiguity(t *testing.T) {
	for _, s := range []string{"", "Name <a@b.test>", "é@example.test", strings.Repeat("a", 255) + "@test", "bad"} {
		if emailValid(s) {
			t.Fatal(s)
		}
	}
	if !emailValid(" Owner@example.test ") || passwordValid("short") || passwordValid(strings.Repeat("a", 129)) || passwordValid(string([]byte{0xff})) || !passwordValid("😀😀😀😀😀😀😀😀😀😀😀😀") || nameValid("") {
		t.Fatal("validation boundary")
	}
	for _, body := range []string{`{"email":null}`, `{"EMAIL":"a@example.test"}`, `{"email":"a@example.test","Email":"b@example.test"}`, `{"x":[]}`, `{"email": [1,2]}`, `{"email":"a"`, `{"a":{"b":1,"b":2}}`, `{"email":"a","email":"b"}`, `{} []`} {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		var dst emailRequest
		err := decode(httptest.NewRecorder(), r, &dst)
		if body == `{"email":null}` {
			if err != nil || emailValid(dst.Email) {
				t.Fatal("null email")
			}
			continue
		}
		if err == nil {
			t.Fatal("invalid body accepted", body)
		}
	}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "forged"
	if clientContext(r).GetClientIp() != "" {
		t.Fatal("trusted bad IP")
	}
	r.Header.Add("Cookie", "__Host-saldo_session=one; __Host-saldo_session=two")
	if browser(r).GetCookieToken() != "" {
		t.Fatal("ambiguous cookie")
	}
	h := &handler{origin: "https://example.test"}
	r.Header["Origin"] = []string{"https://example.test", "https://evil.test"}
	if h.allowedOrigin(r) {
		t.Fatal("multiple origins")
	}
	delete(r.Header, "Origin")
	r.Header.Set("Referer", "https://user@example.test/login")
	if h.allowedOrigin(r) {
		t.Fatal("referer credentials")
	}
	r.Header.Set("Referer", "https://example.test.evil/login")
	if h.allowedOrigin(r) {
		t.Fatal("referer suffix")
	}
	for _, v := range []*timestamppb.Timestamp{nil, {Seconds: 253402300800}, timestamppb.New(time.Now().Add(-time.Second))} {
		if validTime(v) {
			t.Fatal("bad timestamp")
		}
	}
	if validSecurity(nil) || validUser(nil) || validSecurity(&authv1.SecurityContext{}) || validUser(&authv1.UserProfile{}) {
		t.Fatal("malformed upstream accepted")
	}
}

func TestPublicEnvelopeAndLogs(t *testing.T) {
	var logs strings.Builder
	h := New(nil, "https://example.test", slog.New(slog.NewJSONHandler(&logs, nil)), func(context.Context) bool { return false })
	for _, tc := range []struct {
		method, path string
		want         int
	}{{"GET", "/health/live", 200}, {"GET", "/health/ready", 503}, {"POST", "/health/live", 405}, {"GET", "/SECRET-TOKEN", 404}, {"DELETE", "/api/v1/me", 405}} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader("SECRET-PASSWORD"))
		r.Header.Set("Cookie", "SECRET-COOKIE")
		r.Header.Set("X-Request-ID", "SECRET-ID")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want || strings.Contains(w.Body.String(), "SECRET") || w.Header().Get("X-Request-ID") == "SECRET-ID" {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if strings.Contains(logs.String(), "SECRET") {
		t.Fatal("secrets in structured logs")
	}
}

func TestMalformedUpstreamAndInvalidWrites(t *testing.T) {
	// A real network suite covers success/RPC failure. These unit tests target
	// invalid upstream envelopes before setting cookies or writing public DTOs.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	respondChallenge(w, r, nil)
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
	for _, kind := range []string{"register", "login", "verify", "resend", "forgot", "reset"} {
		h := &handler{}
		fns := map[string]http.HandlerFunc{"register": h.register, "login": h.login, "verify": h.verify, "resend": h.resend, "forgot": h.forgot, "reset": h.reset}
		for _, body := range []string{`{`, `{}`} {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			fns[kind](w, r)
			want := 422
			if body == `{` {
				want = 400
			}
			if w.Code != want {
				t.Fatalf("%s %s %d", kind, body, w.Code)
			}
		}
	}
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	(&handler{}).logout(w, r)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}
