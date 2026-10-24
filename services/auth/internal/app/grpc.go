package app

import (
	"context"
	"errors"
	"net"
	"time"

	grpcgo "google.golang.org/grpc"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/config"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/gen/auth/v1"
	handler "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/handler/grpc"
)

func startRPC(ctx context.Context, db database, cfg config.Config) (func(), <-chan error, error) {
	if cfg.GRPCAddress == "" {
		return func() {}, nil, nil
	}
	app, ok := db.(*application)
	if !ok || app.Account == nil {
		return nil, nil, errors.New("auth RPC unavailable")
	}
	creds, err := handler.TLSCredentials(cfg.GRPCCertFile, cfg.GRPCKeyFile, cfg.GRPCCAFile)
	if err != nil {
		return nil, nil, err
	}
	lc := net.ListenConfig{}
	listener, err := lc.Listen(ctx, "tcp", cfg.GRPCAddress)
	if err != nil {
		return nil, nil, errors.New("auth RPC listener unavailable")
	}
	server := grpcgo.NewServer(grpcgo.Creds(creds), grpcgo.MaxRecvMsgSize(1<<20), grpcgo.MaxSendMsgSize(2<<20), grpcgo.MaxConcurrentStreams(64), grpcgo.UnaryInterceptor(handler.AuthenticatePeer))
	pb.RegisterAuthServiceServer(server, handler.New(app.Account))
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	stop := func() {
		done := make(chan struct{})
		go func() { server.GracefulStop(); close(done) }()
		timer := time.NewTimer(cfg.ShutdownTimeout)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
			server.Stop()
			<-done
		}
		_ = listener.Close()
	}
	return stop, served, nil
}
