package httpapi

import (
	"context"
	"log/slog"
	"net/http"
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
