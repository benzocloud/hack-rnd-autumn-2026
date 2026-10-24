package app

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	clientauth "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/client/auth"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/config"
	authv1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/auth/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/securetls"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/testauth"
)

func TestRunFailures(t *testing.T) {
	p, err := testauth.GeneratePKI(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base := config.Config{Address: "127.0.0.1:0", Origin: "https://localhost", AuthAddress: "127.0.0.1:1", CA: p.CA, Cert: p.APICert, Key: p.APIKey, RPCDeadline: time.Second, SlowDeadline: time.Second, ShutdownTimeout: time.Second}
	log := slog.New(slog.DiscardHandler)
	if Run(t.Context(), config.Config{}, log) == nil {
		t.Fatal("bad config")
	}
	c := base
	c.CA = "missing"
	if Run(t.Context(), c, log) == nil {
		t.Fatal("missing CA")
	}
	c = base
	c.HTTPCert = "missing"
	c.HTTPKey = "missing"
	if Run(t.Context(), c, log) == nil {
		t.Fatal("missing HTTP TLS")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	c = base
	c.Address = listener.Addr().String()
	if Run(t.Context(), c, log) == nil {
		t.Fatal("occupied listener")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	if err := Run(ctx, base, log); err != nil {
		t.Fatal(err)
	}
	c = base
	c.HTTPCert = p.HTTPCert
	c.HTTPKey = p.HTTPKey
	secureCtx, secureCancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer secureCancel()
	if err := Run(secureCtx, c, log); err != nil {
		t.Fatal(err)
	}
}

func TestServeLifecycleReadinessAndForcedShutdown(t *testing.T) {
	p, err := testauth.GeneratePKI(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tlsConfig, err := securetls.Load(p.CA, p.AuthCert, p.AuthKey, "spiffe://saldo.local/api", true)
	if err != nil {
		t.Fatal(err)
	}
	rpcListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer rpcListener.Close()
	srv := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConfig)))
	defer srv.Stop()
	fixture := testauth.New()
	authv1.RegisterAuthServiceServer(srv, fixture)
	go func() { _ = srv.Serve(rpcListener) }()
	client, conn, err := clientauth.Dial(rpcListener.Addr().String(), p.CA, p.APICert, p.APIKey, 2*time.Second, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for _, force := range []bool{false, true} {
		t.Run(map[bool]string{false: "graceful", true: "forced"}[force], func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			cfg := config.Config{Origin: "https://localhost", ShutdownTimeout: 2 * time.Second}
			if force {
				cfg.ShutdownTimeout = 100 * time.Millisecond
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- Serve(ctx, cfg, slog.New(slog.DiscardHandler), client, listener) }()
			httpClient := &http.Client{Timeout: 3 * time.Second}
			t.Cleanup(httpClient.CloseIdleConnections)
			get := func(path string) int {
				r, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+listener.Addr().String()+path, nil)
				if err != nil {
					t.Fatal(err)
				}
				res, err := httpClient.Do(r)
				if err != nil {
					return 0
				}
				defer res.Body.Close()
				if _, err := io.Copy(io.Discard, res.Body); err != nil {
					return 0
				}
				return res.StatusCode
			}
			if got := get("/health/ready"); got != 200 {
				t.Fatal("readiness", got)
			}
			fixture.SetDelay("GetPersonalDataConsent", 2*time.Second)
			if get("/health/ready") != 503 {
				t.Fatal("dependency readiness")
			}
			fixture.SetDelay("GetPersonalDataConsent", 0)
			if force {
				fixture.SetDelay("GetSecurityContext", 2*time.Second)
				active := make(chan int, 1)
				go func() { active <- get("/api/v1/auth/csrf") }()
				deadline := time.Now().Add(time.Second)
				for len(fixture.Requests("GetSecurityContext")) == 0 && time.Now().Before(deadline) {
					time.Sleep(time.Millisecond)
				}
				cancel()
				if err := <-done; err == nil {
					t.Fatal("forced timeout unreported")
				}
				<-active
				fixture.SetDelay("GetSecurityContext", 0)
			} else {
				cancel()
				if err := <-done; err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	if err := Serve(t.Context(), config.Config{}, slog.New(slog.DiscardHandler), client, closed); err == nil {
		t.Fatal("unexpected serve failure hidden")
	}
}
