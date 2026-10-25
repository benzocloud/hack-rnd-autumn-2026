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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

const paymentTestID = "019a0c65-9800-7000-8000-000000000004"
const paymentMatchTestID = "019a0c65-9800-7000-8000-000000000005"
const paymentGroupTestID = "019a0c65-9800-7000-8000-000000000006"
const paymentPath = financialRoot + "/payments"
const groupPath = financialRoot + "/reconciliation-groups"
const paymentCreateJSON = `{"direction":"outflow","amount":{"amount_minor":"1200","currency":"RUB"},"scheduled_date":"2026-10-26","counterparty_name":"Арендодатель","description":"Аренда","category_id":null}`
const paymentPatchJSON = `{"version":"1","direction":"inflow","amount":{"amount_minor":"1400","currency":"RUB"},"scheduled_date":"2026-10-27","counterparty_name":"Клиент","description":"Новая дата","category_id":"` + forecastID + `","reconciliation_group_id":"` + paymentGroupTestID + `"}`
const paymentMatchJSON = `{"transaction_id":"` + forecastID + `","amount":{"amount_minor":"100","currency":"RUB"},"payment_version":"1","transaction_version":"1"}`
const groupAdditionalJSON = `{"cutoff_date":"2026-10-25","horizon_start":"2026-10-26","horizon_end":"2026-11-24","kind":"additional","additional":{"confirmed":true}}`
const groupReplacementJSON = `{"cutoff_date":"2026-10-25","horizon_start":"2026-10-26","horizon_end":"2026-11-24","kind":"replacement","replacement":{"complete_for_horizon":true,"historical_operations":[{"transaction_id":"` + forecastID + `","version":"1"}]}}`

type paymentHTTPFixture struct {
	corev1.PaymentsServiceClient
	financial                       *financialFixture
	payment                         *corev1.ExpectedPayment
	match                           *corev1.PaymentMatch
	group                           *corev1.PaymentReconciliationGroup
	err                             error
	nilResponse, badPage, duplicate bool
	request                         proto.Message
	calls                           int
}

