package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/company"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/auth/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/company"
)

var errConfiguration = errors.New("invalid Auth client configuration")

type client struct {
	rpc     pb.AuthServiceClient
	timeout time.Duration
}

// New borrows the connection; app owns its verified mTLS policy and Close.
func New(conn *grpc.ClientConn, timeout time.Duration) (*client, error) {
	if conn == nil || timeout <= 0 || timeout > 3*time.Second {
		return nil, errConfiguration
	}
	return &client{rpc: pb.NewAuthServiceClient(conn), timeout: timeout}, nil
}

func (c *client) AuthorizeBootstrap(ctx context.Context, commandID, companyID uuid.UUID, hash string) (company.Authorization, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	response, err := c.rpc.AuthorizeCompanyBootstrap(ctx, &pb.AuthorizeCompanyBootstrapRequest{CommandId: commandID.String(), CompanyId: companyID.String(), RequestSha256: hash},
		grpc.MaxCallSendMsgSize(1<<20), grpc.MaxCallRecvMsgSize(2<<20))
	if err != nil {
		return company.Authorization{}, safeError(err)
	}
	if response == nil || response.Command == nil {
		return company.Authorization{}, company.ErrUnavailable
	}
	cmd := response.Command
	if cmd.Id != commandID.String() || cmd.CompanyId != companyID.String() || cmd.RequestSha256 != hash || strings.TrimSpace(cmd.InitiatorId) == "" ||
		cmd.CreatedAt == nil || cmd.CreatedAt.CheckValid() != nil ||
		(cmd.State != pb.CompanyCreationState_COMPANY_CREATION_STATE_PREPARED && cmd.State != pb.CompanyCreationState_COMPANY_CREATION_STATE_COMPLETED) {
		return company.Authorization{}, company.ErrUnauthorized
	}
	spec, err := decodeSpec(cmd.Company)
	if err != nil {
		return company.Authorization{}, company.ErrUnauthorized
	}
	expected, err := domain.Fingerprint(spec)
	if err != nil || expected != hash {
		return company.Authorization{}, company.ErrUnauthorized
	}
	return company.Authorization{CommandID: commandID, CompanyID: companyID, RequestSHA256: hash, Spec: spec}, nil
}

func (c *client) AuthorizeCompany(ctx context.Context, request *pb.AuthorizeCompanyRequest) (*pb.AuthorizeCompanyResponse, error) {
	if !validCompanyRequest(request) {
		return nil, company.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	response, err := c.rpc.AuthorizeCompany(ctx, request, grpc.MaxCallSendMsgSize(1<<20), grpc.MaxCallRecvMsgSize(2<<20))
	if err != nil {
		return nil, safeError(err)
	}
	if response == nil || strings.TrimSpace(response.ActorId) == "" ||
		(response.Role != pb.CompanyRole_COMPANY_ROLE_OWNER && response.Role != pb.CompanyRole_COMPANY_ROLE_FINANCIER) ||
		(request.Permission == pb.CompanyPermission_COMPANY_PERMISSION_UPDATE && response.Role != pb.CompanyRole_COMPANY_ROLE_OWNER) {
		return nil, company.ErrUnavailable
	}
	return response, nil
}

func validCompanyRequest(request *pb.AuthorizeCompanyRequest) bool {
	if request == nil {
		return false
	}
	id, err := uuid.Parse(request.CompanyId)
	if err != nil || id == uuid.Nil || id.String() != request.CompanyId {
		return false
	}
	if request.Permission == pb.CompanyPermission_COMPANY_PERMISSION_READ || request.Permission == pb.CompanyPermission_COMPANY_PERMISSION_FINANCIAL_READ {
		return request.GetBrowser() != nil
	}
	if request.Permission == pb.CompanyPermission_COMPANY_PERMISSION_UPDATE || request.Permission == pb.CompanyPermission_COMPANY_PERMISSION_FINANCIAL_WRITE {
		write := request.GetWrite()
		return write != nil && write.Browser != nil
	}
	return false
}

func decodeSpec(spec *pb.CompanyCreateSpec) (domain.Spec, error) {
	if spec == nil || (spec.Inn != nil && *spec.Inn == "") {
		return domain.Spec{}, company.ErrInvalid
	}
	result := domain.Spec{Name: spec.Name, INN: spec.GetInn(), Currency: spec.Currency, Timezone: spec.Timezone}
	return result, result.Validate()
}

type dependencyError struct {
	cause error
	safe  *status.Status
}

func (e *dependencyError) Error() string              { return e.safe.Err().Error() }
func (e *dependencyError) Unwrap() error              { return e.cause }
func (e *dependencyError) GRPCStatus() *status.Status { return e.safe }

func safeError(err error) error {
	code := status.Code(err)
	if code == codes.Canceled || code == codes.DeadlineExceeded {
		return &dependencyError{cause: err, safe: status.New(code, "Auth request interrupted")}
	}
	// Only a typed business error can become a user denial. Peer/configuration
	// failures without AuthErrorDetail remain a dependency failure.
	for _, raw := range status.Convert(err).Details() {
		detail, ok := raw.(*pb.AuthErrorDetail)
		if !ok || !allowedReason(code, detail.Reason) {
			continue
		}
		safe, detailErr := status.New(code, "Auth authorization rejected").WithDetails(&pb.AuthErrorDetail{Reason: detail.Reason, RetryAfterSeconds: detail.RetryAfterSeconds})
		if detailErr == nil {
			return &dependencyError{cause: err, safe: safe}
		}
	}
	return &dependencyError{cause: err, safe: status.New(codes.Unavailable, "Auth dependency unavailable")}
}

func allowedReason(code codes.Code, reason pb.AuthErrorReason) bool {
	return (code == codes.Unauthenticated && reason == pb.AuthErrorReason_AUTH_ERROR_REASON_AUTH_REQUIRED) ||
		(code == codes.PermissionDenied && (reason == pb.AuthErrorReason_AUTH_ERROR_REASON_FORBIDDEN || reason == pb.AuthErrorReason_AUTH_ERROR_REASON_CSRF_INVALID || reason == pb.AuthErrorReason_AUTH_ERROR_REASON_EMAIL_UNVERIFIED)) ||
		(code == codes.NotFound && reason == pb.AuthErrorReason_AUTH_ERROR_REASON_NOT_FOUND) ||
		(code == codes.ResourceExhausted && reason == pb.AuthErrorReason_AUTH_ERROR_REASON_RATE_LIMITED) ||
		(code == codes.Aborted && reason == pb.AuthErrorReason_AUTH_ERROR_REASON_MEMBERSHIP_REVOKED)
}
