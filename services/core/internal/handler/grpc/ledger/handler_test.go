package ledger

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	companyDomain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/company"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	authpb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/auth/v1"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
)

type fakeCompanies struct {
	profile companyDomain.Profile
	reads   int
}

func (c *fakeCompanies) ReadProfile(context.Context, uuid.UUID) (companyDomain.Profile, error) {
	c.reads++
	return c.profile, nil
}

type fakeAccounts struct {
	scope              domain.Scope
	date               domain.Date
	reads              int
	command            ledger.ManualBalanceCommand
	transactionCommand ledger.ManualTransactionCommand
}

func (a *fakeAccounts) CreateManualTransaction(_ context.Context, cmd ledger.ManualTransactionCommand) (ledger.CreateManualTransactionResult, error) {
	a.reads++
	a.transactionCommand = cmd
	data := cmd.Record
	data.ID = uuid.MustParse("019a0c65-9800-7000-8000-000000000021")
	record, err := domain.NewTransaction(data)
	return ledger.CreateManualTransactionResult{Transaction: record, CreatedAt: time.Now().UTC(), DataVersion: 2}, err
}

func (a *fakeAccounts) RecordManualBalance(_ context.Context, cmd ledger.ManualBalanceCommand) (ledger.RecordManualBalanceResult, error) {
	a.reads++
	a.command = cmd
	return ledger.RecordManualBalanceResult{Account: domain.AccountState{Scope: cmd.Scope, DataVersion: 2}}, nil
}

func (a *fakeAccounts) ReadAccount(_ context.Context, scope domain.Scope, date domain.Date) (ledger.ReadAccountResult, error) {
	a.reads++
	a.scope, a.date = scope, date
	return ledger.ReadAccountResult{Account: domain.AccountState{Scope: scope, DataVersion: 1}}, nil
}

type fakeAuth struct {
	err     error
	request *authpb.AuthorizeCompanyRequest
}

func (a *fakeAuth) AuthorizeCompany(_ context.Context, r *authpb.AuthorizeCompanyRequest) (*authpb.AuthorizeCompanyResponse, error) {
	a.request = r
	return &authpb.AuthorizeCompanyResponse{ActorId: "auth-user", Role: authpb.CompanyRole_COMPANY_ROLE_FINANCIER}, a.err
}

func TestGetDefaultAccountChecksAuthAndUsesCompanyAccount(t *testing.T) {
	companyID, accountID := uuid.New(), uuid.New()
	companies := &fakeCompanies{profile: companyDomain.Profile{ID: companyID, AccountID: accountID}}
	accounts, auth := &fakeAccounts{}, &fakeAuth{}
	location, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatal(err)
	}
	h := &handler{companies: companies, accounts: accounts, auth: auth, location: location, now: func() time.Time { return time.Date(2026, time.October, 23, 22, 0, 0, 0, time.UTC) }}
	request := &pb.GetDefaultAccountRequest{CompanyId: companyID.String(), Context: &authpb.BrowserContext{CookieToken: "opaque"}}
	result, err := h.GetDefaultAccount(t.Context(), request)
	if err != nil || result.Account.Balance != nil || result.Account.TransactionSource != nil || result.Account.DataVersion != 1 || accounts.scope.AccountID != accountID || accounts.date != "2026-10-24" {
		t.Fatal(result, err, accounts)
	}
	if auth.request.Permission != authpb.CompanyPermission_COMPANY_PERMISSION_FINANCIAL_READ || auth.request.GetBrowser() != request.Context {
		t.Fatal("lost financial permission or original browser context")
	}
	companies.reads, accounts.reads = 0, 0
	auth.err = errors.New("private Auth failure")
	if _, err := h.GetDefaultAccount(t.Context(), request); err == nil || companies.reads != 0 || accounts.reads != 0 {
		t.Fatal("Auth failure reached financial storage", err)
	}
	request.CompanyId = "not-a-uuid"
	if _, err := h.GetDefaultAccount(t.Context(), request); status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
}
