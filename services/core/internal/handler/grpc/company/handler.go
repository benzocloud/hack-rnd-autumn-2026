package company

import (
	"context"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/company"
	authpb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/auth/v1"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/company"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/mtls"
)

type operations interface {
	Bootstrap(context.Context, company.BootstrapCommand) (company.BootstrapResult, error)
	GetBootstrapResult(context.Context, uuid.UUID) (company.BootstrapResult, error)
	ReadProfile(context.Context, uuid.UUID) (domain.Profile, error)
	Update(context.Context, company.UpdateCommand) (domain.Profile, error)
}

type authorizer interface {
	AuthorizeCompany(context.Context, *authpb.AuthorizeCompanyRequest) (*authpb.AuthorizeCompanyResponse, error)
}

type handler struct {
	pb.UnimplementedCompanyServiceServer
	companies operations
	auth      authorizer
}

// NewServer requires verified mutual TLS and registers only CompanyService.
// Health/reflection and unrelated, unimplemented financial RPCs are not exposed.
func NewServer(tlsCredentials credentials.TransportCredentials, companies operations, auth authorizer) (*grpc.Server, error) {
	if tlsCredentials == nil || tlsCredentials.Info().SecurityProtocol != "tls" || companies == nil || auth == nil {
		return nil, company.ErrUnavailable
	}
	server := grpc.NewServer(grpc.Creds(tlsCredentials), grpc.MaxRecvMsgSize(1<<20), grpc.MaxSendMsgSize(2<<20), grpc.UnaryInterceptor(authorizePeer))
	pb.RegisterCompanyServiceServer(server, &handler{companies: companies, auth: auth})
	return server, nil
}

func authorizePeer(ctx context.Context, request any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
	identity := mtls.Identity(ctx)
	allowed := false
	switch info.FullMethod {
	case pb.CompanyService_CreateCompanyBootstrap_FullMethodName:
		allowed = identity == mtls.API || identity == mtls.Auth
	case pb.CompanyService_GetCompanyCreationResult_FullMethodName:
		allowed = identity == mtls.Auth
	case pb.CompanyService_ReadCompany_FullMethodName, pb.CompanyService_UpdateCompany_FullMethodName:
		allowed = identity == mtls.API
	}
	if !allowed {
		return nil, status.Error(codes.PermissionDenied, "service peer is not permitted")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return next(ctx, request)
}
