// Package app wires owned resources. Stage 3.2 serves health probes only.
package app

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/config"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/postgres"
)

type database interface {
	Ping(context.Context) error
	CheckSchema(context.Context) error
	Close()
}

func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	return run(ctx, cfg, logger, func(ctx context.Context) (database, error) { return postgres.Open(ctx, cfg) }, net.Listen)
}

func run(ctx context.Context, cfg config.Config, logger *slog.Logger, open func(context.Context) (database, error), listen func(string, string) (net.Listener, error)) error {
	startup, cancelStartup := context.WithTimeout(ctx, cfg.StartupTimeout)
	db, err := open(startup)
	if err != nil {
		cancelStartup()
		return errors.New("auth startup: database unavailable")
	}
	defer db.Close()
	if err := db.CheckSchema(startup); err != nil {
		cancelStartup()
		return errors.New("auth startup: schema mismatch; run migrations")
	}
	cancelStartup()
	if ctx.Err() != nil {
		return nil
	}
	listener, err := listen("tcp", cfg.HTTPAddress)
	if err != nil {
		return errors.New("auth startup: health listener unavailable")
	}
	defer func() { _ = listener.Close() }()
	var ready atomic.Bool
	serverContext, cancelServer := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelServer()
	server := &http.Server{Handler: probes(db, &ready, cfg.QueryTimeout), ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout: 3 * time.Second, WriteTimeout: cfg.QueryTimeout + 2*time.Second, IdleTimeout: 30 * time.Second,
		MaxHeaderBytes: 8 << 10, BaseContext: func(net.Listener) context.Context { return serverContext },
		ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelError)}
	served := make(chan error, 1)
	ready.Store(true)
	go func() { served <- server.Serve(listener) }()
	logger.Info("auth health listener started", "address", listener.Addr().String())
	select {
	case err := <-served:
		ready.Store(false)
		cancelServer()
		_ = server.Close()
		if !errors.Is(err, http.ErrServerClosed) {
			return errors.New("auth health server stopped unexpectedly")
		}
		return nil
	case <-ctx.Done():
		ready.Store(false)
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			cancelServer()
			_ = server.Close()
			<-served
			return errors.New("auth shutdown deadline exceeded")
		}
		cancelServer()
		<-served
		return nil
	}
}

func probes(db database, ready *atomic.Bool, timeout time.Duration) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		if !ready.Load() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		if err := db.Ping(ctx); err != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		if err := db.CheckSchema(ctx); err != nil || !ready.Load() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ready\n"))
	})
	return mux
}
