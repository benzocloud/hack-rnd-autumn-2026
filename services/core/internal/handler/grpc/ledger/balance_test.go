package ledger

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	companyDomain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/company"
	authpb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/auth/v1"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
)

func TestRecordManualBalanceUsesOriginalWriteAndServerActor(t *testing.T) {
	companyID, accountID := uuid.New(), uuid.New()
	companies := &fakeCompanies{profile: companyDomain.Profile{ID: companyID, AccountID: accountID}}
	accounts, auth := &fakeAccounts{}, &fakeAuth{}
	location, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatal(err)
	}
	h := &handler{companies: companies, accounts: accounts, auth: auth, location: location, now: func() time.Time { return time.Date(2026, time.October, 23, 22, 0, 0, 0, time.UTC) }}
	request := &pb.RecordManualBalanceRequest{Scope: &pb.AccountScope{CompanyId: companyID.String(), AccountId: accountID.String()}, IdempotencyKey: "balance-key", ExpectedDataVersion: 1,
		Balance: &pb.Money{AmountMinor: 0, Currency: "RUB"}, AsOf: "2026-10-23", Note: "EOD", Context: &authpb.WriteContext{Browser: &authpb.BrowserContext{CookieToken: "opaque"}, CsrfToken: "csrf", Client: &authpb.ClientContext{RequestId: "trace"}}}
	_, err = h.RecordManualBalance(t.Context(), request)
	if err != nil || accounts.command.ActorID != "auth-user" || accounts.command.ViewDate != "2026-10-24" || accounts.command.Amount.Minor() != 0 || accounts.command.IdempotencyKey != request.IdempotencyKey {
		t.Fatal(accounts.command, err)
	}
	if auth.request.Permission != authpb.CompanyPermission_COMPANY_PERMISSION_FINANCIAL_WRITE || auth.request.GetWrite() != request.Context {
		t.Fatal("write context was reconstructed or permission changed")
	}
	accounts.reads, companies.reads = 0, 0
	s, _ := status.New(codes.PermissionDenied, "private").WithDetails(&authpb.AuthErrorDetail{Reason: authpb.AuthErrorReason_AUTH_ERROR_REASON_CSRF_INVALID})
	auth.err = s.Err()
	if _, err := h.RecordManualBalance(t.Context(), request); status.Code(err) != codes.PermissionDenied || accounts.reads != 0 || companies.reads != 0 {
		t.Fatal("Auth denial reached storage", err)
	}
	auth.err = nil
	request.Scope.AccountId = uuid.NewString()
	if _, err := h.RecordManualBalance(t.Context(), request); status.Code(err) != codes.NotFound || accounts.reads != 0 {
		t.Fatal("foreign default account accepted", err)
	}
	request.Balance = nil
	if _, err := h.RecordManualBalance(t.Context(), request); status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
}
