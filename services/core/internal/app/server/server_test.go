package server

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
)

func TestRunRejectsConfigurationBeforeOpeningResources(t *testing.T) {
	if err := Run(t.Context(), config.Config{}, config.RPC{}); err == nil {
		t.Fatal("server accepted missing mTLS configuration")
	}
}

func TestServeStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() { result <- serve(ctx, grpc.NewServer(), bufconn.Listen(1024), time.Second) }()
	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("gRPC shutdown leaked")
	}
}
