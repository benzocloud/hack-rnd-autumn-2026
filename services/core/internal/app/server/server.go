package server

import (
	"context"
	"errors"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/app"
	authClient "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/client/auth"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
	companyHandler "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/company"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/mtls"
)

var errRuntime = errors.New("core runtime dependency unavailable")

// Run owns the Auth connection, application pool, listener and graceful shutdown.
// Migration is an explicit operator step and never runs as a startup side effect.
func Run(ctx context.Context, cfg config.Config, rpc config.RPC) (err error) {
	if err := rpc.Validate(); err != nil {
		return err
	}
	files := mtls.Files{CA: rpc.CAFile, Certificate: rpc.CertificateFile, Key: rpc.KeyFile}
	serverTLS, err := mtls.Server(files)
	if err != nil {
		return err
	}
	clientTLS, err := mtls.Client(files, rpc.AuthServerName, mtls.Core, mtls.Auth)
	if err != nil {
		return err
	}
	conn, err := grpc.NewClient(rpc.AuthAddress, grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)))
	if err != nil {
		return errRuntime
	}
	defer func() { err = errors.Join(err, conn.Close()) }()
	auth, err := authClient.New(conn, rpc.AuthTimeout)
	if err != nil {
		return errRuntime
	}
	application, err := app.NewWithCompanyAuthorizer(ctx, cfg, auth.AuthorizeBootstrap)
	if err != nil {
		return errRuntime
	}
	defer func() { err = errors.Join(err, application.Close()) }()
	server, err := companyHandler.NewServer(credentials.NewTLS(serverTLS), application.Companies, auth)
	if err != nil {
		return err
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", rpc.Address)
	if err != nil {
		server.Stop()
		return errRuntime
	}
	return serve(ctx, server, listener, cfg.ShutdownTimeout)
}

func serve(ctx context.Context, server *grpc.Server, listener net.Listener, timeout time.Duration) error {
	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	select {
	case err := <-result:
		server.Stop()
		if errors.Is(err, grpc.ErrServerStopped) && ctx.Err() != nil {
			return nil
		}
		return err
	case <-ctx.Done():
	}
	stopped := make(chan struct{})
	go func() { server.GracefulStop(); close(stopped) }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-stopped:
	case <-timer.C:
		server.Stop()
		<-stopped
	}
	err := <-result
	if errors.Is(err, grpc.ErrServerStopped) {
		return nil
	}
	return err
}