func newPaymentHTTPFixture() *paymentHTTPFixture {
	at := timestamppb.New(time.Date(2026, 10, 25, 7, 0, 0, 0, time.UTC))
	return &paymentHTTPFixture{financial: newFinancialFixture(), payment: &corev1.ExpectedPayment{Id: paymentTestID, AccountId: accountID, Direction: corev1.Direction_DIRECTION_OUTFLOW, Amount: rub(1200), RemainingAmount: rub(1200), ScheduledDate: "2026-10-26", CounterpartyName: "Арендодатель", Description: "Аренда", Status: corev1.PaymentStatus_PAYMENT_STATUS_PLANNED, Source: &corev1.RecordSource{Kind: corev1.RecordSourceKind_RECORD_SOURCE_KIND_MANUAL, SourceSystem: "manual_v1"}, Version: 1, CreatedAt: at}, match: &corev1.PaymentMatch{Id: paymentMatchTestID, PaymentId: paymentTestID, TransactionId: forecastID, Amount: rub(100), CreatedAt: at}, group: &corev1.PaymentReconciliationGroup{Id: paymentGroupTestID, Scope: &corev1.AccountScope{CompanyId: companyID, AccountId: accountID}, Version: 1, Status: corev1.ReconciliationStatus_RECONCILIATION_STATUS_ACTIVE, CutoffDate: "2026-10-25", HorizonStart: "2026-10-26", HorizonEnd: "2026-11-24", ConfirmedBy: "actor", ConfirmedAt: at, Reconciliation: &corev1.PaymentReconciliationGroup_Additional{Additional: &corev1.AdditionalObligationConfirmation{Confirmed: true}}}}
}
func (f *paymentHTTPFixture) handler() http.Handler {
	return NewWithCore(f.financial, "https://localhost", slog.New(slog.DiscardHandler), func(context.Context) bool { return true }, CoreClients{Ledger: f.financial, Payments: f})
}
func (f *paymentHTTPFixture) capture(r proto.Message) { f.calls++; f.request = proto.Clone(r) }
func (f *paymentHTTPFixture) page() *corev1.PageResponse {
	if f.badPage {
		return nil
	}
	return &corev1.PageResponse{}
}
func (f *paymentHTTPFixture) ListPayments(_ context.Context, r *corev1.ListPaymentsRequest, _ ...grpc.CallOption) (*corev1.ListPaymentsResponse, error) {
	f.capture(r)
	if f.nilResponse {
		return nil, f.err
	}
	items := []*corev1.ExpectedPayment{f.payment}
	if f.duplicate {
		items = append(items, f.payment)
	}
	return &corev1.ListPaymentsResponse{Items: items, Pagination: f.page()}, f.err
}
func (f *paymentHTTPFixture) GetPayment(_ context.Context, r *corev1.GetPaymentRequest, _ ...grpc.CallOption) (*corev1.GetPaymentResponse, error) {
	f.capture(r)
	if f.nilResponse {
		return nil, f.err
	}
	return &corev1.GetPaymentResponse{Payment: f.payment}, f.err
}
func (f *paymentHTTPFixture) CreateManualPayment(_ context.Context, r *corev1.CreateManualPaymentRequest, _ ...grpc.CallOption) (*corev1.CreateManualPaymentResponse, error) {
	f.capture(r)
	if f.nilResponse {
		return nil, f.err
	}
	return &corev1.CreateManualPaymentResponse{Payment: f.payment, DataVersion: 2}, f.err
}
func (f *paymentHTTPFixture) UpdatePayment(_ context.Context, r *corev1.UpdatePaymentRequest, _ ...grpc.CallOption) (*corev1.UpdatePaymentResponse, error) {
	f.capture(r)
	if f.nilResponse {
		return nil, f.err
	}
	return &corev1.UpdatePaymentResponse{Payment: f.payment, DataVersion: 2}, f.err
}
func (f *paymentHTTPFixture) CancelPayment(_ context.Context, r *corev1.CancelPaymentRequest, _ ...grpc.CallOption) (*corev1.CancelPaymentResponse, error) {
	f.capture(r)
	if f.nilResponse {
		return nil, f.err
	}
	return &corev1.CancelPaymentResponse{Payment: f.payment, DataVersion: 2}, f.err
}
func (f *paymentHTTPFixture) ListPaymentMatches(_ context.Context, r *corev1.ListPaymentMatchesRequest, _ ...grpc.CallOption) (*corev1.ListPaymentMatchesResponse, error) {
	f.capture(r)
	if f.nilResponse {
		return nil, f.err
	}
	items := []*corev1.PaymentMatch{f.match}
	if f.duplicate {
		items = append(items, f.match)
	}
	return &corev1.ListPaymentMatchesResponse{Items: items, Pagination: f.page()}, f.err
}
func (f *paymentHTTPFixture) MatchPayment(_ context.Context, r *corev1.MatchPaymentRequest, _ ...grpc.CallOption) (*corev1.MatchPaymentResponse, error) {
	f.capture(r)
	if f.nilResponse {
		return nil, f.err
	}
	return &corev1.MatchPaymentResponse{Match: f.match, PaymentVersion: 2, DataVersion: 2}, f.err
}
func (f *paymentHTTPFixture) RemovePaymentMatch(_ context.Context, r *corev1.RemovePaymentMatchRequest, _ ...grpc.CallOption) (*corev1.RemovePaymentMatchResponse, error) {
	f.capture(r)
	if f.nilResponse {
		return nil, f.err
	}
	return &corev1.RemovePaymentMatchResponse{Payment: f.payment, DataVersion: 2}, f.err
}
func (f *paymentHTTPFixture) ListReconciliationGroups(_ context.Context, r *corev1.ListReconciliationGroupsRequest, _ ...grpc.CallOption) (*corev1.ListReconciliationGroupsResponse, error) {
	f.capture(r)
	if f.nilResponse {
		return nil, f.err
	}
	items := []*corev1.PaymentReconciliationGroup{f.group}
	if f.duplicate {
		items = append(items, f.group)
	}
	return &corev1.ListReconciliationGroupsResponse{Items: items, Pagination: f.page()}, f.err
}
func (f *paymentHTTPFixture) GetReconciliationGroup(_ context.Context, r *corev1.GetReconciliationGroupRequest, _ ...grpc.CallOption) (*corev1.GetReconciliationGroupResponse, error) {
	f.capture(r)
	if f.nilResponse {
		return nil, f.err
	}
	return &corev1.GetReconciliationGroupResponse{Group: f.group}, f.err
}
func (f *paymentHTTPFixture) CreateReconciliationGroup(_ context.Context, r *corev1.CreateReconciliationGroupRequest, _ ...grpc.CallOption) (*corev1.CreateReconciliationGroupResponse, error) {
	f.capture(r)
	if f.nilResponse {
		return nil, f.err
	}
	return &corev1.CreateReconciliationGroupResponse{Group: f.group, DataVersion: 2}, f.err
}
func (f *paymentHTTPFixture) RevokeReconciliationGroup(_ context.Context, r *corev1.RevokeReconciliationGroupRequest, _ ...grpc.CallOption) (*corev1.RevokeReconciliationGroupResponse, error) {
	f.capture(r)
	if f.nilResponse {
		return nil, f.err
	}
	return &corev1.RevokeReconciliationGroupResponse{Group: f.group, DataVersion: 2}, f.err
}

