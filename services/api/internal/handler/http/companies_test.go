package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	authv1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/auth/v1"
	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

const creationID = "550e8400-e29b-41d4-a716-446655440000"
const creationActor = "550e8400-e29b-41d4-a716-446655440001"
const companyBody = `{"name":"  Кофейня  ","inn":null,"currency":"RUB","timezone":"Europe/Moscow"}`

type companyFixture struct {
	*financialFixture
	prepared, completed                 *authv1.CompanyCreationCommand
	beginErr, bootstrapErr, completeErr error
	events                              []string
	begin                               *authv1.BeginCompanyCreationRequest
	bootstrap                           *corev1.CreateCompanyBootstrapRequest
	complete                            *authv1.CompleteCompanyCreationRequest
}

func (f *companyFixture) BeginCompanyCreation(_ context.Context, r *authv1.BeginCompanyCreationRequest, _ ...grpc.CallOption) (*authv1.BeginCompanyCreationResponse, error) {
	f.events = append(f.events, "begin")
	f.begin = proto.Clone(r).(*authv1.BeginCompanyCreationRequest)
	return &authv1.BeginCompanyCreationResponse{Command: f.prepared}, f.beginErr
}
func (f *companyFixture) CreateCompanyBootstrap(_ context.Context, r *corev1.CreateCompanyBootstrapRequest, _ ...grpc.CallOption) (*corev1.CreateCompanyBootstrapResponse, error) {
	f.events = append(f.events, "bootstrap")
	f.bootstrap = proto.Clone(r).(*corev1.CreateCompanyBootstrapRequest)
	return nil, f.bootstrapErr
}
func (f *companyFixture) CompleteCompanyCreation(_ context.Context, r *authv1.CompleteCompanyCreationRequest, _ ...grpc.CallOption) (*authv1.CompleteCompanyCreationResponse, error) {
	f.events = append(f.events, "complete")
	f.complete = proto.Clone(r).(*authv1.CompleteCompanyCreationRequest)
	return &authv1.CompleteCompanyCreationResponse{Command: f.completed}, f.completeErr
}
func (f *companyFixture) ReadCompany(ctx context.Context, r *corev1.ReadCompanyRequest, opts ...grpc.CallOption) (*corev1.ReadCompanyResponse, error) {
	f.events = append(f.events, "read")
	return f.financialFixture.ReadCompany(ctx, r, opts...)
}

func newCompanyFixture() *companyFixture {
	f := &companyFixture{financialFixture: newFinancialFixture()}
	f.company.Company.Name = "Кофейня"
	f.prepared = &authv1.CompanyCreationCommand{Id: creationID, CompanyId: companyID, InitiatorId: creationActor, Company: &authv1.CompanyCreateSpec{Name: "Кофейня", Currency: "RUB", Timezone: "Europe/Moscow"}, RequestSha256: strings.Repeat("a", 64), State: authv1.CompanyCreationState_COMPANY_CREATION_STATE_PREPARED, CreatedAt: timestamppb.New(time.Now().UTC())}
	f.completed = proto.Clone(f.prepared).(*authv1.CompanyCreationCommand)
	f.completed.State = authv1.CompanyCreationState_COMPANY_CREATION_STATE_COMPLETED
	return f
}

func (f *companyFixture) companyHandler() http.Handler {
	return NewWithCore(f, "https://localhost", slog.New(slog.DiscardHandler), func(context.Context) bool { return true }, CoreClients{Companies: f})
}

