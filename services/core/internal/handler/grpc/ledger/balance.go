package ledger

import (
	"context"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	authpb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/auth/v1"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/rpcerror"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
)

func (h *handler) RecordManualBalance(ctx context.Context, request *pb.RecordManualBalanceRequest) (*pb.RecordManualBalanceResponse, error) {
	cmd, err := decodeManualBalance(request)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	grant, err := h.auth.AuthorizeCompany(ctx, &authpb.AuthorizeCompanyRequest{CompanyId: request.Scope.CompanyId,
		Permission: authpb.CompanyPermission_COMPANY_PERMISSION_FINANCIAL_WRITE, Context: &authpb.AuthorizeCompanyRequest_Write{Write: request.Context}})
	if err != nil {
		return nil, rpcerror.From(err)
	}
	profile, err := h.companies.ReadProfile(ctx, cmd.Scope.CompanyID)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	if profile.AccountID != cmd.Scope.AccountID {
		return nil, rpcerror.From(ledger.ErrNotFound)
	}
	cmd.ActorID = grant.ActorId
	cmd.ViewDate = domain.Date(h.now().In(h.location).Format(time.DateOnly))
	result, err := h.accounts.RecordManualBalance(ctx, cmd)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	return &pb.RecordManualBalanceResponse{Account: encodeAccount(result.Account, result.Balance)}, nil
}

func decodeManualBalance(request *pb.RecordManualBalanceRequest) (ledger.ManualBalanceCommand, error) {
	if request == nil || request.Scope == nil || request.Balance == nil {
		return ledger.ManualBalanceCommand{}, ledger.ErrInvalid
	}
	companyID, companyErr := uuid.Parse(request.Scope.CompanyId)
	accountID, accountErr := uuid.Parse(request.Scope.AccountId)
	if companyErr != nil || accountErr != nil || companyID == uuid.Nil || accountID == uuid.Nil || companyID.String() != request.Scope.CompanyId || accountID.String() != request.Scope.AccountId {
		return ledger.ManualBalanceCommand{}, ledger.ErrInvalid
	}
	amount, err := money.New(request.Balance.AmountMinor, request.Balance.Currency)
	if err != nil {
		return ledger.ManualBalanceCommand{}, ledger.ErrInvalid
	}
	return ledger.ManualBalanceCommand{Scope: domain.Scope{CompanyID: companyID, AccountID: accountID}, Date: domain.Date(request.AsOf), Amount: amount, Note: request.Note,
		ExpectedDataVersion: request.ExpectedDataVersion, IdempotencyKey: request.IdempotencyKey}, nil
}
