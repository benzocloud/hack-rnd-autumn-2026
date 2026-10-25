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
	return grpc.NewServer(grpc.Creds(tlsCredentials), grpc.MaxRecvMsgSize(2<<20), grpc.MaxSendMsgSize(2<<20), grpc.UnaryInterceptor(authorizePeer), grpc.StreamInterceptor(authorizeStreamPeer)), nil
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
	case pb.PaymentsService_UpdatePayment_FullMethodName,
		pb.PaymentsService_CancelPayment_FullMethodName,
		pb.PaymentsService_RemovePaymentMatch_FullMethodName,
		pb.PaymentsService_ListPayments_FullMethodName,
		pb.PaymentsService_GetPayment_FullMethodName,
		pb.PaymentsService_CreateManualPayment_FullMethodName,
		pb.PaymentsService_ListPaymentMatches_FullMethodName,
		pb.PaymentsService_MatchPayment_FullMethodName,
		pb.PaymentsService_CreateReconciliationGroup_FullMethodName,
		pb.PaymentsService_GetReconciliationGroup_FullMethodName,
		pb.PaymentsService_ListReconciliationGroups_FullMethodName,
		pb.PaymentsService_RevokeReconciliationGroup_FullMethodName:
		allowed = identity == mtls.API
	case pb.ForecastingService_CreateForecast_FullMethodName, pb.ForecastingService_GetForecast_FullMethodName, pb.ForecastingService_ListForecasts_FullMethodName:
		allowed = identity == mtls.API
	case pb.ImportsService_ListImports_FullMethodName, pb.ImportsService_GetImport_FullMethodName, pb.ImportsService_ListImportIssues_FullMethodName:
		allowed = identity == mtls.API
	case pb.ImportsService_CommitImport_FullMethodName, pb.LedgerService_ListTransactions_FullMethodName, pb.LedgerService_ListCategories_FullMethodName, pb.LedgerService_UpdateTransactionCategory_FullMethodName:
		allowed = identity == mtls.API
	}
	if !allowed {
		return nil, status.Error(codes.PermissionDenied, "service peer is not permitted")
	}
	deadline := 10 * time.Second
	if info.FullMethod == pb.ImportsService_CommitImport_FullMethodName {
		deadline = 100 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	return next(ctx, request)
}

type boundedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s boundedStream) Context() context.Context { return s.ctx }

func (s boundedStream) RecvMsg(message any) error {
	// Returning from the handler cancels the transport receive. The buffered
	// result lets that one in-flight receive finish after our deadline fires.
	result := make(chan error, 1)
	go func() { result <- s.ServerStream.RecvMsg(message) }()
	select {
	case err := <-result:
		return err
	case <-s.ctx.Done():
		return status.FromContextError(s.ctx.Err()).Err()
	}
}

func authorizeStreamPeer(service any, stream grpc.ServerStream, info *grpc.StreamServerInfo, next grpc.StreamHandler) error {
	if info.FullMethod != pb.ImportsService_UploadImport_FullMethodName || mtls.Identity(stream.Context()) != mtls.API {
		return status.Error(codes.PermissionDenied, "service peer is not permitted")
	}
	ctx, cancel := context.WithTimeout(stream.Context(), 2*time.Minute)
	defer cancel()
	return next(service, boundedStream{ServerStream: stream, ctx: ctx})
}
