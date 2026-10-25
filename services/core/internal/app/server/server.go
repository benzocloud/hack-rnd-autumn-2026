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
	mlClient "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/client/ml"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
	companyHandler "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/company"
	forecastHandler "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/forecasting"
	importsHandler "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/imports"
	ledgerHandler "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/ledger"
	paymentsHandler "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/payments"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/grpcserver"
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
	var model interface {
		PredictFinancial(context.Context, forecasting.FinancialModelInput) (forecasting.FinancialModelResult, error)
	}
	if rpc.MLAddress != "" {
		mlTLS, err := mtls.Client(files, rpc.MLServerName, mtls.Core, mtls.ML)
		if err != nil {
			return err
		}
		mlConn, err := grpc.NewClient(rpc.MLAddress, grpc.WithTransportCredentials(credentials.NewTLS(mlTLS)), grpc.WithDisableRetry())
		if err != nil {
			return errRuntime
		}
		defer func() { err = errors.Join(err, mlConn.Close()) }()
		model, err = mlClient.New(mlConn, 25*time.Second)
		if err != nil {
			return errRuntime
		}
	}
	application, err := app.NewWithForecastModel(ctx, cfg, auth.AuthorizeBootstrap, model)
	if err != nil {
		return errRuntime
	}
	defer func() { err = errors.Join(err, application.Close()) }()
	server, err := grpcserver.New(credentials.NewTLS(serverTLS))
	if err != nil {
		return err
	}
	companyHandler.Register(server, application.Companies, auth)
	paymentsHandler.Register(server, application.Payments, application.Companies, auth)
	if err := ledgerHandler.Register(server, application.Ledger, application.Companies, auth); err != nil {
		server.Stop()
		return err
	}
	if err := forecastHandler.Register(server, application.ForecastJobs, application.Companies, auth, application.Scenarios); err != nil {
		server.Stop()
		return err
	}
	if err := importsHandler.Register(server, application.Imports, application.Companies, auth); err != nil {
		server.Stop()
		return err
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", rpc.Address)
	if err != nil {
		server.Stop()
		return errRuntime
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	worker := make(chan error, 2)
	workers := 1
	go func() { worker <- application.Imports.Run(runCtx); cancel() }()
	if model != nil {
		workers++
		go func() { worker <- application.ForecastJobs.Run(runCtx); cancel() }()
	}
	serveErr := serve(runCtx, server, listener, cfg.ShutdownTimeout)
	cancel()
	for range workers {
		serveErr = errors.Join(serveErr, <-worker)
	}
	return serveErr
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
