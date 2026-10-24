//go:build integration

package integration_test

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/app"
)

func TestRealDatabaseReadinessAndServiceRestart(t *testing.T) {
	cfg, db := scratch(t)
	if err := app.Migrate(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: time.Second}
	defer client.CloseIdleConnections()
	for range 2 {
		var listenConfig net.ListenConfig
		listener, err := listenConfig.Listen(t.Context(), "tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		cfg.HTTPAddress = listener.Addr().String()
		if err := listener.Close(); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- app.Run(ctx, cfg, slog.New(slog.DiscardHandler)) }()
		base := "http://" + cfg.HTTPAddress
		deadline := time.Now().Add(5 * time.Second)
		ready := false
		for time.Now().Before(deadline) {
			response, err := getResponse(t.Context(), client, base+"/health/ready")
			if err == nil {
				_ = response.Body.Close()
				if response.StatusCode == http.StatusOK {
					ready = true
					break
				}
			}
			select {
			case err := <-done:
				cancel()
				t.Fatalf("server exited before ready: %v", err)
			default:
			}
			time.Sleep(10 * time.Millisecond)
		}
		if !ready {
			stopService(t, cancel, done)
			t.Fatal("real service never became ready")
		}
		// Liveness survives schema incompatibility; readiness must fail closed.
		if _, err := db.Exec(t.Context(), "UPDATE goose_db_version SET version_id=99 WHERE version_id=3"); err != nil {
			stopService(t, cancel, done)
			t.Fatal(err)
		}
		for _, tc := range []struct {
			path   string
			status int
		}{{"/health/ready", http.StatusServiceUnavailable}, {"/health/live", http.StatusOK}, {"/api/v1/auth/register", http.StatusNotFound}} {
			response, err := getResponse(t.Context(), client, base+tc.path)
			if err != nil {
				stopService(t, cancel, done)
				t.Fatal(err)
			}
			_ = response.Body.Close()
			if response.StatusCode != tc.status {
				stopService(t, cancel, done)
				t.Fatalf("%s: %d", tc.path, response.StatusCode)
			}
		}
		if _, err := db.Exec(t.Context(), "UPDATE goose_db_version SET version_id=3 WHERE version_id=99"); err != nil {
			stopService(t, cancel, done)
			t.Fatal(err)
		}
		stopService(t, cancel, done)
		dialer := net.Dialer{Timeout: 100 * time.Millisecond}
		connection, err := dialer.DialContext(t.Context(), "tcp", cfg.HTTPAddress)
		if err == nil {
			_ = connection.Close()
			t.Fatal("real listener leaked after shutdown")
		}
	}
}

func getResponse(ctx context.Context, client *http.Client, url string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return client.Do(request)
}

func stopService(t *testing.T, cancel context.CancelFunc, done <-chan error) {
	t.Helper()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("real server shutdown hung")
	}
}
