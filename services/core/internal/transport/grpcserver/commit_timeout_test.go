package grpcserver

import (
	"context"
	"testing"
	"time"

	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/mtls"
)

type commitBudgetProbe struct {
	pb.UnimplementedImportsServiceServer
	observed chan time.Duration
}

func (p *commitBudgetProbe) CommitImport(ctx context.Context, _ *pb.CommitImportRequest) (*pb.CommitImportResponse, error) {
	deadline, _ := ctx.Deadline()
	p.observed <- time.Until(deadline)
	return &pb.CommitImportResponse{}, nil
}
func (p *commitBudgetProbe) GetImport(ctx context.Context, _ *pb.GetImportRequest) (*pb.GetImportResponse, error) {
	deadline, _ := ctx.Deadline()
	p.observed <- time.Until(deadline)
	return &pb.GetImportResponse{}, nil
}
func TestCommitServerBudgetPreservesReadAndCallerDeadlines(t *testing.T) {
	probe := &commitBudgetProbe{observed: make(chan time.Duration, 1)}
	address, ca := startTransportServer(t, probe)
	client := pb.NewImportsServiceClient(transportClient(t, address, ca, mtls.API))
	if _, err := client.CommitImport(t.Context(), &pb.CommitImportRequest{}); err != nil {
		t.Fatal(err)
	}
	if remaining := <-probe.observed; remaining < 90*time.Second || remaining > 100*time.Second {
		t.Fatal("server commit budget does not cover SQL work", remaining)
	}
	if _, err := client.GetImport(t.Context(), &pb.GetImportRequest{}); err != nil {
		t.Fatal(err)
	}
	if remaining := <-probe.observed; remaining <= 0 || remaining > 10*time.Second {
		t.Fatal("ordinary read deadline expanded", remaining)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	if _, err := client.CommitImport(ctx, &pb.CommitImportRequest{}); err != nil {
		t.Fatal(err)
	}
	if remaining := <-probe.observed; remaining <= 0 || remaining > 500*time.Millisecond {
		t.Fatal("server extended caller deadline", remaining)
	}
}
