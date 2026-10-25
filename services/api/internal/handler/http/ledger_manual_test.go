package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

type manualHTTPFixture struct {
	*financialFixture
	request   *pb.CreateManualTransactionRequest
	manualErr error
}

func (f *manualHTTPFixture) CreateManualTransaction(_ context.Context, r *pb.CreateManualTransactionRequest, _ ...grpc.CallOption) (*pb.CreateManualTransactionResponse, error) {
	f.request = r
	if f.manualErr != nil {
		return nil, f.manualErr
	}
	return &pb.CreateManualTransactionResponse{Transaction: &pb.Transaction{Id: forecastID, AccountId: r.Scope.AccountId, OperationDate: r.OperationDate, Direction: r.Direction, Amount: r.Amount, CounterpartyInn: r.CounterpartyInn, CounterpartyName: r.CounterpartyName, PaymentPurpose: r.PaymentPurpose, CategoryId: r.CategoryId, Source: &pb.RecordSource{Kind: pb.RecordSourceKind_RECORD_SOURCE_KIND_MANUAL, SourceSystem: "manual_v1"}, Version: 1, CreatedAt: timestamppb.New(time.Now())}, DataVersion: r.ExpectedDataVersion + 1}, nil
}

func (f *manualHTTPFixture) handler() http.Handler {
	return NewWithCore(f, "https://localhost", slog.New(slog.DiscardHandler), func(context.Context) bool { return true }, CoreClients{Companies: f, Ledger: f, Forecasts: f})
}

const manualHTTPBody = `{"operation_date":"2026-10-24","direction":"inflow","amount":{"amount_minor":"9007199254740993","currency":"RUB"},"counterparty_inn":"0012345678","counterparty_name":"Client","payment_purpose":"Manual supplement","expected_data_version":"2"}`

func TestManualHTTPWriteAcceptsImportedAccountAndPreservesReplayVersion(t *testing.T) {
	f := &manualHTTPFixture{financialFixture: newFinancialFixture()}
	f.account.DataVersion = 5
	f.account.TransactionSource = &pb.FactualSourceBinding{Mode: pb.FactualSourceMode_FACTUAL_SOURCE_MODE_IMPORTED, SourceSystem: "dataset_v1", SourceCompanyId: new("C001")}
	w := financialRequest(f.handler(), http.MethodPost, financialRoot+"/transactions", manualHTTPBody)
	if w.Code != http.StatusCreated || f.request == nil || f.request.Amount.AmountMinor != 9007199254740993 || f.request.GetCounterpartyInn() != "0012345678" || f.request.ExpectedDataVersion != 2 || f.request.Scope.CompanyId != companyID || f.request.Context.Browser.CookieToken != "opaque-session" || !strings.Contains(w.Body.String(), `"kind":"manual"`) {
		t.Fatal(w.Code, w.Body.String(), f.request)
	}
}

