package grpc

import (
	"context"
	"strconv"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/gen/auth/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/service/account"
)

func (s *server) ListMembers(ctx context.Context, r *pb.ListMembersRequest) (*pb.ListMembersResponse, error) {
	companyID, err := companyID(r.GetCompanyId())
	if err != nil {
		return nil, rpcError(err)
	}
	limit, cursor, err := pageArgs(r.GetPage())
	if err != nil {
		return nil, rpcError(err)
	}
	out, err := s.accounts.ListMembers(ctx, account.ListMembersInput{CookieToken: r.GetContext().GetCookieToken(), CompanyID: companyID, Limit: limit, Cursor: cursor})
	if err != nil {
		return nil, rpcError(err)
	}
	items := make([]*pb.Member, 0, len(out.Items))
	for _, m := range out.Items {
		items = append(items, &pb.Member{
			User:     &pb.UserProfile{Id: m.UserID.String(), Email: m.Email, DisplayName: m.DisplayName, EmailVerified: true},
			Role:     wireRole(m.Role),
			JoinedAt: timestamppb.New(m.JoinedAt),
		})
	}
	return &pb.ListMembersResponse{Items: items, Pagination: &pb.PageResponse{NextCursor: out.NextCursor}}, nil
}

func (s *server) RemoveMember(ctx context.Context, r *pb.RemoveMemberRequest) (*pb.RemoveMemberResponse, error) {
	companyID, err := companyID(r.GetCompanyId())
	if err != nil {
		return nil, rpcError(err)
	}
	userID, err := uuid.Parse(r.GetUserId())
	if err != nil || userID == uuid.Nil || userID.String() != r.GetUserId() {
		return nil, rpcError(account.ErrValidation)
	}
	if err := s.accounts.RemoveMember(ctx, account.RemoveMemberInput{Context: writeContext(r.GetContext()), CompanyID: companyID, UserID: userID}); err != nil {
		return nil, rpcError(err)
	}
	return &pb.RemoveMemberResponse{}, nil
}

