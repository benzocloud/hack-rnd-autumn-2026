package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

type ledgerFixture struct {
	*financialFixture
	transaction *corev1.Transaction
	category    *corev1.Category
	query       *corev1.ListTransactionsRequest
	update      *corev1.UpdateTransactionCategoryRequest
	failure     error
}

func (f *ledgerFixture) ListTransactions(_ context.Context, r *corev1.ListTransactionsRequest, _ ...grpc.CallOption) (*corev1.ListTransactionsResponse, error) {
	f.query = proto.Clone(r).(*corev1.ListTransactionsRequest)
	return &corev1.ListTransactionsResponse{Items: []*corev1.Transaction{f.transaction}, Pagination: &corev1.PageResponse{}}, f.failure
}
func (f *ledgerFixture) ListCategories(context.Context, *corev1.ListCategoriesRequest, ...grpc.CallOption) (*corev1.ListCategoriesResponse, error) {
	return &corev1.ListCategoriesResponse{Items: []*corev1.Category{f.category}, Pagination: &corev1.PageResponse{}}, f.failure
}
func (f *ledgerFixture) UpdateTransactionCategory(_ context.Context, r *corev1.UpdateTransactionCategoryRequest, _ ...grpc.CallOption) (*corev1.UpdateTransactionCategoryResponse, error) {
	f.update = proto.Clone(r).(*corev1.UpdateTransactionCategoryRequest)
	return &corev1.UpdateTransactionCategoryResponse{Transaction: f.transaction, DataVersion: 2}, f.failure
}
func ledgerFixtureHandler(f *ledgerFixture, imports corev1.ImportsServiceClient) http.Handler {
	return NewWithCore(f.financialFixture, "https://localhost", slog.New(slog.DiscardHandler), func(context.Context) bool { return true }, CoreClients{Ledger: f, Imports: imports})
}
func newLedgerFixture() *ledgerFixture {
	f := newFinancialFixture()
	return &ledgerFixture{financialFixture: f, category: &corev1.Category{Id: forecastID, Name: "Операционные расходы", SemanticKind: "operating_expense"}, transaction: &corev1.Transaction{Id: forecastID, AccountId: accountID, OperationDate: "2026-09-30", Direction: corev1.Direction_DIRECTION_OUTFLOW, Amount: rub(9007199254740993), CounterpartyInn: proto.String("0012345678"), CounterpartyName: "Поставщик", PaymentPurpose: "", Source: &corev1.RecordSource{Kind: corev1.RecordSourceKind_RECORD_SOURCE_KIND_FILE, SourceSystem: "dataset_v1", SourceCompanyId: proto.String("C001"), ExternalOperationId: proto.String("OP1"), ImportId: proto.String(companyID)}, Version: 2, CreatedAt: f.forecast.CreatedAt}}
}
func TestLedgerHTTPReadAndNullableCategoryMapping(t *testing.T) {
	f := newLedgerFixture()
	h := ledgerFixtureHandler(f, nil)
	for _, path := range []string{financialRoot + "/categories", financialRoot + "/transactions?from=2026-09-01&to=2026-09-30&direction=outflow&category_id=" + forecastID + "&limit=20"} {
		w := financialRequest(h, http.MethodGet, path, "")
		if w.Code != 200 {
			t.Fatal(path, w.Code, w.Body.String())
		}
		if strings.Contains(path, "categories") && !strings.Contains(w.Body.String(), `"direction":null`) {
			t.Fatal(w.Body.String())
		}
		if strings.Contains(path, "transactions") && (!strings.Contains(w.Body.String(), `"amount_minor":"9007199254740993"`) || !strings.Contains(w.Body.String(), `"counterparty_inn":"0012345678"`)) {
			t.Fatal(w.Body.String())
		}
	}
	if f.query.Scope.AccountId != accountID || f.query.Context.CookieToken != "opaque-session" || f.query.Dates.GetFrom() != "2026-09-01" || f.query.GetDirection() != corev1.Direction_DIRECTION_OUTFLOW {
		t.Fatal("lost scope/filter/context", f.query)
	}
	for _, value := range []string{`null`, `"` + companyID + `"`} {
		w := financialRequest(h, http.MethodPatch, financialRoot+"/transactions/"+forecastID+"/category", `{"version":"2","category_id":`+value+`}`)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		if f.update.ExpectedVersion != 2 || f.update.Scope.AccountId != accountID || f.update.Context.CsrfToken != strings.Repeat("c", 24) {
			t.Fatal(f.update)
		}
		if value == "null" && f.update.GetClearCategory() == nil {
			t.Fatal("clear lost")
		}
		if value != "null" && f.update.GetCategoryId() != companyID {
			t.Fatal("category assignment lost")
		}
	}
}

