package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/config"
)

type fakeDB struct {
	closed    atomic.Bool
	schemaErr error
	pingErr   atomic.Bool
}

func (f *fakeDB) Close()                            { f.closed.Store(true) }
func (f *fakeDB) CheckSchema(context.Context) error { return f.schemaErr }
func (f *fakeDB) Ping(context.Context) error {
	if f.pingErr.Load() {
		return errors.New("TOP_SECRET")
	}
	return nil
}

func testConfig(t *testing.T) config.Config {
	t.Helper()
	env := map[string]string{"AUTH_ENV": "local", "AUTH_DATABASE_URL": "postgres://user@127.0.0.1/auth?sslmode=disable", "AUTH_HTTP_ADDRESS": "127.0.0.1:0"}
	cfg, err := config.Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestStartupFailureClosesDatabaseBeforeListening(t *testing.T) {
	for _, schemaFailure := range []bool{true, false} {
		t.Run(map[bool]string{true: "schema", false: "listen"}[schemaFailure], func(t *testing.T) {
			db := &fakeDB{}
			if schemaFailure {
				db.schemaErr = errors.New("TOP_SECRET")
			}
			listened := false
			err := run(t.Context(), testConfig(t), slog.New(slog.NewTextHandler(io.Discard, nil)),
				func(context.Context) (database, error) { return db, nil }, func(string, string) (net.Listener, error) { listened = true; return nil, errors.New("TOP_SECRET") })
			if err == nil || !db.closed.Load() {
				t.Fatal("failed startup did not release database")
			}
			if schemaFailure && listened {
				t.Fatal("server listened before schema became ready")
			}
		})
	}
}

func TestReadinessFailsClosedAndLivenessStaysAvailable(t *testing.T) {
	db := &fakeDB{}
	var ready atomic.Bool
	handler := probes(db, &ready, 50*time.Millisecond)
	check := func(path string, status int) {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != status {
			t.Fatalf("%s: status %d", path, w.Code)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("probe can be cached")
		}
	}
	check("/health/ready", 503)
	ready.Store(true)
	check("/health/ready", 200)
	db.pingErr.Store(true)
	check("/health/ready", 503)
	check("/health/live", 200)
	ready.Store(false)
	db.pingErr.Store(false)
	check("/health/ready", 503)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("POST", "/auth/login", nil))
	if w.Code != 404 {
		t.Fatal("unexpected auth operation exposed")
	}
}

func TestRunCancellationClosesListenerAndPool(t *testing.T) {
	db := &fakeDB{}
	bound := make(chan net.Listener, 1)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, testConfig(t), slog.New(slog.NewTextHandler(io.Discard, nil)), func(context.Context) (database, error) { return db, nil }, func(network, address string) (net.Listener, error) {
			listener, err := net.Listen(network, address)
			if err == nil {
				bound <- listener
			}
			return listener, err
		})
	}()
	var listener net.Listener
	select {
	case listener = <-bound:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not bind")
	}
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + listener.Addr().String() + "/health/ready")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("server not ready after startup")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown hung")
	}
	if !db.closed.Load() {
		t.Fatal("database leaked")
	}
	connection, err := net.DialTimeout("tcp", listener.Addr().String(), 100*time.Millisecond)
	if err == nil {
		_ = connection.Close()
		t.Fatal("listener leaked")
	}
}

type blockingDB struct {
	fakeDB
	entered  chan struct{}
	finished chan struct{}
}

func (db *blockingDB) Ping(ctx context.Context) error {
	close(db.entered)
	<-ctx.Done()
	close(db.finished)
	return ctx.Err()
}

func TestShutdownDeadlineCancelsInflightProbe(t *testing.T) {
	cfg := testConfig(t)
	cfg.ShutdownTimeout = 50 * time.Millisecond
	db := &blockingDB{entered: make(chan struct{}), finished: make(chan struct{})}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	bound := make(chan net.Listener, 1)
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), func(context.Context) (database, error) { return db, nil }, func(network, address string) (net.Listener, error) {
			listener, err := net.Listen(network, address)
			if err == nil {
				bound <- listener
			}
			return listener, err
		})
	}()
	var listener net.Listener
	select {
	case listener = <-bound:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not bind")
	}
	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		client := &http.Client{Timeout: 2 * time.Second}
		response, err := client.Get("http://" + listener.Addr().String() + "/health/ready")
		if err == nil {
			_ = response.Body.Close()
		}
	}()
	select {
	case <-db.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("probe did not enter database")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("forced shutdown was not reported")
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown deadline did not bound exit")
	}
	select {
	case <-db.finished:
	case <-time.After(time.Second):
		t.Fatal("in-flight probe was not cancelled")
	}
	select {
	case <-clientDone:
	case <-time.After(time.Second):
		t.Fatal("client remained blocked")
	}
	if !db.closed.Load() {
		t.Fatal("pool was not closed after forced shutdown")
	}
}