func (s *server) CreateInvitation(ctx context.Context, r *pb.CreateInvitationRequest) (*pb.CreateInvitationResponse, error) {
	companyID, err := companyID(r.GetCompanyId())
	if err != nil {
		return nil, rpcError(err)
	}
	out, err := s.accounts.CreateInvitation(ctx, account.CreateInvitationInput{
		Context: writeContext(r.GetContext()), CompanyID: companyID, Email: r.GetEmail(),
		CompanyNameSnapshot: r.GetCompanyNameSnapshot(), IdempotencyKey: r.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, rpcError(err)
	}
	return &pb.CreateInvitationResponse{Invitation: wireInvitation(out)}, nil
}

func (s *server) ListOutgoingInvitations(ctx context.Context, r *pb.ListOutgoingInvitationsRequest) (*pb.ListOutgoingInvitationsResponse, error) {
	companyID, err := companyID(r.GetCompanyId())
	if err != nil {
		return nil, rpcError(err)
	}
	limit, cursor, err := pageArgs(r.GetPage())
	if err != nil {
		return nil, rpcError(err)
	}
	out, err := s.accounts.ListOutgoingInvitations(ctx, account.ListOutgoingInvitationsInput{CookieToken: r.GetContext().GetCookieToken(), CompanyID: companyID, Limit: limit, Cursor: cursor})
	if err != nil {
		return nil, rpcError(err)
	}
	return &pb.ListOutgoingInvitationsResponse{Items: wireInvitations(out.Items), Pagination: &pb.PageResponse{NextCursor: out.NextCursor}}, nil
}

func (s *server) ListIncomingInvitations(ctx context.Context, r *pb.ListIncomingInvitationsRequest) (*pb.ListIncomingInvitationsResponse, error) {
	limit, cursor, err := pageArgs(r.GetPage())
	if err != nil {
		return nil, rpcError(err)
	}
	out, err := s.accounts.ListIncomingInvitations(ctx, account.ListIncomingInvitationsInput{CookieToken: r.GetContext().GetCookieToken(), Limit: limit, Cursor: cursor})
	if err != nil {
		return nil, rpcError(err)
	}
	return &pb.ListIncomingInvitationsResponse{Items: wireInvitations(out.Items), Pagination: &pb.PageResponse{NextCursor: out.NextCursor}}, nil
}

func (s *server) AcceptInvitation(ctx context.Context, r *pb.AcceptInvitationRequest) (*pb.AcceptInvitationResponse, error) {
	id, err := invitationID(r.GetInvitationId())
	if err != nil {
		return nil, rpcError(err)
	}
	companyID, err := s.accounts.AcceptInvitation(ctx, account.AcceptInvitationInput{Context: writeContext(r.GetContext()), InvitationID: id, Version: r.GetVersion(), IdempotencyKey: r.GetIdempotencyKey()})
	if err != nil {
		return nil, rpcError(err)
	}
	return &pb.AcceptInvitationResponse{CompanyId: companyID.String()}, nil
}

func (s *server) DeclineInvitation(ctx context.Context, r *pb.DeclineInvitationRequest) (*pb.DeclineInvitationResponse, error) {
	id, err := invitationID(r.GetInvitationId())
	if err != nil {
		return nil, rpcError(err)
	}
	out, err := s.accounts.DeclineInvitation(ctx, account.DeclineInvitationInput{Context: writeContext(r.GetContext()), InvitationID: id, Version: r.GetVersion()})
	if err != nil {
		return nil, rpcError(err)
	}
	return &pb.DeclineInvitationResponse{Invitation: wireInvitation(out)}, nil
}

func (s *server) RevokeInvitation(ctx context.Context, r *pb.RevokeInvitationRequest) (*pb.RevokeInvitationResponse, error) {
	companyID, err := companyID(r.GetCompanyId())
	if err != nil {
		return nil, rpcError(err)
	}
	id, err := invitationID(r.GetInvitationId())
	if err != nil {
		return nil, rpcError(err)
	}
	if err := s.accounts.RevokeInvitation(ctx, account.RevokeInvitationInput{Context: writeContext(r.GetContext()), CompanyID: companyID, InvitationID: id}); err != nil {
		return nil, rpcError(err)
	}
	return &pb.RevokeInvitationResponse{}, nil
}

func companyID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil || id == uuid.Nil || id.Version() != 7 || id.String() != value {
		return uuid.Nil, account.ErrValidation
	}
	return id, nil
}

func invitationID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil || id == uuid.Nil || id.Version() != 4 || id.String() != value {
		return uuid.Nil, account.ErrValidation
	}
	return id, nil
}

func pageArgs(page *pb.PageRequest) (int, string, error) {
	limit := 50
	cursor := ""
	if page != nil {
		cursor = page.GetCursor()
		if page.Limit != nil {
			limit = int(page.GetLimit())
			if limit < 1 || limit > 100 {
				return 0, "", account.ErrValidation
			}
		}
	}
	return limit, cursor, nil
}

func wireInvitation(in account.Invitation) *pb.Invitation {
	status := pb.InvitationStatus_INVITATION_STATUS_UNSPECIFIED
	switch in.Status {
	case "pending":
		status = pb.InvitationStatus_INVITATION_STATUS_PENDING
	case "accepted":
		status = pb.InvitationStatus_INVITATION_STATUS_ACCEPTED
	case "declined":
		status = pb.InvitationStatus_INVITATION_STATUS_DECLINED
	case "revoked":
		status = pb.InvitationStatus_INVITATION_STATUS_REVOKED
	case "expired":
		status = pb.InvitationStatus_INVITATION_STATUS_EXPIRED
	}
	return &pb.Invitation{
		Id: in.ID.String(), CompanyId: in.CompanyID.String(), CompanyNameSnapshot: in.CompanyNameSnapshot,
		Email: in.Email, Role: wireRole(in.Role), Status: status, CreatedAt: timestamppb.New(in.CreatedAt),
		ExpiresAt: timestamppb.New(in.ExpiresAt), Version: strconv.FormatInt(in.Version, 10),
	}
}

func wireInvitations(items []account.Invitation) []*pb.Invitation {
	out := make([]*pb.Invitation, 0, len(items))
	for _, item := range items {
		out = append(out, wireInvitation(item))
	}
	return out
}
