package grpcserver

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/mtls"
)

// New owns service-peer authentication and bounded request budgets. Features
// register only their own handlers. Reflection is deliberately not enabled.
func New(tlsCredentials credentials.TransportCredentials) (*grpc.Server, error) {
	if tlsCredentials == nil || tlsCredentials.Info().SecurityProtocol != "tls" {
		return nil, mtls.ErrConfiguration
	}
	return grpc.NewServer(grpc.Creds(tlsCredentials), grpc.MaxRecvMsgSize(1<<20), grpc.MaxSendMsgSize(2<<20), grpc.UnaryInterceptor(authorizePeer)), nil
}

func authorizePeer(ctx context.Context, request any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
	identity := mtls.Identity(ctx)
	allowed := false
	switch info.FullMethod {
	case pb.CompanyService_CreateCompanyBootstrap_FullMethodName:
		allowed = identity == mtls.API || identity == mtls.Auth
	case pb.CompanyService_GetCompanyCreationResult_FullMethodName:
		allowed = identity == mtls.Auth
	case pb.CompanyService_ReadCompany_FullMethodName, pb.CompanyService_UpdateCompany_FullMethodName, pb.LedgerService_GetDefaultAccount_FullMethodName, pb.LedgerService_RecordManualBalance_FullMethodName, pb.LedgerService_CreateManualTransaction_FullMethodName:
		allowed = identity == mtls.API
	}
	if !allowed {
		return nil, status.Error(codes.PermissionDenied, "service peer is not permitted")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return next(ctx, request)
}
