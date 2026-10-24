package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
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

const companyID = "019a0c65-9800-7000-8000-000000000001"
const accountID = "019a0c65-9800-7000-8000-000000000002"
const forecastID = "019a0c65-9800-7000-8000-000000000003"
const financialRoot = "/api/v1/companies/" + companyID

type financialFixture struct {
	authv1.AuthServiceClient
	company            *corev1.ReadCompanyResponse
	account            *corev1.Account
	forecast           *corev1.Forecast
	memberships        *authv1.ListMembershipsResponse
	listed             *corev1.ListForecastsResponse
	accepted           *corev1.ForecastAccepted
	err, membershipErr error
	calls              int
	create             *corev1.CreateForecastRequest
	browserToken       string
}

func (f *financialFixture) CreateCompanyBootstrap(context.Context, *corev1.CreateCompanyBootstrapRequest, ...grpc.CallOption) (*corev1.CreateCompanyBootstrapResponse, error) {
	return nil, status.Error(codes.Unimplemented, "fixture company creation unavailable")
}

func (f *financialFixture) ReadCompany(_ context.Context, r *corev1.ReadCompanyRequest, _ ...grpc.CallOption) (*corev1.ReadCompanyResponse, error) {
	f.calls++
	f.browserToken = r.GetContext().GetCookieToken()
	return f.company, f.err
}
func (f *financialFixture) GetDefaultAccount(_ context.Context, r *corev1.GetDefaultAccountRequest, _ ...grpc.CallOption) (*corev1.GetDefaultAccountResponse, error) {
	f.calls++
	f.browserToken = r.GetContext().GetCookieToken()
	return &corev1.GetDefaultAccountResponse{Account: f.account}, f.err
}
func (f *financialFixture) GetForecast(_ context.Context, r *corev1.GetForecastRequest, _ ...grpc.CallOption) (*corev1.GetForecastResponse, error) {
	f.calls++
	if r.Scope.AccountId != accountID {
		return nil, errors.New("bad account")
	}
	return &corev1.GetForecastResponse{Forecast: f.forecast}, f.err
}
func (f *financialFixture) ListForecasts(_ context.Context, r *corev1.ListForecastsRequest, _ ...grpc.CallOption) (*corev1.ListForecastsResponse, error) {
	f.calls++
	if r.Scope.AccountId != accountID {
		return nil, errors.New("bad account")
	}
	return f.listed, f.err
}
func (f *financialFixture) CreateForecast(_ context.Context, r *corev1.CreateForecastRequest, _ ...grpc.CallOption) (*corev1.CreateForecastResponse, error) {
	f.calls++
	f.create = proto.Clone(r).(*corev1.CreateForecastRequest)
	return &corev1.CreateForecastResponse{Accepted: f.accepted}, f.err
}
func (f *financialFixture) ListMemberships(_ context.Context, r *authv1.ListMembershipsRequest, _ ...grpc.CallOption) (*authv1.ListMembershipsResponse, error) {
	f.calls++
	f.browserToken = r.GetContext().GetCookieToken()
	return f.memberships, f.membershipErr
}
func rub(n int64) *corev1.Money { return &corev1.Money{AmountMinor: n, Currency: "RUB"} }
func unavailableProbability() *corev1.Probability {
	return &corev1.Probability{Estimate: &corev1.Probability_UnavailableReason{UnavailableReason: "not_estimated"}}
}
func newFinancialFixture() *financialFixture {
	now := timestamppb.New(time.Date(2026, time.October, 23, 12, 0, 0, 0, time.UTC))
	f := &financialFixture{company: &corev1.ReadCompanyResponse{Company: &corev1.Company{Id: companyID, Name: "Компания", Currency: "RUB", Timezone: "Europe/Moscow", DefaultAccountId: accountID, Version: 1, CreatedAt: now}, Role: authv1.CompanyRole_COMPANY_ROLE_OWNER}, account: &corev1.Account{Id: accountID, CompanyId: companyID, Name: "Основной счёт", Currency: "RUB", DataVersion: 1}, memberships: &authv1.ListMembershipsResponse{Items: []*authv1.Membership{{CompanyId: companyID, Role: authv1.CompanyRole_COMPANY_ROLE_OWNER}}, Pagination: &authv1.PageResponse{}}, listed: &corev1.ListForecastsResponse{Pagination: &corev1.PageResponse{}}, accepted: &corev1.ForecastAccepted{Id: forecastID, Status: corev1.ForecastStatus_FORECAST_STATUS_QUEUED, PollAfterMs: 1000}}
	f.forecast = &corev1.Forecast{Id: forecastID, CompanyId: companyID, CreatedAt: now, CurrentDataVersion: 1, Readiness: &corev1.ForecastReadiness{Status: "ready", HistoryDays: 90, ObservedDays: 90}, Input: &corev1.ForecastInput{SnapshotId: forecastID, DataVersion: 1, CutoffDate: "2026-10-23", DataAsOf: "2026-10-23", StartDate: "2026-10-24", EndDate: "2026-11-22", HorizonDays: 30, Currency: "RUB", AccountId: accountID, OpeningBalance: rub(0), BalanceAsOf: "2026-10-23", RiskDefinition: "below_zero_eod", ModelContext: &corev1.ForecastModelContext{Id: forecastID, ModelName: "mean_90", ModelVersion: "mean_90_components_v1", PreprocessingVersion: "calendar_replacement_v1", ConfigurationId: "cfg_fixture"}}, State: &corev1.Forecast_Pending{Pending: &corev1.ForecastPending{Status: corev1.ForecastStatus_FORECAST_STATUS_QUEUED}}}
	return f
}
func (f *financialFixture) handler() http.Handler {
	return NewWithCore(f, "https://localhost", slog.New(slog.DiscardHandler), func(context.Context) bool { return true }, CoreClients{Companies: f, Ledger: f, Forecasts: f})
}
func financialRequest(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://localhost")
	r.Header.Set("X-CSRF-Token", strings.Repeat("c", 24))
	r.Header.Set("Idempotency-Key", "financial-key-123")
	r.AddCookie(&http.Cookie{Name: "__Host-saldo_session", Value: "opaque-session"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func succeeded(f *financialFixture) {
	result := &corev1.ForecastResult{Metrics: &corev1.ForecastMetrics{MaxShortfall: rub(0), MinimumBalance: rub(0), ClosingBalance: rub(0), GapProbability: unavailableProbability()}, ScenarioCapabilities: &corev1.ScenarioCapabilities{}}
	start, _ := time.Parse(time.DateOnly, f.forecast.Input.StartDate)
	for i := 0; i < 30; i++ {
		result.DailyPoints = append(result.DailyPoints, &corev1.ForecastPoint{Date: start.AddDate(0, 0, i).Format(time.DateOnly), Inflow: rub(0), Outflow: rub(0), Balance: rub(0), NegativeBalanceProbability: unavailableProbability()})
	}
	f.forecast.State = &corev1.Forecast_Succeeded{Succeeded: &corev1.ForecastSucceeded{CompletedAt: timestamppb.New(f.forecast.CreatedAt.AsTime().Add(time.Second)), Result: result}}
}
func TestFinancialContractResponses(t *testing.T) {
	var reports []object
	capture := func(f *financialFixture, method, path, template, body string, want int) {
		t.Helper()
		w := financialRequest(f.handler(), method, path, body)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		var value any
		if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		reports = append(reports, object{"method": method, "path": template, "status": want, "body": value})
		if strings.Contains(w.Body.String(), "opaque-session") || strings.Contains(w.Body.String(), "PRIVATE") {
			t.Fatal("private data leaked")
		}
	}
	f := newFinancialFixture()
	capture(f, "GET", "/api/v1/companies?limit=1", "/api/v1/companies", "", 200)
	capture(f, "GET", financialRoot, "/api/v1/companies/{company_id}", "", 200)
	capture(f, "GET", financialRoot+"/account", "/api/v1/companies/{company_id}/account", "", 200)
	f.account.Balance = &corev1.DatedBalance{Amount: rub(0), AsOf: "2026-10-23", Source: corev1.BalanceSource_BALANCE_SOURCE_MANUAL}
	capture(f, "GET", financialRoot+"/account", "/api/v1/companies/{company_id}/account", "", 200)
	f.account.TransactionSource = &corev1.FactualSourceBinding{Mode: corev1.FactualSourceMode_FACTUAL_SOURCE_MODE_IMPORTED, SourceSystem: "dataset_v1", SourceCompanyId: proto.String("C001")}
	capture(f, "GET", financialRoot+"/account", "/api/v1/companies/{company_id}/account", "", 200)
	f.listed.Items = []*corev1.ForecastSummary{{Id: forecastID, Status: corev1.ForecastStatus_FORECAST_STATUS_SUCCEEDED, CutoffDate: "2026-10-23", StartDate: "2026-10-24", EndDate: "2026-11-22", CreatedAt: f.forecast.CreatedAt}}
	capture(f, "GET", financialRoot+"/forecasts?include_scenarios=false&limit=100", "/api/v1/companies/{company_id}/forecasts", "", 200)
	capture(f, "POST", financialRoot+"/forecasts", "/api/v1/companies/{company_id}/forecasts", `{"cutoff_date":"2026-10-23","horizon_days":30}`, 202)
	if f.create.Scope.AccountId != accountID || f.create.IdempotencyKey != "financial-key-123" || f.create.Context.Browser.CookieToken != "opaque-session" || f.create.Context.CsrfToken != strings.Repeat("c", 24) {
		t.Fatal("lost trusted context", f.create)
	}
	capture(f, "GET", financialRoot+"/forecasts/"+forecastID, "/api/v1/companies/{company_id}/forecasts/{forecast_id}", "", 200)
	f.forecast.GetPending().Status = corev1.ForecastStatus_FORECAST_STATUS_RUNNING
	capture(f, "GET", financialRoot+"/forecasts/"+forecastID, "/api/v1/companies/{company_id}/forecasts/{forecast_id}", "", 200)
	succeeded(f)
	capture(f, "GET", financialRoot+"/forecasts/"+forecastID, "/api/v1/companies/{company_id}/forecasts/{forecast_id}", "", 200)
	f.forecast.GetSucceeded().Result.Metrics.GapProbability = &corev1.Probability{Estimate: &corev1.Probability_Value{Value: 0}}
	capture(f, "GET", financialRoot+"/forecasts/"+forecastID, "/api/v1/companies/{company_id}/forecasts/{forecast_id}", "", 200)
	f.forecast.State = &corev1.Forecast_Failed{Failed: &corev1.ForecastFailed{CompletedAt: timestamppb.New(f.forecast.CreatedAt.AsTime().Add(time.Second)), Error: &corev1.OperationError{Code: "INSUFFICIENT_HISTORY", Message: "PRIVATE", RequestId: "safe-request", FieldErrors: []*corev1.OperationFieldError{{Field: "cutoff_date", Code: "BAD", Message: "PRIVATE"}}}}}
	failedResponse := financialRequest(f.handler(), "GET", financialRoot+"/forecasts/"+forecastID, "")
	if failedResponse.Code != 200 || !strings.Contains(failedResponse.Body.String(), "Недостаточно подтверждённой истории") || strings.Contains(failedResponse.Body.String(), "PRIVATE") {
		t.Fatal("failed forecast lost safe public explanation", failedResponse.Code, failedResponse.Body.String())
	}
	capture(f, "GET", financialRoot+"/forecasts/"+forecastID, "/api/v1/companies/{company_id}/forecasts/{forecast_id}", "", 200)
	for _, reason := range []string{"FORBIDDEN", "NOT_FOUND", "DEPENDENCY_UNAVAILABLE", "BALANCE_AS_OF_UNAVAILABLE"} {
		rpc, httpCode := coreReasonStatus(reason)
		enum := corev1.CoreErrorReason(corev1.CoreErrorReason_value["CORE_ERROR_REASON_"+reason])
		st, err := status.New(rpc, "PRIVATE").WithDetails(&corev1.CoreErrorDetail{Reason: enum})
		if err != nil {
			t.Fatal(err)
		}
		f.err = st.Err()
		if reason == "BALANCE_AS_OF_UNAVAILABLE" {
			capture(f, "POST", financialRoot+"/forecasts", "/api/v1/companies/{company_id}/forecasts", `{"cutoff_date":"2026-10-23","horizon_days":30}`, httpCode)
		} else {
			capture(f, "GET", financialRoot+"/account", "/api/v1/companies/{company_id}/account", "", httpCode)
		}
	}
	if path := os.Getenv("API_FINANCIAL_REPORT"); path != "" {
		data, err := json.MarshalIndent(reports, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
func TestFinancialInputAndTransportGuards(t *testing.T) {
	tests := []struct {
		name, method, path, body string
		want                     int
	}{
		{"outside API prefix", "GET", "/companies", "", 404},
		{"outside API forecast prefix", "GET", strings.TrimPrefix(financialRoot, "/api/v1") + "/forecasts", "", 404},
		{"bad company", "GET", "/api/v1/companies/not-uuid", "", 422},
		{"nil company", "GET", "/api/v1/companies/00000000-0000-0000-0000-000000000000", "", 422},
		{"bad forecast", "GET", financialRoot + "/forecasts/not-uuid", "", 422},
		{"wrong method", "DELETE", financialRoot, "", 405},
		{"forecast method", "PATCH", financialRoot + "/forecasts", "", 405},
		{"unknown", "GET", financialRoot + "/unknown", "", 404},
		{"bad query", "GET", "/api/v1/companies?limit=0", "", 400},
		{"repeated query", "GET", "/api/v1/companies?limit=1&limit=2", "", 400},
		{"unknown query", "GET", "/api/v1/companies?role=owner", "", 400},
		{"bad encoding", "GET", "/api/v1/companies?cursor=%ZZ", "", 400},
		{"bad cursor", "GET", financialRoot + "/forecasts?cursor=bad+cursor", "", 400},
		{"bad bool", "GET", financialRoot + "/forecasts?include_scenarios=yes", "", 400},
		{"bad date", "POST", financialRoot + "/forecasts", `{"cutoff_date":"2026-02-30","horizon_days":30}`, 422},
		{"short horizon", "POST", financialRoot + "/forecasts", `{"cutoff_date":"2026-10-23","horizon_days":29}`, 422},
		{"extra field", "POST", financialRoot + "/forecasts", `{"cutoff_date":"2026-10-23","horizon_days":30,"account_id":"foreign"}`, 422},
		{"duplicate field", "POST", financialRoot + "/forecasts", `{"cutoff_date":"2026-10-23","cutoff_date":"2026-10-22","horizon_days":30}`, 422},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFinancialFixture()
			w := financialRequest(f.handler(), tt.method, tt.path, tt.body)
			if w.Code != tt.want || f.calls != 0 {
				t.Fatal(w.Code, w.Body.String(), f.calls)
			}
		})
	}
	for _, edit := range []func(*http.Request){func(r *http.Request) { r.Header.Set("Origin", "https://evil.test") }, func(r *http.Request) { r.Header.Del("X-CSRF-Token") }, func(r *http.Request) { r.Header.Add("X-CSRF-Token", strings.Repeat("c", 24)) }} {
		f := newFinancialFixture()
		r := httptest.NewRequest(http.MethodPost, financialRoot+"/forecasts", nil)
		r.Header.Set("Origin", "https://localhost")
		r.Header.Set("X-CSRF-Token", strings.Repeat("c", 24))
		edit(r)
		w := httptest.NewRecorder()
		f.handler().ServeHTTP(w, r)
		if w.Code != 403 || f.calls != 0 {
			t.Fatal(w.Code, f.calls)
		}
	}
	f := newFinancialFixture()
	h := New(f, "https://localhost", slog.New(slog.DiscardHandler), func(context.Context) bool { return true })
	if w := financialRequest(h, "GET", "/api/v1/companies", ""); w.Code != 503 {
		t.Fatal(w.Code)
	}
	for _, key := range []string{"short", "bad key !!!", strings.Repeat("a", 129)} {
		f := newFinancialFixture()
		r := httptest.NewRequest(http.MethodPost, financialRoot+"/forecasts", strings.NewReader(`{"cutoff_date":"2026-10-23","horizon_days":30}`))
		r.Header.Set("Origin", "https://localhost")
		r.Header.Set("X-CSRF-Token", strings.Repeat("c", 24))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		f.handler().ServeHTTP(w, r)
		if w.Code != 422 || f.calls != 0 {
			t.Fatal(w.Code, f.calls)
		}
	}
}
func TestCoreTypedErrorsAndMalformedUpstream(t *testing.T) {
	for value, name := range corev1.CoreErrorReason_name {
		if value == 0 {
			continue
		}
		reason := strings.TrimPrefix(name, "CORE_ERROR_REASON_")
		c, want := coreReasonStatus(reason)
		t.Run(reason, func(t *testing.T) {
			st, err := status.New(c, "PRIVATE").WithDetails(&corev1.CoreErrorDetail{Reason: corev1.CoreErrorReason(value), RetryAfterSeconds: 7})
			if err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			coreRPCError(w, httptest.NewRequest(http.MethodGet, "/", nil), st.Err())
			if w.Code != want || strings.Contains(w.Body.String(), "PRIVATE") {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
	for _, code := range []codes.Code{codes.DeadlineExceeded, codes.Unavailable, codes.Internal, codes.Canceled, codes.PermissionDenied, codes.Unimplemented} {
		w := httptest.NewRecorder()
		coreRPCError(w, httptest.NewRequest(http.MethodGet, "/", nil), status.Error(code, "PRIVATE"))
		want := 503
		if code == codes.DeadlineExceeded {
			want = 504
		}
		if w.Code != want {
			t.Fatal(code, w.Code)
		}
	}
	for _, mode := range []string{"nil membership", "nil page", "foreign membership", "duplicate membership", "role mismatch", "malformed company", "malformed account", "nil forecast", "wrong forecast", "missing state", "bad list", "list page", "accepted"} {
		t.Run(mode, func(t *testing.T) {
			f := newFinancialFixture()
			path, method, body := "/api/v1/companies", "GET", ""
			switch mode {
			case "nil membership":
				f.memberships = nil
			case "nil page":
				f.memberships.Pagination = nil
			case "foreign membership":
				f.memberships.Items[0].CompanyId = "not-id"
			case "duplicate membership":
				f.memberships.Items = append(f.memberships.Items, f.memberships.Items[0])
			case "role mismatch":
				f.company.Role = authv1.CompanyRole_COMPANY_ROLE_FINANCIER
			case "malformed company":
				f.company.Company.Currency = "USD"
				path = financialRoot
			case "malformed account":
				f.account.CompanyId = forecastID
				path = financialRoot + "/account"
			case "nil forecast":
				f.forecast = nil
				path = financialRoot + "/forecasts/" + forecastID
			case "wrong forecast":
				f.forecast.Id = accountID
				path = financialRoot + "/forecasts/" + forecastID
			case "missing state":
				f.forecast.State = nil
				path = financialRoot + "/forecasts/" + forecastID
			case "bad list":
				f.listed.Items = []*corev1.ForecastSummary{{}}
				path = financialRoot + "/forecasts"
			case "list page":
				f.listed.Pagination = nil
				path = financialRoot + "/forecasts"
			case "accepted":
				f.accepted.Status = corev1.ForecastStatus_FORECAST_STATUS_SUCCEEDED
				path = financialRoot + "/forecasts"
				method = "POST"
				body = `{"cutoff_date":"2026-10-23","horizon_days":30}`
			}
			w := financialRequest(f.handler(), method, path, body)
			if w.Code != 503 {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
	f := newFinancialFixture()
	f.membershipErr = status.Error(codes.Unavailable, "PRIVATE")
	if w := financialRequest(f.handler(), "GET", "/api/v1/companies", ""); w.Code != 503 {
		t.Fatal(w.Code)
	}
	for _, path := range []string{financialRoot, financialRoot + "/forecasts", financialRoot + "/forecasts/" + forecastID} {
		f = newFinancialFixture()
		f.err = status.Error(codes.Unavailable, "PRIVATE")
		if w := financialRequest(f.handler(), "GET", path, ""); w.Code != 503 {
			t.Fatal(w.Code)
		}
	}
}
func TestFinancialProjectionPreservesPresence(t *testing.T) {
	f := newFinancialFixture()
	if !validForecast(f.forecast, f.account, forecastID) {
		t.Fatal("pending rejected")
	}
	succeeded(f)
	result := f.forecast.GetSucceeded().Result
	result.Metrics.FirstGap = &corev1.CashGap{Date: "2026-10-24", Shortfall: rub(1)}
	result.DailyPoints[0].Outflow, result.DailyPoints[0].Balance = rub(1), rub(-1)
	result.DailyPoints[1].Inflow = rub(1)
	result.Metrics.MinimumBalance, result.Metrics.MaxShortfall = rub(-1), rub(1)
	result.DailyPoints[0].BalanceLower, result.DailyPoints[0].BalanceUpper = rub(-1), rub(1)
	result.Components = []*corev1.ForecastComponent{{Id: "component", OccurrenceId: "occurrence", Kind: "known_payment", Direction: corev1.Direction_DIRECTION_OUTFLOW, Date: "2026-10-24", Amount: rub(1), RemainingAmount: rub(0), CategoryId: proto.String(accountID), PaymentId: proto.String(forecastID), RecurrenceGroupId: proto.String("group"), Source: &corev1.RecordSource{Kind: corev1.RecordSourceKind_RECORD_SOURCE_KIND_FILE, SourceSystem: "dataset_v1", SourceCompanyId: proto.String("C001"), ImportId: proto.String(forecastID)}}}
	result.EffectiveOccurrences = []*corev1.EffectiveOccurrence{{EffectiveOccurrenceId: "piece", ComponentId: "component", OccurrenceId: "occurrence", PaymentId: proto.String(forecastID), Direction: corev1.Direction_DIRECTION_OUTFLOW, Date: "2026-10-24", RemainingAmount: rub(0)}}
	result.Recommendations = []*corev1.Recommendation{{Id: "rec", Title: "Совет", SuggestedAction: proto.String("reschedule_payment"), Evidence: []*corev1.Evidence{{Type: "payment", ResourceId: forecastID}}}}
	f.forecast.Readiness.QualityIssues = []*corev1.ImportIssue{{Code: "SHORT_HISTORY", Severity: corev1.IssueSeverity_ISSUE_SEVERITY_WARNING, RowNumber: proto.Uint64(2), Field: proto.String("purpose")}}
	f.forecast.Input.ModelContext.RandomSeed = proto.String("18446744073709551616")
	f.forecast.Input.ModelContext.TrajectoryCount = proto.Uint32(1000)
	if !validForecast(f.forecast, f.account, forecastID) {
		t.Fatal("valid rich result rejected")
	}
	b, err := json.Marshal(forecastDTO(f.forecast))
	if err != nil || !strings.Contains(string(b), `"remaining_amount":{"amount_minor":"0"`) {
		t.Fatal(string(b), err)
	}
	for _, source := range []corev1.RecordSourceKind{corev1.RecordSourceKind_RECORD_SOURCE_KIND_MANUAL, corev1.RecordSourceKind_RECORD_SOURCE_KIND_MOCK_EXTERNAL} {
		result.Components[0].Source.Kind = source
		if !validForecast(f.forecast, f.account, forecastID) {
			t.Fatal(source)
		}
		_ = forecastDTO(f.forecast)
	}
	f.account.TransactionSource = &corev1.FactualSourceBinding{Mode: corev1.FactualSourceMode_FACTUAL_SOURCE_MODE_MANUAL, SourceSystem: "manual_v1"}
	if !validAccount(f.account, companyID) {
		t.Fatal("manual binding")
	}
	_ = accountDTO(f.account)
	for _, p := range []*corev1.Probability{nil, {}, {Estimate: &corev1.Probability_Value{Value: math.NaN()}}, {Estimate: &corev1.Probability_Value{Value: math.Inf(1)}}, {Estimate: &corev1.Probability_Value{Value: -0.1}}, {Estimate: &corev1.Probability_UnavailableReason{UnavailableReason: "unknown"}}} {
		if validProbability(p) {
			t.Fatal("bad probability", p)
		}
	}
	if !validProbability(&corev1.Probability{Estimate: &corev1.Probability_Value{Value: 0}}) {
		t.Fatal("zero estimate lost")
	}
}
func TestForecastRejectsIncompleteOrForeignResult(t *testing.T) {
	for name, edit := range map[string]func(*corev1.Forecast){
		"foreign account": func(f *corev1.Forecast) { f.Input.AccountId = companyID },
		"stale mismatch":  func(f *corev1.Forecast) { f.CurrentDataVersion = 2 },
		"unknown state": func(f *corev1.Forecast) {
			f.State = &corev1.Forecast_Pending{Pending: &corev1.ForecastPending{Status: 99}}
		},
		"missing model":          func(f *corev1.Forecast) { f.Input.ModelContext = nil },
		"future cutoff end":      func(f *corev1.Forecast) { f.Input.EndDate = "2026-11-23" },
		"missing readiness":      func(f *corev1.Forecast) { f.Readiness = nil },
		"insufficient succeeded": func(f *corev1.Forecast) { f.Readiness.Status = "insufficient" },
		"wrong closing balance":  func(f *corev1.Forecast) { f.GetSucceeded().Result.Metrics.ClosingBalance = rub(1) },
		"wrong minimum balance":  func(f *corev1.Forecast) { f.GetSucceeded().Result.Metrics.MinimumBalance = rub(-1) },
		"invented shortfall":     func(f *corev1.Forecast) { f.GetSucceeded().Result.Metrics.MaxShortfall = rub(1) },
		"invented first gap": func(f *corev1.Forecast) {
			f.GetSucceeded().Result.Metrics.FirstGap = &corev1.CashGap{Date: f.Input.StartDate, Shortfall: rub(1)}
		},
		"hidden gap":      func(f *corev1.Forecast) { f.GetSucceeded().Result.DailyPoints[0].Balance = rub(-1) },
		"unknown quality": func(f *corev1.Forecast) { f.Readiness.QualityIssues = []*corev1.ImportIssue{{Code: "UNKNOWN"}} },
		"short horizon": func(f *corev1.Forecast) {
			f.GetSucceeded().Result.DailyPoints = f.GetSucceeded().Result.DailyPoints[:29]
		},
		"wrong point date":    func(f *corev1.Forecast) { f.GetSucceeded().Result.DailyPoints[0].Date = "2026-10-25" },
		"wrong currency":      func(f *corev1.Forecast) { f.GetSucceeded().Result.DailyPoints[0].Balance.Currency = "USD" },
		"one sided interval":  func(f *corev1.Forecast) { f.GetSucceeded().Result.DailyPoints[0].BalanceLower = rub(0) },
		"unknown probability": func(f *corev1.Forecast) { f.GetSucceeded().Result.Metrics.GapProbability = &corev1.Probability{} },
		"nil metrics":         func(f *corev1.Forecast) { f.GetSucceeded().Result.Metrics = nil },
		"bad component":       func(f *corev1.Forecast) { f.GetSucceeded().Result.Components = []*corev1.ForecastComponent{{}} },
		"bad piece": func(f *corev1.Forecast) {
			f.GetSucceeded().Result.EffectiveOccurrences = []*corev1.EffectiveOccurrence{{}}
		},
		"bad recommendation": func(f *corev1.Forecast) { f.GetSucceeded().Result.Recommendations = []*corev1.Recommendation{{}} },
	} {
		t.Run(name, func(t *testing.T) {
			f := newFinancialFixture()
			succeeded(f)
			edit(f.forecast)
			w := financialRequest(f.handler(), "GET", financialRoot+"/forecasts/"+forecastID, "")
			if w.Code != 503 {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
}

func TestForecastMetricsUseFirstGapAndWorstDeficitSeparately(t *testing.T) {
	f := newFinancialFixture()
	succeeded(f)
	r := f.forecast.GetSucceeded().Result
	r.DailyPoints[0].Balance, r.DailyPoints[0].Outflow = rub(-10), rub(10)
	r.DailyPoints[1].Balance, r.DailyPoints[1].Outflow = rub(-30), rub(20)
	r.DailyPoints[2].Inflow = rub(30)
	r.Metrics.FirstGap = &corev1.CashGap{Date: r.DailyPoints[0].Date, Shortfall: rub(10)}
	r.Metrics.MinimumBalance, r.Metrics.MaxShortfall = rub(-30), rub(30)
	for _, readiness := range []string{"ready", "limited"} {
		f.forecast.Readiness.Status = readiness
		if w := financialRequest(f.handler(), "GET", financialRoot+"/forecasts/"+forecastID, ""); w.Code != 200 {
			t.Fatal("consistent metrics rejected", readiness, w.Code, w.Body.String())
		}
	}
	for name, edit := range map[string]func(*corev1.Forecast){
		"wrong first date":   func(f *corev1.Forecast) { f.GetSucceeded().Result.Metrics.FirstGap.Date = f.Input.EndDate },
		"wrong first amount": func(f *corev1.Forecast) { f.GetSucceeded().Result.Metrics.FirstGap.Shortfall = rub(30) },
		"missing first gap":  func(f *corev1.Forecast) { f.GetSucceeded().Result.Metrics.FirstGap = nil },
		"min int overflow":   func(f *corev1.Forecast) { f.GetSucceeded().Result.DailyPoints[0].Balance = rub(math.MinInt64) },
	} {
		t.Run(name, func(t *testing.T) {
			copyFixture := newFinancialFixture()
			copyFixture.forecast = proto.Clone(f.forecast).(*corev1.Forecast)
			edit(copyFixture.forecast)
			if w := financialRequest(copyFixture.handler(), "GET", financialRoot+"/forecasts/"+forecastID, ""); w.Code != 503 {
				t.Fatal("inconsistent metrics accepted", w.Code, w.Body.String())
			}
		})
	}
	f.forecast.Readiness.Status = "insufficient"
	f.forecast.State = &corev1.Forecast_Failed{Failed: &corev1.ForecastFailed{CompletedAt: timestamppb.New(f.forecast.CreatedAt.AsTime().Add(time.Second)), Error: &corev1.OperationError{Code: "INSUFFICIENT_HISTORY", RequestId: "safe-request"}}}
	if w := financialRequest(f.handler(), "GET", financialRoot+"/forecasts/"+forecastID, ""); w.Code != 200 {
		t.Fatal("honest failed forecast rejected", w.Code, w.Body.String())
	}
}