func TestCompanyCreationSequenceUsesSavedSpecAndWaitsForOwner(t *testing.T) {
	f := newCompanyFixture()
	w := financialRequest(f.companyHandler(), "POST", "/api/v1/companies", companyBody)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if !reflect.DeepEqual(f.events, []string{"begin", "bootstrap", "complete", "read"}) {
		t.Fatal(f.events)
	}
	if f.begin.IdempotencyKey != "financial-key-123" || f.begin.Company.Name != "  Кофейня  " || f.begin.Context.Browser.CookieToken != "opaque-session" {
		t.Fatal(f.begin)
	}
	if f.bootstrap.CommandId != creationID || f.bootstrap.CompanyId != companyID || !proto.Equal(f.bootstrap.CompanySpec, f.prepared.Company) || f.bootstrap.RequestSha256 != f.prepared.RequestSha256 {
		t.Fatal("browser spec substituted for saved Auth spec", f.bootstrap)
	}
	if f.complete.CommandId != creationID || f.complete.Context.Browser.CookieToken != "opaque-session" || f.complete.Context.CsrfToken != strings.Repeat("c", 24) {
		t.Fatal(f.complete)
	}
	if strings.Contains(w.Body.String(), "opaque-session") || w.Header().Get("X-Request-ID") == "" {
		t.Fatal(w.Body.String())
	}
	var body object
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["id"] != companyID || body["default_account_id"] != accountID || body["name"] != "Кофейня" {
		t.Fatal(body)
	}
	// The same completed Auth mapping follows the canonical chain again.
	f.prepared.State = authv1.CompanyCreationState_COMPANY_CREATION_STATE_COMPLETED
	if w := financialRequest(f.companyHandler(), "POST", "/api/v1/companies", companyBody); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func companyAuthFailure(t *testing.T, code codes.Code, reason authv1.AuthErrorReason) error {
	t.Helper()
	s, err := status.New(code, "PRIVATE_DIAGNOSTIC").WithDetails(&authv1.AuthErrorDetail{Reason: reason})
	if err != nil {
		t.Fatal(err)
	}
	return s.Err()
}

func TestCompanyCreationDoesNotReportPrematureSuccess(t *testing.T) {
	for _, tt := range []struct {
		name         string
		edit         func(*companyFixture)
		want, events int
	}{
		{"begin conflict", func(f *companyFixture) {
			f.beginErr = companyAuthFailure(t, codes.Aborted, authv1.AuthErrorReason_AUTH_ERROR_REASON_IDEMPOTENCY_CONFLICT)
		}, 409, 1},
		{"session revoked", func(f *companyFixture) {
			f.beginErr = companyAuthFailure(t, codes.Unauthenticated, authv1.AuthErrorReason_AUTH_ERROR_REASON_AUTH_REQUIRED)
		}, 401, 1},
		{"unverified", func(f *companyFixture) {
			f.beginErr = companyAuthFailure(t, codes.PermissionDenied, authv1.AuthErrorReason_AUTH_ERROR_REASON_EMAIL_UNVERIFIED)
		}, 403, 1},
		{"begin nil", func(f *companyFixture) { f.prepared = nil }, 503, 1},
		{"quarantined", func(f *companyFixture) {
			f.prepared.State = authv1.CompanyCreationState_COMPANY_CREATION_STATE_QUARANTINED
		}, 403, 1},
		{"bootstrap lost reply", func(f *companyFixture) { f.bootstrapErr = status.Error(codes.DeadlineExceeded, "PRIVATE_DIAGNOSTIC") }, 503, 2},
		{"wrong peer", func(f *companyFixture) { f.bootstrapErr = status.Error(codes.PermissionDenied, "PRIVATE_DIAGNOSTIC") }, 503, 2},
		{"complete missing receipt", func(f *companyFixture) {
			f.completeErr = companyAuthFailure(t, codes.Unavailable, authv1.AuthErrorReason_AUTH_ERROR_REASON_COMPANY_CREATION_IN_PROGRESS)
		}, 503, 3},
		{"complete nil", func(f *companyFixture) { f.completed = nil }, 503, 3},
		{"complete prepared", func(f *companyFixture) {
			f.completed.State = authv1.CompanyCreationState_COMPANY_CREATION_STATE_PREPARED
		}, 503, 3},
		{"complete quarantined", func(f *companyFixture) {
			f.completed.State = authv1.CompanyCreationState_COMPANY_CREATION_STATE_QUARANTINED
		}, 403, 3},
		{"complete wrong id", func(f *companyFixture) { f.completed.Id = creationActor }, 503, 3},
		{"complete wrong company", func(f *companyFixture) { f.completed.CompanyId = accountID }, 503, 3},
		{"complete wrong actor", func(f *companyFixture) { f.completed.InitiatorId = creationID }, 503, 3},
		{"complete wrong hash", func(f *companyFixture) { f.completed.RequestSha256 = strings.Repeat("b", 64) }, 503, 3},
		{"complete wrong spec", func(f *companyFixture) { f.completed.Company.Name = "Other" }, 503, 3},
		{"read revoked", func(f *companyFixture) {
			f.err = companyAuthFailure(t, codes.PermissionDenied, authv1.AuthErrorReason_AUTH_ERROR_REASON_FORBIDDEN)
		}, 403, 4},
		{"read malformed", func(f *companyFixture) { f.company = nil }, 503, 4},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newCompanyFixture()
			tt.edit(f)
			w := financialRequest(f.companyHandler(), "POST", "/api/v1/companies", companyBody)
			if w.Code != tt.want || len(f.events) != tt.events || strings.Contains(w.Body.String(), "PRIVATE_DIAGNOSTIC") {
				t.Fatal(w.Code, w.Body.String(), f.events)
			}
			var body errorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error.RequestID != w.Header().Get("X-Request-ID") {
				t.Fatal("request ID changed")
			}
		})
	}
}

