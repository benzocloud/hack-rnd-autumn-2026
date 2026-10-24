package grpc

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/gen/auth/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/service/account"
)

type accountOperations interface {
	BeginCompanyCreation(context.Context, account.BeginCompanyCreationInput) (account.CreationCommand, error)
	AuthorizeCompanyBootstrap(context.Context, account.AuthorizeCompanyBootstrapInput) (account.CreationCommand, error)
	CompleteCompanyCreation(context.Context, account.CompleteCompanyCreationInput) (account.CreationCommand, error)
	GetPersonalDataConsent(context.Context) (account.Document, error)
	EmailVerificationMode() string
	GetSecurityContext(context.Context, string) (account.SecurityContext, error)
	Register(context.Context, account.RegisterInput) (account.Challenge, error)
	Login(context.Context, account.LoginInput) (account.LoginResult, error)
	VerifyEmail(context.Context, account.VerifyEmailInput) error
	ResendVerification(context.Context, account.WriteContext, string) (account.Challenge, error)
	Logout(context.Context, account.WriteContext) error
	GetCurrentAccount(context.Context, string) (account.User, error)
	AuthorizeCompany(context.Context, account.AuthorizeCompanyInput) (account.Authorization, error)
	ListMemberships(context.Context, string, int, string) (account.MembershipPage, error)
}

type server struct {
	pb.UnimplementedAuthServiceServer
	accounts accountOperations
}

func New(accounts accountOperations) *server { return &server{accounts: accounts} }