type paymentRouteCase struct {
	name, method, path, body string
	code                     int
}

func paymentRouteCases() []paymentRouteCase {
	return []paymentRouteCase{
		{"list", "GET", paymentPath + "?limit=2&cursor=abc&status=planned&from=2026-10-01&to=2026-10-31", "", 200},
		{"get", "GET", paymentPath + "/" + paymentTestID, "", 200},
		{"create", "POST", paymentPath, paymentCreateJSON, 201},
		{"update", "PATCH", paymentPath + "/" + paymentTestID, paymentPatchJSON, 200},
		{"cancel", "DELETE", paymentPath + "/" + paymentTestID + "?version=1", "", 204},
		{"history", "GET", paymentPath + "/" + paymentTestID + "/matches?limit=2&cursor=abc", "", 200},
		{"match", "POST", paymentPath + "/" + paymentTestID + "/matches", paymentMatchJSON, 201},
		{"unmatch", "DELETE", paymentPath + "/" + paymentTestID + "/matches/" + paymentMatchTestID + "?payment_version=1", "", 204},
		{"groups", "GET", groupPath + "?limit=2&cursor=abc&include_revoked=true", "", 200},
		{"group", "GET", groupPath + "/" + paymentGroupTestID, "", 200},
		{"additional", "POST", groupPath, groupAdditionalJSON, 201},
		{"replacement", "POST", groupPath, groupReplacementJSON, 201},
		{"revoke", "POST", groupPath + "/" + paymentGroupTestID + "/revoke", `{"version":"1"}`, 200},
	}
}
func TestPaymentHTTPAdaptersPreserveScopeContextAndEconomics(t *testing.T) {
	t.Parallel()
	for _, tc := range paymentRouteCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := newPaymentHTTPFixture()
			w := financialRequest(f.handler(), tc.method, tc.path, tc.body)
			if w.Code != tc.code {
				t.Fatalf("status=%d body=%s", w.Code, w.Body)
			}
			if f.calls != 1 {
				t.Fatalf("Core calls=%d", f.calls)
			}
			encoded, err := json.Marshal(f.request)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(encoded), accountID) || !strings.Contains(string(encoded), companyID) || !strings.Contains(string(encoded), "opaque-session") {
				t.Fatalf("scope/browser context missing: %s", encoded)
			}
			if tc.method != "GET" && !strings.Contains(string(encoded), strings.Repeat("c", 24)) {
				t.Fatalf("CSRF context missing: %s", encoded)
			}
			switch r := f.request.(type) {
			case *corev1.CreateManualPaymentRequest:
				if r.Amount.AmountMinor != 1200 || r.IdempotencyKey != "financial-key-123" {
					t.Fatal("create economics/key changed")
				}
			case *corev1.MatchPaymentRequest:
				if r.Amount.AmountMinor != 100 || r.ExpectedPaymentVersion != 1 || r.ExpectedTransactionVersion != 1 {
					t.Fatal("match CAS/economics changed")
				}
			case *corev1.UpdatePaymentRequest:
				if r.Amount.AmountMinor != 1400 || r.GetReconciliationGroupId() != paymentGroupTestID {
					t.Fatal("patch fields lost")
				}
			}
		})
	}
}
func TestPaymentHTTPAdaptersFailClosed(t *testing.T) {
	t.Parallel()
	for _, tc := range paymentRouteCases() {
		t.Run(tc.name, func(t *testing.T) {
			for _, failure := range []string{"account unavailable", "Core unavailable", "malformed projection", "nil response"} {
				t.Run(failure, func(t *testing.T) {
					f := newPaymentHTTPFixture()
					switch failure {
					case "account unavailable":
						f.financial.err = status.Error(codes.Unavailable, "unavailable")
					case "Core unavailable":
						f.err = status.Error(codes.Unavailable, "unavailable")
					case "malformed projection":
						f.payment.AccountId = paymentMatchTestID
						f.match.PaymentId = paymentMatchTestID
						f.group.Scope.AccountId = paymentMatchTestID
					case "nil response":
						f.nilResponse = true
					}
					w := financialRequest(f.handler(), tc.method, tc.path, tc.body)
					if w.Code != 503 {
						t.Fatalf("status=%d body=%s", w.Code, w.Body)
					}
					if failure == "account unavailable" && f.calls != 0 {
						t.Fatal("financial RPC called after failed account authorization")
					}
				})
			}
		})
	}
}