func TestManualHTTPStrictBodyAndDuplicateError(t *testing.T) {
	for _, body := range []string{strings.Replace(manualHTTPBody, `"inflow"`, `"sideways"`, 1), strings.Replace(manualHTTPBody, `"9007199254740993"`, `9007199254740993`, 1), strings.Replace(manualHTTPBody, `"Client"`, `null`, 1), strings.Replace(manualHTTPBody, `"expected_data_version":"2"`, `"expected_data_version":"2","source":"file"`, 1), strings.Replace(manualHTTPBody, `"direction":"inflow"`, `"direction":"inflow","direction":"outflow"`, 1)} {
		f := &manualHTTPFixture{financialFixture: newFinancialFixture()}
		w := financialRequest(f.handler(), http.MethodPost, financialRoot+"/transactions", body)
		if w.Code != 422 || f.request != nil {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	f := &manualHTTPFixture{financialFixture: newFinancialFixture()}
	st, err := status.New(codes.FailedPrecondition, "private").WithDetails(&pb.CoreErrorDetail{Reason: pb.CoreErrorReason_CORE_ERROR_REASON_TRANSACTION_DUPLICATE_SUSPECTED})
	if err != nil {
		t.Fatal(err)
	}
	f.manualErr = st.Err()
	w := financialRequest(f.handler(), http.MethodPost, financialRoot+"/transactions", manualHTTPBody)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "TRANSACTION_DUPLICATE_SUSPECTED") || strings.Contains(w.Body.String(), "private") {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestManualHTTPRejectsMalformedMoneyAndOptionalReferencesBeforeCore(t *testing.T) {
	for _, test := range []struct {
		name, old, replacement string
	}{
		{"missing date", `"operation_date":"2026-10-24",`, ""},
		{"noncanonical version", `"expected_data_version":"2"`, `"expected_data_version":"02"`},
		{"amount is not an object", `{"amount_minor":"9007199254740993","currency":"RUB"}`, `[]`},
		{"extra money field", `"currency":"RUB"`, `"currency":"RUB","scale":2`},
		{"foreign currency", `"currency":"RUB"`, `"currency":"USD"`},
		{"zero amount", `"amount_minor":"9007199254740993"`, `"amount_minor":"0"`},
		{"invalid INN", `"counterparty_inn":"0012345678"`, `"counterparty_inn":"123"`},
		{"numeric INN", `"counterparty_inn":"0012345678"`, `"counterparty_inn":1234567890`},
		{"invalid category", `"expected_data_version":"2"`, `"expected_data_version":"2","category_id":"other"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := &manualHTTPFixture{financialFixture: newFinancialFixture()}
			body := strings.Replace(manualHTTPBody, test.old, test.replacement, 1)
			w := financialRequest(f.handler(), http.MethodPost, financialRoot+"/transactions", body)
			if w.Code != http.StatusUnprocessableEntity || f.request != nil || f.calls != 0 {
				t.Fatalf("malformed input reached Core: status=%d body=%s request=%v calls=%d", w.Code, w.Body.String(), f.request, f.calls)
			}
		})
	}
}

func TestManualHTTPOutflowPreservesCategoryAndNullableINN(t *testing.T) {
	f := &manualHTTPFixture{financialFixture: newFinancialFixture()}
	body := strings.Replace(manualHTTPBody, `"direction":"inflow"`, `"direction":"outflow"`, 1)
	body = strings.Replace(body, `"counterparty_inn":"0012345678"`, `"counterparty_inn":null`, 1)
	body = strings.Replace(body, `"expected_data_version":"2"`, `"expected_data_version":"2","category_id":"`+forecastID+`"`, 1)
	w := financialRequest(f.handler(), http.MethodPost, financialRoot+"/transactions", body)
	if w.Code != http.StatusCreated || f.request == nil || f.request.Direction != pb.Direction_DIRECTION_OUTFLOW || f.request.CounterpartyInn != nil || f.request.GetCategoryId() != forecastID || !strings.Contains(w.Body.String(), `"counterparty_inn":null`) || !strings.Contains(w.Body.String(), `"category_id":"`+forecastID+`"`) {
		t.Fatal(w.Code, w.Body.String(), f.request)
	}
}

func TestManualHTTPAccountFailureDoesNotCreateTransaction(t *testing.T) {
	f := &manualHTTPFixture{financialFixture: newFinancialFixture()}
	f.err = status.Error(codes.Unavailable, "private account failure")
	w := financialRequest(f.handler(), http.MethodPost, financialRoot+"/transactions", manualHTTPBody)
	if w.Code != http.StatusServiceUnavailable || f.request != nil || strings.Contains(w.Body.String(), "private") {
		t.Fatal(w.Code, w.Body.String(), f.request)
	}
}

func TestManualHTTPRejectsInvalidJSONEnvelopeBeforeCore(t *testing.T) {
	for _, test := range []struct {
		name, contentType, body string
	}{
		{"wrong content type", "text/plain", manualHTTPBody},
		{"oversized body", "application/json", `{"payment_purpose":"` + strings.Repeat("x", 16<<10) + `"}`},
		{"invalid UTF-8", "application/json", string([]byte{'{', 0xff, '}'})},
		{"extra JSON value", "application/json", manualHTTPBody + `{}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := &manualHTTPFixture{financialFixture: newFinancialFixture()}
			r := httptest.NewRequest(http.MethodPost, financialRoot+"/transactions", strings.NewReader(test.body))
			r.Header.Set("Content-Type", test.contentType)
			r.Header.Set("Origin", "https://localhost")
			r.Header.Set("X-CSRF-Token", strings.Repeat("c", 24))
			r.Header.Set("Idempotency-Key", "financial-key-123")
			r.AddCookie(&http.Cookie{Name: "__Host-saldo_session", Value: "opaque-session"})
			w := httptest.NewRecorder()
			f.handler().ServeHTTP(w, r)
			if w.Code != http.StatusUnprocessableEntity || f.request != nil || f.calls != 0 {
				t.Fatalf("invalid JSON envelope reached Core: %d %s", w.Code, w.Body.String())
			}
		})
	}
}
