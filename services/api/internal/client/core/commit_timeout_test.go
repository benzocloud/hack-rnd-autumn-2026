package core

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/securetls"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/testauth"
)

type commitBudgetProbe struct {
	corev1.UnimplementedImportsServiceServer
	observed chan time.Duration
}

func (p *commitBudgetProbe) CommitImport(ctx context.Context, _ *corev1.CommitImportRequest) (*corev1.CommitImportResponse, error) {
	deadline, _ := ctx.Deadline()
	p.observed <- time.Until(deadline)
	return &corev1.CommitImportResponse{}, nil
}

func (p *commitBudgetProbe) GetImport(ctx context.Context, _ *corev1.GetImportRequest) (*corev1.GetImportResponse, error) {
	deadline, _ := ctx.Deadline()
	p.observed <- time.Until(deadline)
	return &corev1.GetImportResponse{}, nil
}

func TestCoreClientCommitHasSeparateBoundedBudget(t *testing.T) {
	pki, err := testauth.GeneratePKI(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	identity, err := securetls.Load(pki.CA, pki.CoreCert, pki.CoreKey, "spiffe://saldo.local/api", true)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	probe := &commitBudgetProbe{observed: make(chan time.Duration, 1)}
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(identity)))
	corev1.RegisterImportsServiceServer(server, probe)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	conn, err := Dial(listener.Addr().String(), "localhost", pki.CA, pki.APICert, pki.APIKey, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client := corev1.NewImportsServiceClient(conn)
	if _, err := client.CommitImport(t.Context(), &corev1.CommitImportRequest{}); err != nil {
		t.Fatal(err)
	}
	if remaining := <-probe.observed; remaining < 90*time.Second || remaining > 2*time.Minute {
		t.Fatal("commit cannot use its SQL budget or is unbounded", remaining)
	}
	if _, err := client.GetImport(t.Context(), &corev1.GetImportRequest{}); err != nil {
		t.Fatal(err)
	}
	if remaining := <-probe.observed; remaining <= 0 || remaining > time.Second {
		t.Fatal("ordinary read deadline was expanded", remaining)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	if _, err := client.CommitImport(ctx, &corev1.CommitImportRequest{}); err != nil {
		t.Fatal(err)
	}
	if remaining := <-probe.observed; remaining <= 0 || remaining > 500*time.Millisecond {
		t.Fatal("caller deadline was extended", remaining)
	}
}
