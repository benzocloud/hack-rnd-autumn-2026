package auth

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	authv1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/auth/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/securetls"
)

// Dial owns the connection returned to the composing application. No mutation retries.
func Dial(address, ca, cert, key string, normal, slow time.Duration) (authv1.AuthServiceClient, *grpc.ClientConn, error) {
	tlsConfig, err := securetls.Load(ca, cert, key, "spiffe://saldo.local/auth", false)
	if err != nil {
		return nil, nil, err
	}
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)), grpc.WithDisableRetry(), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(2<<20), grpc.MaxCallSendMsgSize(1<<20)), grpc.WithUnaryInterceptor(deadlines(normal, slow)))
	if err != nil {
		return nil, nil, err
	}
	return authv1.NewAuthServiceClient(conn), conn, nil
}

func deadlines(normal, slow time.Duration) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		timeout := normal
		switch method {
		case authv1.AuthService_Register_FullMethodName, authv1.AuthService_Login_FullMethodName, authv1.AuthService_ResetPassword_FullMethodName:
			timeout = slow
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