func TestLegacyTransactionTimestampStaysUnknown(t *testing.T) {
	transaction := newLedgerFixture().transaction
	transaction.CreatedAt = nil
	value, err := transactionDTO(transaction, accountID)
	if err != nil || value["created_at"] != nil {
		t.Fatalf("legacy timestamp should stay null: %#v, %v", value, err)
	}
	encoded, err := json.Marshal(value)
	if err != nil || !strings.Contains(string(encoded), `"created_at":null`) {
		t.Fatalf("legacy timestamp should serialize as null: %s, %v", encoded, err)
	}
}

func TestLegacyTransactionTimestampHTTPReadAndCategoryUpdate(t *testing.T) {
	f := newLedgerFixture()
	f.transaction.CreatedAt = nil
	h := ledgerFixtureHandler(f, nil)
	for _, test := range []struct {
		name, method, path, body string
		list                     bool
	}{
		{name: "list", method: http.MethodGet, path: financialRoot + "/transactions", list: true},
		{name: "category clear", method: http.MethodPatch, path: financialRoot + "/transactions/" + forecastID + "/category", body: `{"version":"2","category_id":null}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := financialRequest(h, test.method, test.path, test.body)
			if w.Code != http.StatusOK {
				t.Fatalf("legacy projection failed: %d %s", w.Code, w.Body.String())
			}
			var value map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil {
				t.Fatal(err)
			}
			if test.list {
				items, ok := value["items"].([]any)
				if !ok || len(items) != 1 {
					t.Fatal(value)
				}
				value, ok = items[0].(map[string]any)
				if !ok {
					t.Fatal(items)
				}
			}
			created, present := value["created_at"]
			if !present || created != nil {
				t.Fatalf("unknown creation time was omitted or fabricated: %#v", value)
			}
			if value["id"] != forecastID || value["version"] != "2" {
				t.Fatalf("projection changed resource identity/version: %#v", value)
			}
		})
	}
	if f.update == nil || f.update.GetClearCategory() == nil {
		t.Fatal("category update was not sent to Core")
	}
	if f.transaction.CreatedAt != nil {
		t.Fatal("legacy creation timestamp was fabricated")
	}
}

func TestTransactionTimestampPreservesKnownAndRejectsMalformed(t *testing.T) {
	f := newLedgerFixture()
	want := utc(f.transaction.CreatedAt)
	value, err := transactionDTO(f.transaction, accountID)
	if err != nil || value["created_at"] != want {
		t.Fatalf("known timestamp changed: %#v %v", value, err)
	}
	f.transaction.CreatedAt = &timestamppb.Timestamp{Seconds: 253402300800}
	if _, err := transactionDTO(f.transaction, accountID); err == nil {
		t.Fatal("malformed present timestamp was accepted as legacy absence")
	}
	if w := financialRequest(ledgerFixtureHandler(f, nil), http.MethodGet, financialRoot+"/transactions", ""); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("invalid upstream timestamp must fail closed: %d", w.Code)
	}
}
func TestLedgerInvalidInputAndUpstreamFailClosed(t *testing.T) {
	f := newLedgerFixture()
	h := ledgerFixtureHandler(f, nil)
	for _, query := range []string{"from=2026-10-02&to=2026-10-01", "direction=sideways", "category_id=other", "limit=101", "direction=inflow&direction=outflow", "certified=true", "from="} {
		if w := financialRequest(h, "GET", financialRoot+"/transactions?"+query, ""); w.Code != 400 {
			t.Fatal(query, w.Code, w.Body.String())
		}
	}
	for _, body := range []string{`{"version":"2"}`, `{"version":2,"category_id":null}`, `{"version":"9223372036854775808","category_id":null}`, `{"version":"02","category_id":null}`, `{"version":"2","category_id":null,"category_id":null}`, `{"Version":"2","category_id":null}`, `{"version":"2","category_id":""}`, `{"version":"2","category_id":null,"actor_id":"admin"}`} {
		if w := financialRequest(h, "PATCH", financialRoot+"/transactions/"+forecastID+"/category", body); w.Code != 422 {
			t.Fatal(body, w.Code, w.Body.String())
		}
	}
	f.transaction.AccountId = companyID
	if w := financialRequest(h, "GET", financialRoot+"/transactions", ""); w.Code != 503 {
		t.Fatal("foreign account", w.Code)
	}
	f.transaction = nil
	if w := financialRequest(h, "GET", financialRoot+"/transactions", ""); w.Code != 503 {
		t.Fatal("nil resource", w.Code)
	}
	f.category.Direction = corev1.Direction(99)
	if w := financialRequest(h, "GET", financialRoot+"/categories", ""); w.Code != 503 {
		t.Fatal("unknown enum", w.Code)
	}
}

type commitFixture struct {
	*importsReadFixture
	request *corev1.CommitImportRequest
	failure error
}

func (f *commitFixture) CommitImport(_ context.Context, r *corev1.CommitImportRequest, _ ...grpc.CallOption) (*corev1.CommitImportResponse, error) {
	f.request = proto.Clone(r).(*corev1.CommitImportRequest)
	return &corev1.CommitImportResponse{Import: f.item}, f.failure
}
func TestCommitImportStrictVersionsAckAndReplayEnvelope(t *testing.T) {
	ledger := newLedgerFixture()
	item := queuedImport()
	item.Status = corev1.ImportStatus_IMPORT_STATUS_APPLIED
	item.AppliedDataVersion = proto.Int64(2)
	item.Preview = &corev1.ImportPreview{AccountId: accountID, SourceCompanyId: "C001", SourceSystem: "dataset_v1", Currency: "RUB", PeriodStart: "2026-09-01", PeriodEnd: "2026-09-30", BaseDataVersion: 1, PreviewVersion: 2, RecordKinds: item.RecordKinds}
	f := &commitFixture{importsReadFixture: &importsReadFixture{item: item}}
	h := ledgerFixtureHandler(ledger, f)
	path := financialRoot + "/imports/" + forecastID + "/commit"
	for range 2 {
		w := financialRequest(h, "POST", path, `{"version":"2","expected_data_version":"1","acknowledge_warnings":true}`)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"applied"`) {
			t.Fatal(w.Code, w.Body.String())
		}
		if f.request.ExpectedVersion != 2 || f.request.ExpectedDataVersion != 1 || !f.request.AcknowledgeWarnings || f.request.IdempotencyKey != "financial-key-123" || f.request.Context.Browser.CookieToken != "opaque-session" || f.request.Scope.AccountId != accountID {
			t.Fatal(f.request)
		}
	}
	for _, body := range []string{`{"version":"2","expected_data_version":"1"}`, `{"version":"2","expected_data_version":"1","acknowledge_warnings":null}`, `{"version":"2","expected_data_version":"1","acknowledge_warnings":"true"}`, `{"version":"2","expected_data_version":"9223372036854775808","acknowledge_warnings":true}`, `{"version":"2","expected_data_version":"1","acknowledge_warnings":true,"scope":"other"}`} {
		if w := financialRequest(h, "POST", path, body); w.Code != 422 {
			t.Fatal(body, w.Code, w.Body.String())
		}
	}
	st, err := status.New(codes.Aborted, "private").WithDetails(&corev1.CoreErrorDetail{Reason: corev1.CoreErrorReason_CORE_ERROR_REASON_VERSION_CONFLICT})
	if err != nil {
		t.Fatal(err)
	}
	f.failure = st.Err()
	if w := financialRequest(h, "POST", path, `{"version":"2","expected_data_version":"1","acknowledge_warnings":false}`); w.Code != 409 || strings.Contains(w.Body.String(), "private") {
		t.Fatal(w.Code, w.Body.String())
	}
}