func (s *server) GetPersonalDataConsent(ctx context.Context, _ *pb.GetPersonalDataConsentRequest) (*pb.GetPersonalDataConsentResponse, error) {
	d, err := s.accounts.GetPersonalDataConsent(ctx)
	if err != nil {
		return nil, rpcError(err)
	}
	return &pb.GetPersonalDataConsentResponse{Document: &pb.ConsentDocument{Text: d.Text, DocumentVersion: d.Version, DocumentSha256: d.SHA256}}, nil
}
func (s *server) GetSecurityContext(ctx context.Context, r *pb.GetSecurityContextRequest) (*pb.GetSecurityContextResponse, error) {
	c, err := s.accounts.GetSecurityContext(ctx, r.GetContext().GetCookieToken())
	if err != nil {
		return nil, rpcError(err)
	}
	return &pb.GetSecurityContextResponse{Context: wireContext(c), EmailVerificationMode: s.accounts.EmailVerificationMode()}, nil
}
func (s *server) Register(ctx context.Context, r *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	c := r.GetPersonalDataConsent()
	out, err := s.accounts.Register(ctx, account.RegisterInput{Context: writeContext(r.GetContext()), Email: r.GetEmail(), Password: r.GetPassword(), DisplayName: r.GetDisplayName(), ConsentAccepted: c.GetAccepted(), ConsentVersion: c.GetDocumentVersion(), ConsentSHA256: c.GetDocumentSha256()})
	if err != nil {
		return nil, rpcError(err)
	}
	return &pb.RegisterResponse{Challenge: wireChallenge(out)}, nil
}
func (s *server) Login(ctx context.Context, r *pb.LoginRequest) (*pb.LoginResponse, error) {
	out, err := s.accounts.Login(ctx, account.LoginInput{Context: writeContext(r.GetContext()), Email: r.GetEmail(), Password: r.GetPassword()})
	if err != nil {
		return nil, rpcError(err)
	}
	return &pb.LoginResponse{User: wireUser(out.User), Context: wireContext(out.Context)}, nil
}
func (s *server) VerifyEmail(ctx context.Context, r *pb.VerifyEmailRequest) (*pb.VerifyEmailResponse, error) {
	id, err := uuid.Parse(r.GetChallengeId())
	if err != nil || id == uuid.Nil {
		return nil, rpcError(account.ErrValidation)
	}
	if err := s.accounts.VerifyEmail(ctx, account.VerifyEmailInput{Context: writeContext(r.GetContext()), ChallengeID: id, Code: r.GetCode()}); err != nil {
		return nil, rpcError(err)
	}
	return &pb.VerifyEmailResponse{}, nil
}
func (s *server) ResendVerification(ctx context.Context, r *pb.ResendVerificationRequest) (*pb.ResendVerificationResponse, error) {
	out, err := s.accounts.ResendVerification(ctx, writeContext(r.GetContext()), r.GetEmail())
	if err != nil {
		return nil, rpcError(err)
	}
	return &pb.ResendVerificationResponse{Challenge: wireChallenge(out)}, nil
}
func (s *server) Logout(ctx context.Context, r *pb.LogoutRequest) (*pb.LogoutResponse, error) {
	if err := s.accounts.Logout(ctx, writeContext(r.GetContext())); err != nil {
		return nil, rpcError(err)
	}
	return &pb.LogoutResponse{}, nil
}
func (s *server) GetCurrentAccount(ctx context.Context, r *pb.GetCurrentAccountRequest) (*pb.GetCurrentAccountResponse, error) {
	out, err := s.accounts.GetCurrentAccount(ctx, r.GetContext().GetCookieToken())
	if err != nil {
		return nil, rpcError(err)
	}
	return &pb.GetCurrentAccountResponse{User: wireUser(out)}, nil
}
func (s *server) AuthorizeCompany(ctx context.Context, r *pb.AuthorizeCompanyRequest) (*pb.AuthorizeCompanyResponse, error) {
	id, err := uuid.Parse(r.GetCompanyId())
	if err != nil || id == uuid.Nil {
		return nil, rpcError(account.ErrValidation)
	}
	permissions := map[pb.CompanyPermission]string{
		pb.CompanyPermission_COMPANY_PERMISSION_READ: "read", pb.CompanyPermission_COMPANY_PERMISSION_UPDATE: "update",
		pb.CompanyPermission_COMPANY_PERMISSION_FINANCIAL_READ: "financial_read", pb.CompanyPermission_COMPANY_PERMISSION_FINANCIAL_WRITE: "financial_write",
		pb.CompanyPermission_COMPANY_PERMISSION_MEMBERS_READ: "members_read", pb.CompanyPermission_COMPANY_PERMISSION_MEMBERS_MANAGE: "members_manage",
		pb.CompanyPermission_COMPANY_PERMISSION_INVITATIONS_MANAGE: "invitations_manage",
	}
	permission, ok := permissions[r.GetPermission()]
	if !ok {
		return nil, rpcError(account.ErrValidation)
	}
	input := account.AuthorizeCompanyInput{CompanyID: id, Permission: permission}
	switch c := r.GetContext().(type) {
	case *pb.AuthorizeCompanyRequest_Browser:
		input.CookieToken = c.Browser.GetCookieToken()
	case *pb.AuthorizeCompanyRequest_Write:
		input.CookieToken = c.Write.GetBrowser().GetCookieToken()
		input.CSRFToken = c.Write.GetCsrfToken()
		input.Write = true
	default:
		return nil, rpcError(account.ErrAuthRequired)
	}
	out, err := s.accounts.AuthorizeCompany(ctx, input)
	if err != nil {
		return nil, rpcError(err)
	}
	return &pb.AuthorizeCompanyResponse{ActorId: out.ActorID.String(), Role: wireRole(out.Role)}, nil
}
func (s *server) ListMemberships(ctx context.Context, r *pb.ListMembershipsRequest) (*pb.ListMembershipsResponse, error) {
	limit := 50
	if r.GetPage() != nil && r.GetPage().Limit != nil {
		limit = int(r.GetPage().GetLimit())
		if limit < 1 || limit > 100 {
			return nil, rpcError(account.ErrValidation)
		}
	}
	out, err := s.accounts.ListMemberships(ctx, r.GetContext().GetCookieToken(), limit, r.GetPage().GetCursor())
	if err != nil {
		return nil, rpcError(err)
	}
	items := make([]*pb.Membership, 0, len(out.Items))
	for _, m := range out.Items {
		items = append(items, &pb.Membership{CompanyId: m.CompanyID.String(), Role: wireRole(m.Role), CreatedAt: timestamppb.New(m.CreatedAt)})
	}
	return &pb.ListMembershipsResponse{Items: items, Pagination: &pb.PageResponse{NextCursor: out.NextCursor}}, nil
}
func writeContext(c *pb.WriteContext) account.WriteContext {
	return account.WriteContext{CookieToken: c.GetBrowser().GetCookieToken(), CSRFToken: c.GetCsrfToken()}
}
func wireContext(c account.SecurityContext) *pb.SecurityContext {
	return &pb.SecurityContext{CookieToken: c.CookieToken, CsrfToken: c.CSRFToken, CookieExpiresAt: timestamppb.New(c.ExpiresAt)}
}
func wireUser(u account.User) *pb.UserProfile {
	return &pb.UserProfile{Id: u.ID.String(), Email: u.Email, DisplayName: u.DisplayName, EmailVerified: u.EmailVerified}
}
func wireChallenge(c account.Challenge) *pb.EmailVerificationChallenge {
	return &pb.EmailVerificationChallenge{ChallengeId: c.ID.String(), ExpiresAt: timestamppb.New(c.ExpiresAt)}
}
func wireRole(role string) pb.CompanyRole {
	switch role {
	case "owner":
		return pb.CompanyRole_COMPANY_ROLE_OWNER
	case "financier":
		return pb.CompanyRole_COMPANY_ROLE_FINANCIER
	default:
		return pb.CompanyRole_COMPANY_ROLE_UNSPECIFIED
	}
}