func TestPaymentHTTPRejectsMalformedRoutesAndQueries(t *testing.T) {
	t.Parallel()
	tests := []paymentRouteCase{
		{"payment unsupported", "PUT", paymentPath, "", 405}, {"get unsupported", "POST", paymentPath + "/" + paymentTestID, "", 405}, {"match unsupported", "PUT", paymentPath + "/" + paymentTestID + "/matches", "", 405}, {"unmatch unsupported", "GET", paymentPath + "/" + paymentTestID + "/matches/" + paymentMatchTestID, "", 405},
		{"group unsupported", "PATCH", groupPath, "", 405}, {"group resource unsupported", "DELETE", groupPath + "/" + paymentGroupTestID, "", 405}, {"revoke unsupported", "DELETE", groupPath + "/" + paymentGroupTestID + "/revoke", "", 405},
		{"bad company", "GET", "/api/v1/companies/no/payments", "", 422}, {"bad payment", "GET", paymentPath + "/no", "", 422}, {"bad match", "DELETE", paymentPath + "/" + paymentTestID + "/matches/no?payment_version=1", "", 422}, {"bad group", "GET", groupPath + "/no", "", 422},
		{"unknown route", "GET", paymentPath + "/" + paymentTestID + "/unknown", "", 404}, {"unknown group route", "GET", groupPath + "/" + paymentGroupTestID + "/unknown", "", 404},
		{"get query", "GET", paymentPath + "/" + paymentTestID + "?extra=1", "", 400}, {"group query", "GET", groupPath + "/" + paymentGroupTestID + "?extra=1", "", 400},
		{"bad list query", "GET", paymentPath + "?extra=1", "", 400}, {"bad match query", "GET", paymentPath + "/" + paymentTestID + "/matches?extra=1", "", 400},
		{"cancel no CAS", "DELETE", paymentPath + "/" + paymentTestID, "", 422}, {"cancel numeric format", "DELETE", paymentPath + "/" + paymentTestID + "?version=01", "", 422}, {"cancel extra query", "DELETE", paymentPath + "/" + paymentTestID + "?version=1&extra=1", "", 422}, {"unmatch no CAS", "DELETE", paymentPath + "/" + paymentTestID + "/matches/" + paymentMatchTestID, "", 422},
		{"bad create", "POST", paymentPath, `{}`, 422}, {"bad update", "PATCH", paymentPath + "/" + paymentTestID, `{}`, 422}, {"bad match", "POST", paymentPath + "/" + paymentTestID + "/matches", `{}`, 422}, {"bad group create", "POST", groupPath, `{}`, 422}, {"bad revoke", "POST", groupPath + "/" + paymentGroupTestID + "/revoke", `{}`, 422},
	}
	for _, query := range []string{"limit=0", "limit=101", "limit=01", "cursor=%20", "status=unknown", "from=invalid", "to=invalid", "from=2026-10-30&to=2026-10-01", "limit=1&limit=2", "%bad"} {
		tests = append(tests, paymentRouteCase{"list " + query, "GET", paymentPath + "?" + query, "", 400})
	}
	for _, query := range []string{"limit=0", "limit=01", "cursor=%20", "include_revoked=1", "extra=1", "limit=1&limit=2", "%bad"} {
		tests = append(tests, paymentRouteCase{"groups " + query, "GET", groupPath + "?" + query, "", 400})
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newPaymentHTTPFixture()
			w := financialRequest(f.handler(), tc.method, tc.path, tc.body)
			if w.Code != tc.code {
				t.Fatalf("status=%d body=%s", w.Code, w.Body)
			}
			if f.calls != 0 {
				t.Fatal("invalid input reached Core")
			}
		})
	}
}
func TestPaymentHTTPWriteSecurityAndMissingDependencies(t *testing.T) {
	t.Parallel()
	for _, tc := range paymentRouteCases() {
		t.Run(tc.name, func(t *testing.T) {
			if tc.method != "GET" {
				for _, invalid := range []string{"origin", "csrf"} {
					t.Run(invalid, func(t *testing.T) {
						f := newPaymentHTTPFixture()
						r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
						r.Header.Set("Content-Type", "application/json")
						r.Header.Set("Origin", "https://localhost")
						r.Header.Set("X-CSRF-Token", strings.Repeat("c", 24))
						if invalid == "origin" {
							r.Header.Set("Origin", "https://foreign.example")
						} else {
							r.Header.Del("X-CSRF-Token")
						}
						w := httptest.NewRecorder()
						f.handler().ServeHTTP(w, r)
						if w.Code != 403 || f.calls != 0 {
							t.Fatalf("security status=%d calls=%d", w.Code, f.calls)
						}
					})
				}
			}
			h := NewWithCore(newFinancialFixture(), "https://localhost", slog.New(slog.DiscardHandler), func(context.Context) bool { return true }, CoreClients{})
			w := financialRequest(h, tc.method, tc.path, tc.body)
			if w.Code != 503 {
				t.Fatalf("dependency status=%d", w.Code)
			}
		})
	}
}
func TestPaymentHTTPListRejectsInvalidPaginationAndDuplicateIDs(t *testing.T) {
	t.Parallel()
	for _, path := range []string{paymentPath, paymentPath + "/" + paymentTestID + "/matches", groupPath} {
		for _, mode := range []string{"page", "duplicate"} {
			t.Run(path+mode, func(t *testing.T) {
				f := newPaymentHTTPFixture()
				f.badPage = mode == "page"
				f.duplicate = mode == "duplicate"
				w := financialRequest(f.handler(), "GET", path, "")
				if w.Code != 503 {
					t.Fatalf("status=%d body=%s", w.Code, w.Body)
				}
			})
		}
	}
}
func TestPaymentHTTPPropagatesStaleAndForeignResourceErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range paymentRouteCases() {
		for _, failure := range []struct {
			code   codes.Code
			reason corev1.CoreErrorReason
			http   int
		}{{codes.Aborted, corev1.CoreErrorReason_CORE_ERROR_REASON_VERSION_CONFLICT, 409}, {codes.NotFound, corev1.CoreErrorReason_CORE_ERROR_REASON_NOT_FOUND, 404}} {
			t.Run(tc.name+failure.code.String(), func(t *testing.T) {
				f := newPaymentHTTPFixture()
				st, err := status.New(failure.code, "fixture").WithDetails(&corev1.CoreErrorDetail{Reason: failure.reason})
				if err != nil {
					t.Fatal(err)
				}
				f.err = st.Err()
				w := financialRequest(f.handler(), tc.method, tc.path, tc.body)
				if w.Code != failure.http {
					t.Fatalf("status=%d body=%s", w.Code, w.Body)
				}
			})
		}
	}
}

