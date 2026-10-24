package ledger

import (
	"context"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	authpb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/auth/v1"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/rpcerror"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
)

func (h *handler) CreateManualTransaction(ctx context.Context, request *pb.CreateManualTransactionRequest) (*pb.CreateManualTransactionResponse, error) {
	cmd, err := decodeManualTransaction(request)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	grant, err := h.auth.AuthorizeCompany(ctx, &authpb.AuthorizeCompanyRequest{CompanyId: request.Scope.CompanyId,
		Permission: authpb.CompanyPermission_COMPANY_PERMISSION_FINANCIAL_WRITE, Context: &authpb.AuthorizeCompanyRequest_Write{Write: request.Context}})
	if err != nil {
		return nil, rpcerror.From(err)
	}
	profile, err := h.companies.ReadProfile(ctx, cmd.Record.Scope.CompanyID)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	if profile.AccountID != cmd.Record.Scope.AccountID {
		return nil, rpcerror.From(ledger.ErrNotFound)
	}
	cmd.ActorID, cmd.ViewDate = grant.ActorId, domain.Date(h.now().In(h.location).Format(time.DateOnly))
	result, err := h.accounts.CreateManualTransaction(ctx, cmd)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	return &pb.CreateManualTransactionResponse{Transaction: encodeManualTransaction(result), DataVersion: result.DataVersion}, nil
}

func decodeManualTransaction(request *pb.CreateManualTransactionRequest) (ledger.ManualTransactionCommand, error) {
	if request == nil || request.Scope == nil || request.Amount == nil {
		return ledger.ManualTransactionCommand{}, ledger.ErrInvalid
	}
	companyID, companyErr := uuid.Parse(request.Scope.CompanyId)
	accountID, accountErr := uuid.Parse(request.Scope.AccountId)
	if companyErr != nil || accountErr != nil || companyID == uuid.Nil || accountID == uuid.Nil || companyID.String() != request.Scope.CompanyId || accountID.String() != request.Scope.AccountId {
		return ledger.ManualTransactionCommand{}, ledger.ErrInvalid
	}
	amount, err := money.New(request.Amount.AmountMinor, request.Amount.Currency)
	if err != nil {
		return ledger.ManualTransactionCommand{}, ledger.ErrInvalid
	}
	var direction domain.Direction
	switch request.Direction {
	case pb.Direction_DIRECTION_INFLOW:
		direction = domain.Inflow
	case pb.Direction_DIRECTION_OUTFLOW:
		direction = domain.Outflow
	case pb.Direction_DIRECTION_UNSPECIFIED:
		return ledger.ManualTransactionCommand{}, ledger.ErrInvalid
	default:
		return ledger.ManualTransactionCommand{}, ledger.ErrInvalid
	}
	category := domain.Category{}
	if request.CategoryId != nil {
		id, err := uuid.Parse(*request.CategoryId)
		if err != nil || id == uuid.Nil || id.String() != *request.CategoryId {
			return ledger.ManualTransactionCommand{}, ledger.ErrInvalid
		}
		category = domain.Category{ID: id, CompanyID: companyID}
	}
	if request.CounterpartyInn != nil && *request.CounterpartyInn == "" {
		return ledger.ManualTransactionCommand{}, ledger.ErrInvalid
	}
	return ledger.ManualTransactionCommand{Record: domain.TransactionData{Scope: domain.Scope{CompanyID: companyID, AccountID: accountID}, Date: domain.Date(request.OperationDate), Direction: direction,
		Amount: amount, Source: domain.Source{Kind: domain.Manual}, Category: category, CounterpartyINN: request.GetCounterpartyInn(), CounterpartyName: request.CounterpartyName, Purpose: request.PaymentPurpose},
		ExpectedDataVersion: request.ExpectedDataVersion, IdempotencyKey: request.IdempotencyKey}, nil
}

func encodeManualTransaction(result ledger.CreateManualTransactionResult) *pb.Transaction {
	d := result.Transaction.Data()
	direction := pb.Direction_DIRECTION_INFLOW
	if d.Direction == domain.Outflow {
		direction = pb.Direction_DIRECTION_OUTFLOW
	}
	out := &pb.Transaction{Id: d.ID.String(), AccountId: d.Scope.AccountID.String(), OperationDate: string(d.Date), Direction: direction,
		Amount: &pb.Money{AmountMinor: d.Amount.Minor(), Currency: d.Amount.Currency()}, CounterpartyName: d.CounterpartyName, PaymentPurpose: d.Purpose,
		Source: &pb.RecordSource{Kind: pb.RecordSourceKind_RECORD_SOURCE_KIND_MANUAL, SourceSystem: "manual_v1"}, Version: result.Transaction.Version(), CreatedAt: timestamppb.New(result.CreatedAt)}
	if d.CounterpartyINN != "" {
		out.CounterpartyInn = &d.CounterpartyINN
	}
	if d.Category.ID != uuid.Nil {
		id := d.Category.ID.String()
		out.CategoryId = &id
	}
	return out
}
