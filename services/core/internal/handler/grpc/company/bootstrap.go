package company

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/company"
	authpb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/auth/v1"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/rpcerror"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/company"
)

func (h *handler) CreateCompanyBootstrap(ctx context.Context, request *pb.CreateCompanyBootstrapRequest) (*pb.CreateCompanyBootstrapResponse, error) {
	cmd, err := decodeBootstrap(request)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	result, err := h.companies.Bootstrap(ctx, cmd)
	if err != nil {
		if errors.Is(err, company.ErrConflict) {
			return nil, rpcerror.New(codes.AlreadyExists, pb.CoreErrorReason_CORE_ERROR_REASON_IDEMPOTENCY_CONFLICT)
		}
		return nil, rpcerror.From(err)
	}
	return &pb.CreateCompanyBootstrapResponse{Receipt: encodeReceipt(result)}, nil
}

func (h *handler) GetCompanyCreationResult(ctx context.Context, request *pb.GetCompanyCreationResultRequest) (*pb.GetCompanyCreationResultResponse, error) {
	if request == nil {
		return nil, rpcerror.From(company.ErrInvalid)
	}
	id, err := parseID(request.CommandId)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	result, err := h.companies.GetBootstrapResult(ctx, id)
	if err != nil {
		return nil, rpcerror.From(err)
	}
	return &pb.GetCompanyCreationResultResponse{Receipt: encodeReceipt(result)}, nil
}

func decodeBootstrap(request *pb.CreateCompanyBootstrapRequest) (company.BootstrapCommand, error) {
	if request == nil || request.CompanySpec == nil || (request.CompanySpec.Inn != nil && *request.CompanySpec.Inn == "") {
		return company.BootstrapCommand{}, company.ErrInvalid
	}
	commandID, err := parseID(request.CommandId)
	if err != nil {
		return company.BootstrapCommand{}, err
	}
	companyID, err := parseID(request.CompanyId)
	if err != nil || companyID.Version() != 7 {
		return company.BootstrapCommand{}, company.ErrInvalid
	}
	s := request.CompanySpec
	return company.BootstrapCommand{CommandID: commandID, CompanyID: companyID, RequestSHA256: request.RequestSha256,
		Spec: domain.Spec{Name: s.Name, INN: s.GetInn(), Currency: s.Currency, Timezone: s.Timezone}}, nil
}

func parseID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil || id == uuid.Nil || id.String() != value {
		return uuid.Nil, company.ErrInvalid
	}
	return id, nil
}

func encodeSpec(spec domain.Spec) *authpb.CompanyCreateSpec {
	result := &authpb.CompanyCreateSpec{Name: spec.Name, Currency: spec.Currency, Timezone: spec.Timezone}
	if spec.INN != "" {
		result.Inn = &spec.INN
	}
	return result
}

func encodeReceipt(result company.BootstrapResult) *pb.CompanyCreationReceipt {
	return &pb.CompanyCreationReceipt{CommandId: result.CommandID.String(), CompanyId: result.CompanyID.String(), RequestSha256: result.RequestSHA256,
		CompanySpec: encodeSpec(result.Spec), DefaultAccountId: result.AccountID.String(), CommittedAt: timestamppb.New(result.CommittedAt)}
}
