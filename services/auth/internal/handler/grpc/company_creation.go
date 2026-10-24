package grpc

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/domain/account"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/gen/auth/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/service/account"
)

func (s *server) BeginCompanyCreation(ctx context.Context, r *pb.BeginCompanyCreationRequest) (*pb.BeginCompanyCreationResponse, error) {
	spec := r.GetCompany()
	if spec == nil || (spec.Inn != nil && *spec.Inn == "") {
		return nil, rpcError(account.ErrValidation)
	}
	command, err := s.accounts.BeginCompanyCreation(ctx, account.BeginCompanyCreationInput{Context: writeContext(r.GetContext()), Key: r.GetIdempotencyKey(), Spec: account.CompanySpec{Name: spec.Name, INN: spec.GetInn(), Currency: spec.Currency, Timezone: spec.Timezone}})
	if err != nil {
		return nil, rpcError(err)
	}
	return &pb.BeginCompanyCreationResponse{Command: wireCreation(command)}, nil
}

func (s *server) AuthorizeCompanyBootstrap(ctx context.Context, r *pb.AuthorizeCompanyBootstrapRequest) (*pb.AuthorizeCompanyBootstrapResponse, error) {
	id, err := creationID(r.GetCommandId(), 4)
	if err != nil {
		return nil, rpcError(err)
	}
	companyID, err := creationID(r.GetCompanyId(), 7)
	if err != nil {
		return nil, rpcError(err)
	}
	command, err := s.accounts.AuthorizeCompanyBootstrap(ctx, account.AuthorizeCompanyBootstrapInput{CommandID: id, CompanyID: companyID, RequestSHA256: r.GetRequestSha256()})
	if err != nil {
		return nil, rpcError(err)
	}
	return &pb.AuthorizeCompanyBootstrapResponse{Command: wireCreation(command)}, nil
}

func (s *server) CompleteCompanyCreation(ctx context.Context, r *pb.CompleteCompanyCreationRequest) (*pb.CompleteCompanyCreationResponse, error) {
	id, err := creationID(r.GetCommandId(), 4)
	if err != nil {
		return nil, rpcError(err)
	}
	command, err := s.accounts.CompleteCompanyCreation(ctx, account.CompleteCompanyCreationInput{Context: writeContext(r.GetContext()), CommandID: id})
	if err != nil {
		return nil, rpcError(err)
	}
	return &pb.CompleteCompanyCreationResponse{Command: wireCreation(command)}, nil
}

func creationID(value string, version uuid.Version) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil || id == uuid.Nil || id.Version() != version || id.String() != value {
		return uuid.Nil, account.ErrValidation
	}
	return id, nil
}

func wireCreation(command account.CreationCommand) *pb.CompanyCreationCommand {
	spec := &pb.CompanyCreateSpec{Name: command.Spec.Name, Currency: command.Spec.Currency, Timezone: command.Spec.Timezone}
	if command.Spec.INN != "" {
		spec.Inn = &command.Spec.INN
	}
	state := pb.CompanyCreationState_COMPANY_CREATION_STATE_UNSPECIFIED
	switch command.State {
	case domain.CreationPrepared:
		state = pb.CompanyCreationState_COMPANY_CREATION_STATE_PREPARED
	case domain.CreationCompleted:
		state = pb.CompanyCreationState_COMPANY_CREATION_STATE_COMPLETED
	case domain.CreationQuarantined:
		state = pb.CompanyCreationState_COMPANY_CREATION_STATE_QUARANTINED
	}
	return &pb.CompanyCreationCommand{Id: command.ID.String(), CompanyId: command.CompanyID.String(), InitiatorId: command.InitiatorID.String(), Company: spec, RequestSha256: command.RequestSHA256, State: state, CreatedAt: timestamppb.New(command.CreatedAt)}
}