func TestPaymentHTTPProjectsSettlementStatesAndReplacementEvidence(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		status    corev1.PaymentStatus
		remaining int64
		name      string
	}{{corev1.PaymentStatus_PAYMENT_STATUS_PLANNED, 1200, "planned"}, {corev1.PaymentStatus_PAYMENT_STATUS_PARTIALLY_MATCHED, 800, "partially_matched"}, {corev1.PaymentStatus_PAYMENT_STATUS_MATCHED, 0, "matched"}, {corev1.PaymentStatus_PAYMENT_STATUS_CANCELLED, 800, "cancelled"}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newPaymentHTTPFixture()
			f.payment.Status = tc.status
			f.payment.RemainingAmount = rub(tc.remaining)
			f.payment.Source = &corev1.RecordSource{Kind: corev1.RecordSourceKind_RECORD_SOURCE_KIND_FILE, SourceSystem: "dataset_v1", SourceCompanyId: proto.String("C001"), ExternalPaymentId: proto.String("PAY1")}
			w := financialRequest(f.handler(), "GET", paymentPath+"?status="+tc.name, "")
			if w.Code != 200 || !strings.Contains(w.Body.String(), tc.name) {
				t.Fatalf("status=%d body=%s", w.Code, w.Body)
			}
		})
	}
	f := newPaymentHTTPFixture()
	f.group.Status = corev1.ReconciliationStatus_RECONCILIATION_STATUS_REVOKED
	f.group.Reconciliation = &corev1.PaymentReconciliationGroup_Replacement{Replacement: &corev1.CompleteReplacement{CompleteForHorizon: true, HistoricalOperations: []*corev1.HistoricalOperationRef{{TransactionId: forecastID, Version: 2}}}}
	w := financialRequest(f.handler(), "GET", groupPath+"/"+paymentGroupTestID, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"version":"2"`) || !strings.Contains(w.Body.String(), `"kind":"replacement"`) {
		t.Fatalf("replacement projection status=%d body=%s", w.Code, w.Body)
	}
}
func TestPaymentHTTPProjectionGuards(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		mutate func(*paymentHTTPFixture)
	}{
		{"unknown status", func(f *paymentHTTPFixture) { f.payment.Status = 99 }},
		{"planned remaining", func(f *paymentHTTPFixture) { f.payment.RemainingAmount = rub(100) }},
		{"partial full remaining", func(f *paymentHTTPFixture) { f.payment.Status = corev1.PaymentStatus_PAYMENT_STATUS_PARTIALLY_MATCHED }},
		{"matched nonzero", func(f *paymentHTTPFixture) { f.payment.Status = corev1.PaymentStatus_PAYMENT_STATUS_MATCHED }},
		{"cancelled zero", func(f *paymentHTTPFixture) {
			f.payment.Status = corev1.PaymentStatus_PAYMENT_STATUS_CANCELLED
			f.payment.RemainingAmount = rub(0)
		}},
		{"manual foreign source", func(f *paymentHTTPFixture) { f.payment.Source.SourceCompanyId = proto.String("foreign") }},
		{"imported missing external", func(f *paymentHTTPFixture) { f.payment.Source.Kind = corev1.RecordSourceKind_RECORD_SOURCE_KIND_FILE }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newPaymentHTTPFixture()
			tc.mutate(f)
			w := financialRequest(f.handler(), "GET", paymentPath+"/"+paymentTestID, "")
			if w.Code != 503 {
				t.Fatalf("status=%d body=%s", w.Code, w.Body)
			}
		})
	}
	for _, tc := range []struct {
		name   string
		mutate func(*corev1.PaymentReconciliationGroup)
	}{
		{"horizon", func(g *corev1.PaymentReconciliationGroup) { g.HorizonEnd = "2026-11-23" }},
		{"unspecified status", func(g *corev1.PaymentReconciliationGroup) {
			g.Status = corev1.ReconciliationStatus_RECONCILIATION_STATUS_UNSPECIFIED
		}},
		{"unknown status", func(g *corev1.PaymentReconciliationGroup) { g.Status = 99 }},
		{"missing branch", func(g *corev1.PaymentReconciliationGroup) { g.Reconciliation = nil }},
		{"unconfirmed additional", func(g *corev1.PaymentReconciliationGroup) { g.GetAdditional().Confirmed = false }},
		{"unproven replacement", func(g *corev1.PaymentReconciliationGroup) {
			g.Reconciliation = &corev1.PaymentReconciliationGroup_Replacement{Replacement: &corev1.CompleteReplacement{}}
		}},
		{"foreign replacement duplicate", func(g *corev1.PaymentReconciliationGroup) {
			g.Reconciliation = &corev1.PaymentReconciliationGroup_Replacement{Replacement: &corev1.CompleteReplacement{CompleteForHorizon: true, HistoricalOperations: []*corev1.HistoricalOperationRef{{TransactionId: forecastID, Version: 1}, {TransactionId: forecastID, Version: 1}}}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newPaymentHTTPFixture()
			tc.mutate(f.group)
			w := financialRequest(f.handler(), "GET", groupPath+"/"+paymentGroupTestID, "")
			if w.Code != 503 {
				t.Fatalf("status=%d body=%s", w.Code, w.Body)
			}
		})
	}
}
