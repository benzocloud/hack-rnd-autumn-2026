package ledger

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	companyDomain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/company"
	authpb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/auth/v1"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
)

func TestManualTransactionRPCMappingAndWriteContext(t *testing.T) {
	companyID, accountID := uuid.New(), uuid.New()
	companies := &fakeCompanies{profile: companyDomain.Profile{ID: companyID, AccountID: accountID}}
	accounts, auth := &fakeAccounts{}, &fakeAuth{}
	location, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatal(err)
	}
	h := &handler{companies: companies, accounts: accounts, auth: auth, location: location, now: func() time.Time { return time.Date(2026, 10, 23, 22, 0, 0, 0, time.UTC) }}
	r := &pb.CreateManualTransactionRequest{Scope: &pb.AccountScope{CompanyId: companyID.String(), AccountId: accountID.String()}, Context: &authpb.WriteContext{Browser: &authpb.BrowserContext{CookieToken: "opaque"}, CsrfToken: "csrf"},
		IdempotencyKey: "manual-create-key", ExpectedDataVersion: 1, OperationDate: "2026-10-23", Direction: pb.Direction_DIRECTION_OUTFLOW, Amount: &pb.Money{AmountMinor: 1, Currency: "RUB"}}
	result, err := h.CreateManualTransaction(t.Context(), r)
	if err != nil || result.Transaction.Direction != pb.Direction_DIRECTION_OUTFLOW || result.Transaction.CounterpartyInn != nil || result.Transaction.CategoryId != nil || accounts.transactionCommand.ActorID != "auth-user" || accounts.transactionCommand.ViewDate != "2026-10-24" || auth.request.GetWrite() != r.Context || auth.request.Permission != authpb.CompanyPermission_COMPANY_PERMISSION_FINANCIAL_WRITE {
		t.Fatal(result, err)
	}
	auth.err = status.Error(codes.PermissionDenied, "private denial")
	accounts.reads, companies.reads = 0, 0
	if _, err := h.CreateManualTransaction(t.Context(), r); err == nil || accounts.reads != 0 || companies.reads != 0 {
		t.Fatal("denial reached storage", err)
	}
	auth.err = nil
	r.Scope.AccountId = uuid.NewString()
	if _, err := h.CreateManualTransaction(t.Context(), r); status.Code(err) != codes.NotFound || accounts.reads != 0 {
		t.Fatal(err)
	}
	for _, test := range []*pb.CreateManualTransactionRequest{nil, {}, {Scope: r.Scope}, proto.Clone(r).(*pb.CreateManualTransactionRequest)} {
		if test != nil && test.Amount != nil {
			test.Direction = pb.Direction(99)
		}
		if _, err := decodeManualTransaction(test); err == nil {
			t.Fatal("bad request accepted", test)
		}
	}
}
