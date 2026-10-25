package grpcserver

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/url"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/mtls"
)

func TestPaymentMethodsAllowOnlyVerifiedAPIPeer(t *testing.T) {
	t.Parallel()
	methods := []string{
		pb.PaymentsService_UpdatePayment_FullMethodName, pb.PaymentsService_CancelPayment_FullMethodName, pb.PaymentsService_RemovePaymentMatch_FullMethodName,
		pb.PaymentsService_ListPayments_FullMethodName, pb.PaymentsService_GetPayment_FullMethodName, pb.PaymentsService_CreateManualPayment_FullMethodName,
		pb.PaymentsService_ListPaymentMatches_FullMethodName, pb.PaymentsService_MatchPayment_FullMethodName, pb.PaymentsService_CreateReconciliationGroup_FullMethodName,
		pb.PaymentsService_GetReconciliationGroup_FullMethodName, pb.PaymentsService_ListReconciliationGroups_FullMethodName, pb.PaymentsService_RevokeReconciliationGroup_FullMethodName,
	}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			for _, identity := range []string{mtls.API, mtls.Auth, mtls.Core, mtls.ML, ""} {
				t.Run(identity, func(t *testing.T) {
					ctx := t.Context()
					if identity != "" {
						uri, err := url.Parse(identity)
						if err != nil {
							t.Fatal(err)
						}
						leaf := &x509.Certificate{URIs: []*url.URL{uri}}
						ctx = peer.NewContext(ctx, &peer.Peer{AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{leaf}}}}})
					}
					called := false
					_, err := authorizePeer(ctx, nil, &grpc.UnaryServerInfo{FullMethod: method}, func(ctx context.Context, _ any) (any, error) {
						called = true
						if _, ok := ctx.Deadline(); !ok {
							t.Fatal("missing request deadline")
						}
						return nil, nil
					})
					if identity == mtls.API {
						if err != nil || !called {
							t.Fatalf("API denied: %v", err)
						}
					} else if status.Code(err) != codes.PermissionDenied || called {
						t.Fatalf("foreign peer allowed: %v", err)
					}
				})
			}
		})
	}
}
