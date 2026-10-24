package company

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/company"
	authpb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/auth/v1"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/company"
)

func (h *handler) ReadCompany(ctx context.Context, request *pb.ReadCompanyRequest) (*pb.ReadCompanyResponse, error) {
	if request == nil {
		return nil, mapError(company.ErrInvalid)
	}
	id, err := parseID(request.CompanyId)
	if err != nil {
		return nil, mapError(err)
	}
	grant, err := h.auth.AuthorizeCompany(ctx, &authpb.AuthorizeCompanyRequest{CompanyId: request.CompanyId,
		Permission: authpb.CompanyPermission_COMPANY_PERMISSION_READ, Context: &authpb.AuthorizeCompanyRequest_Browser{Browser: request.Context}})
	if err != nil {
		return nil, mapError(err)
	}
	profile, err := h.companies.ReadProfile(ctx, id)
	if err != nil {
		return nil, mapError(err)
	}
	return &pb.ReadCompanyResponse{Company: encodeProfile(profile), Role: grant.Role}, nil
}

func (h *handler) UpdateCompany(ctx context.Context, request *pb.UpdateCompanyRequest) (*pb.UpdateCompanyResponse, error) {
	if request == nil {
		return nil, mapError(company.ErrInvalid)
	}
	id, err := parseID(request.CompanyId)
	if err != nil {
		return nil, mapError(err)
	}
	grant, err := h.auth.AuthorizeCompany(ctx, &authpb.AuthorizeCompanyRequest{CompanyId: request.CompanyId,
		Permission: authpb.CompanyPermission_COMPANY_PERMISSION_UPDATE, Context: &authpb.AuthorizeCompanyRequest_Write{Write: request.Context}})
	if err != nil {
		return nil, mapError(err)
	}
	cmd := company.UpdateCommand{CompanyID: id, ExpectedVersion: request.ExpectedVersion, Name: request.Name}
	switch change := request.InnChange.(type) {
	case *pb.UpdateCompanyRequest_Inn:
		cmd.INN = &change.Inn
	case *pb.UpdateCompanyRequest_ClearInn:
		if change.ClearInn == nil {
			return nil, mapError(company.ErrInvalid)
		}
		cmd.ClearINN = true
	case nil:
	default:
		return nil, mapError(company.ErrInvalid)
	}
	profile, err := h.companies.Update(ctx, cmd)
	if err != nil {
		return nil, mapError(err)
	}
	return &pb.UpdateCompanyResponse{Company: encodeProfile(profile), Role: grant.Role}, nil
}

func encodeProfile(profile domain.Profile) *pb.Company {
	return &pb.Company{Id: profile.ID.String(), Name: profile.Spec.Name, Inn: encodeSpec(profile.Spec).Inn, Currency: profile.Spec.Currency,
		Timezone: profile.Spec.Timezone, DefaultAccountId: profile.AccountID.String(), Version: profile.Version, CreatedAt: timestamppb.New(profile.CreatedAt)}
}