func TestCompanyCreationRejectsMalformedInputAndDependencyCommands(t *testing.T) {
	for _, body := range []string{`{}`, `{"name":"","currency":"RUB","timezone":"Europe/Moscow"}`, `{"name":"  ","currency":"RUB","timezone":"Europe/Moscow"}`, `{"name":"X","currency":"USD","timezone":"Europe/Moscow"}`, `{"name":"X","currency":"RUB","timezone":"UTC"}`, `{"name":"X","inn":"","currency":"RUB","timezone":"Europe/Moscow"}`, `{"name":"X","currency":"RUB","timezone":"Europe/Moscow","owner_id":"attacker"}`} {
		f := newCompanyFixture()
		w := financialRequest(f.companyHandler(), "POST", "/api/v1/companies", body)
		if w.Code != 422 || len(f.events) != 0 {
			t.Fatal(w.Code, w.Body.String(), f.events)
		}
	}
	for name, edit := range map[string]func(*authv1.CompanyCreationCommand){
		"command id":      func(c *authv1.CompanyCreationCommand) { c.Id = "bad" },
		"company version": func(c *authv1.CompanyCreationCommand) { c.CompanyId = creationID },
		"actor id":        func(c *authv1.CompanyCreationCommand) { c.InitiatorId = "bad" },
		"hash":            func(c *authv1.CompanyCreationCommand) { c.RequestSha256 = "bad" },
		"timestamp":       func(c *authv1.CompanyCreationCommand) { c.CreatedAt = nil },
		"spec":            func(c *authv1.CompanyCreationCommand) { c.Company = nil },
		"unnormalized":    func(c *authv1.CompanyCreationCommand) { c.Company.Name = " Other " },
		"NUL":             func(c *authv1.CompanyCreationCommand) { c.Company.Name = "Other\x00" },
		"currency":        func(c *authv1.CompanyCreationCommand) { c.Company.Currency = "USD" },
		"timezone":        func(c *authv1.CompanyCreationCommand) { c.Company.Timezone = "UTC" },
		"INN":             func(c *authv1.CompanyCreationCommand) { c.Company.Inn = new(string) },
		"state": func(c *authv1.CompanyCreationCommand) {
			c.State = authv1.CompanyCreationState_COMPANY_CREATION_STATE_UNSPECIFIED
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newCompanyFixture()
			edit(f.prepared)
			w := financialRequest(f.companyHandler(), "POST", "/api/v1/companies", companyBody)
			if w.Code != 503 || len(f.events) != 1 {
				t.Fatal(w.Code, w.Body.String(), f.events)
			}
		})
	}
	for _, mode := range []string{"missing key", "duplicate key", "bad key", "CSRF", "Origin"} {
		t.Run(mode, func(t *testing.T) {
			f := newCompanyFixture()
			r := httptest.NewRequest(http.MethodPost, "/api/v1/companies", strings.NewReader(companyBody))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Origin", "https://localhost")
			r.Header.Set("X-CSRF-Token", strings.Repeat("c", 24))
			r.Header.Set("Idempotency-Key", "create-company-key")
			want := 422
			switch mode {
			case "missing key":
				r.Header.Del("Idempotency-Key")
			case "duplicate key":
				r.Header.Add("Idempotency-Key", "another-company-key")
			case "bad key":
				r.Header.Set("Idempotency-Key", "bad key")
			case "CSRF":
				r.Header.Del("X-CSRF-Token")
				want = 403
			case "Origin":
				r.Header.Set("Origin", "https://attacker.test")
				want = 403
			}
			w := httptest.NewRecorder()
			f.companyHandler().ServeHTTP(w, r)
			if w.Code != want || len(f.events) != 0 {
				t.Fatal(w.Code, f.events)
			}
		})
	}
}

func TestCompanyCreationContractResponses(t *testing.T) {
	var reports []object
	for _, mode := range []string{"created", "conflict", "in progress", "quarantined", "invalid"} {
		f := newCompanyFixture()
		body := companyBody
		want := 201
		switch mode {
		case "conflict":
			f.beginErr = companyAuthFailure(t, codes.Aborted, authv1.AuthErrorReason_AUTH_ERROR_REASON_IDEMPOTENCY_CONFLICT)
			want = 409
		case "in progress":
			f.completeErr = companyAuthFailure(t, codes.Unavailable, authv1.AuthErrorReason_AUTH_ERROR_REASON_COMPANY_CREATION_IN_PROGRESS)
			want = 503
		case "quarantined":
			f.prepared.State = authv1.CompanyCreationState_COMPANY_CREATION_STATE_QUARANTINED
			want = 403
		case "invalid":
			body = `{}`
			want = 422
		}
		w := financialRequest(f.companyHandler(), "POST", "/api/v1/companies", body)
		if w.Code != want {
			t.Fatal(w.Code, w.Body.String())
		}
		var value any
		if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		reports = append(reports, object{"method": "POST", "path": "/api/v1/companies", "status": want, "body": value})
	}
	if path := os.Getenv("API_COMPANY_REPORT"); path != "" {
		data, err := json.Marshal(reports)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
