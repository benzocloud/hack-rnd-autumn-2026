package ledger

import (
	"context"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"

	companyDomain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/company"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	authpb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/auth/v1"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/rpcerror"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
)

type accounts interface {
	ReadAccount(context.Context, domain.Scope, domain.Date) (ledger.ReadAccountResult, error)
	RecordManualBalance(context.Context, ledger.ManualBalanceCommand) (ledger.RecordManualBalanceResult, error)
	CreateManualTransaction(context.Context, ledger.ManualTransactionCommand) (ledger.CreateManualTransactionResult, error)
}
type companies interface {
	ReadProfile(context.Context, uuid.UUID) (companyDomain.Profile, error)
}
type authorizer interface {
	AuthorizeCompany(context.Context, *authpb.AuthorizeCompanyRequest) (*authpb.AuthorizeCompanyResponse, error)
}
type handler struct {
	pb.UnimplementedLedgerServiceServer
	accounts  accounts
	companies companies
	auth      authorizer
	location  *time.Location
	now       func() time.Time
}

func Register(server grpc.ServiceRegistrar, accounts accounts, companies companies, auth authorizer) error {
	location, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		return ledger.ErrUnavailable
	}
	pb.RegisterLedgerServiceServer(server, &handler{accounts: accounts, companies: companies, auth: auth, location: location, now: time.Now})
	return nil
}

func (h *handler) GetDefaultAccount(ctx context.Context, request *pb.GetDefaultAccountRequest) (*pb.GetDefaultAccountResponse, error) {
	if request == nil {
		return nil, rpcerror.From(ledger.ErrInvalid)
	}
	id, err := uuid.Parse(request.CompanyId)
	if err != nil || id == uuid.Nil || id.String() != request.CompanyId {
		return nil, rpcerror.From(ledger.ErrInvalid)
	}
	_, err = h.auth.AuthorizeCompany(ctx, &authpb.AuthorizeCompanyRequest{CompanyId: request.CompanyId, Permission: authpb.CompanyPermission_COMPANY_PERMISSION_FINANCIAL_READ,
		Context: &authpb.AuthorizeCompanyRequest_Browser{Browser: request.Context}})
	if err != nil {
		return nil, rpcerror.From(err)
	}
	profile, err := h.companies.ReadProfile(ctx, id)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	date := domain.Date(h.now().In(h.location).Format(time.DateOnly))
	result, err := h.accounts.ReadAccount(ctx, domain.Scope{CompanyID: id, AccountID: profile.AccountID}, date)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	return &pb.GetDefaultAccountResponse{Account: encodeAccount(result.Account, result.Balance)}, nil
}
