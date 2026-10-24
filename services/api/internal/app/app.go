package app

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/client/auth"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/config"
	authv1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/auth/v1"
	httpapi "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/handler/http"
)

func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	client, conn, err := auth.Dial(cfg.AuthAddress, cfg.CA, cfg.Cert, cfg.Key, cfg.RPCDeadline, cfg.SlowDeadline)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	var cert tls.Certificate
	if cfg.HTTPCert != "" {
		cert, err = tls.LoadX509KeyPair(cfg.HTTPCert, cfg.HTTPKey)
		if err != nil {
			return errors.New("cannot load HTTP TLS identity")
		}
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.Address)
	if err != nil {
		return errors.New("cannot bind HTTP listener")
	}
	defer func() { _ = listener.Close() }()
	if cfg.HTTPCert != "" {
		listener = tls.NewListener(listener, &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}})
	}
	return Serve(ctx, cfg, logger, client, listener)
}

// Serve owns HTTP connections; the caller owns the dependency and listener.
func Serve(ctx context.Context, cfg config.Config, logger *slog.Logger, client authv1.AuthServiceClient, listener net.Listener) error {
	var stopping atomic.Bool
	ready := func(ctx context.Context) bool {
		if stopping.Load() {
			return false
		}
		probe, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		response, err := client.GetPersonalDataConsent(probe, &authv1.GetPersonalDataConsentRequest{})
		return err == nil && response.GetDocument().GetText() != "" && !stopping.Load()
	}
	serverCtx, cancelServer := context.WithCancel(context.Background())
	defer cancelServer()
	server := &http.Server{Handler: httpapi.New(client, cfg.Origin, logger, ready), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: max(cfg.RPCDeadline, cfg.SlowDeadline) + 20*time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10, BaseContext: func(net.Listener) context.Context { return serverCtx }}
	errorsCh := make(chan error, 1)
	go func() { errorsCh <- server.Serve(listener) }()
	logger.InfoContext(ctx, "API started", "address", listener.Addr().String())
	select {
	case err := <-errorsCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return errors.New("HTTP server stopped unexpectedly")
	case <-ctx.Done():
		stopping.Store(true)
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			cancelServer()
			_ = server.Close()
			if serveErr := <-errorsCh; serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
				return errors.New("HTTP server stopped during forced shutdown")
			}
			return errors.New("HTTP shutdown deadline exceeded")
		}
		if serveErr := <-errorsCh; serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return errors.New("HTTP server stopped during shutdown")
		}
		return nil
	}
}
