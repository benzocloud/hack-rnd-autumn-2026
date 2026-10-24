package core

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/securetls"
)

// Dial returns a connection owned by app. Writes are never automatically retried.
func Dial(address, serverName, ca, cert, key string, timeout time.Duration) (*grpc.ClientConn, error) {
	if address == "" || timeout <= 0 {
		return nil, errors.New("invalid Core client configuration")
	}
	tlsConfig, err := securetls.Load(ca, cert, key, "spiffe://saldo.local/core", false)
	if err != nil {
		return nil, err
	}
	tlsConfig.ServerName = serverName
	return grpc.NewClient(address, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)), grpc.WithDisableRetry(),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(2<<20), grpc.MaxCallSendMsgSize(1<<20)),
		grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoke grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			return invoke(ctx, method, req, reply, cc, opts...)
		}))
}
