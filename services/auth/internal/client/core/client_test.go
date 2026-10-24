package core

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	authpb "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/gen/auth/v1"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/service/account"
)

type receiptServer struct {
	pb.UnimplementedCompanyServiceServer
	mu      sync.Mutex
	receipt *pb.CompanyCreationReceipt
	err     error
	wait    bool
	last    *pb.CreateCompanyBootstrapRequest
}

func (s *receiptServer) GetCompanyCreationResult(ctx context.Context, _ *pb.GetCompanyCreationResultRequest) (*pb.GetCompanyCreationResultResponse, error) {
	s.mu.Lock()
	wait, err := s.wait, s.err
	receipt := proto.Clone(s.receipt)
	s.mu.Unlock()
	if wait {
		<-ctx.Done()
		return nil, status.Error(codes.DeadlineExceeded, "PRIVATE_DIAGNOSTIC")
	}
	if receipt == nil {
		return &pb.GetCompanyCreationResultResponse{}, err
	}
	value, ok := receipt.(*pb.CompanyCreationReceipt)
	if !ok {
		return nil, status.Error(codes.Internal, "fixture error")
	}
	return &pb.GetCompanyCreationResultResponse{Receipt: value}, err
}
func (s *receiptServer) CreateCompanyBootstrap(_ context.Context, r *pb.CreateCompanyBootstrapRequest) (*pb.CreateCompanyBootstrapResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := proto.Clone(r).(*pb.CreateCompanyBootstrapRequest)
	if !ok {
		return nil, status.Error(codes.Internal, "fixture error")
	}
	s.last = value
	return &pb.CreateCompanyBootstrapResponse{Receipt: s.receipt}, s.err
}

func coreReceiptFixture(t *testing.T) (account.CreationCommand, *pb.CompanyCreationReceipt) {
	t.Helper()
	spec := account.CompanySpec{Name: "Кофейня <&>", Currency: "RUB", Timezone: "Europe/Moscow"}
	hash, err := spec.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	command := account.CreationCommand{ID: uuid.New(), CompanyID: uuid.Must(uuid.NewV7()), InitiatorID: uuid.New(), Spec: spec, RequestSHA256: hash, State: "PREPARED", CreatedAt: time.Now().UTC()}
	return command, &pb.CompanyCreationReceipt{CommandId: command.ID.String(), CompanyId: command.CompanyID.String(), DefaultAccountId: uuid.Must(uuid.NewV7()).String(), CompanySpec: &authpb.CompanyCreateSpec{Name: spec.Name, Currency: spec.Currency, Timezone: spec.Timezone}, RequestSha256: hash, CommittedAt: timestamppb.Now()}
}

func TestClientReceiptBootstrapAndDeadline(t *testing.T) {
	command, receipt := coreReceiptFixture(t)
	server := &receiptServer{receipt: receipt}
	listener := bufconn.Listen(1 << 20)
	rpc := grpc.NewServer()
	pb.RegisterCompanyServiceServer(rpc, server)
	done := make(chan error, 1)
	go func() { done <- rpc.Serve(listener) }()
	t.Cleanup(func() {
		rpc.Stop()
		_ = listener.Close()
		if err := <-done; err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Error(err)
		}
	})
	conn, err := grpc.NewClient("passthrough:///core-fixture", grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDisableRetry())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client, err := New(conn, 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.GetCreationResult(t.Context(), command.ID)
	if err != nil || got.CommandID != command.ID || got.CompanyID != command.CompanyID || got.Spec != command.Spec {
		t.Fatal(got, err)
	}
	if err := client.BootstrapCompany(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	server.mu.Lock()
	last := server.last
	server.wait = true
	server.mu.Unlock()
	if last.CommandId != command.ID.String() || last.CompanyId != command.CompanyID.String() || last.CompanySpec.Name != command.Spec.Name || last.CompanySpec.Inn != nil || last.RequestSha256 != command.RequestSHA256 {
		t.Fatal(last)
	}
	if _, err := client.GetCreationResult(t.Context(), command.ID); !errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "PRIVATE_DIAGNOSTIC") {
		t.Fatal(err)
	}
	if _, err := New(nil, time.Second); err == nil {
		t.Fatal("nil connection accepted")
	}
	if _, err := New(conn, 4*time.Second); err == nil {
		t.Fatal("unbounded timeout accepted")
	}
}

func TestDecodeReceiptRejectsUncommittedOrMismatchedData(t *testing.T) {
	for name, edit := range map[string]func(*pb.CompanyCreationReceipt){
		"command":         func(r *pb.CompanyCreationReceipt) { r.CommandId = uuid.NewString() },
		"company version": func(r *pb.CompanyCreationReceipt) { r.CompanyId = uuid.NewString() },
		"account missing": func(r *pb.CompanyCreationReceipt) { r.DefaultAccountId = "" },
		"account same":    func(r *pb.CompanyCreationReceipt) { r.DefaultAccountId = r.CompanyId },
		"spec missing":    func(r *pb.CompanyCreationReceipt) { r.CompanySpec = nil },
		"empty INN":       func(r *pb.CompanyCreationReceipt) { r.CompanySpec.Inn = new(string) },
		"unnormalized":    func(r *pb.CompanyCreationReceipt) { r.CompanySpec.Name = " Other " },
		"hash":            func(r *pb.CompanyCreationReceipt) { r.RequestSha256 = strings.Repeat("0", 64) },
		"commit absent":   func(r *pb.CompanyCreationReceipt) { r.CommittedAt = nil },
		"commit invalid":  func(r *pb.CompanyCreationReceipt) { r.CommittedAt = &timestamppb.Timestamp{Seconds: 1 << 62} },
	} {
		t.Run(name, func(t *testing.T) {
			command, r := coreReceiptFixture(t)
			edit(r)
			if got, err := decodeReceipt(r, command.ID); !errors.Is(err, account.ErrReceiptMismatch) || got.CommandID != uuid.Nil {
				t.Fatal(got, err)
			}
		})
	}
}

func TestDependencyErrorsRequireTypedBusinessEvidence(t *testing.T) {
	for _, tt := range []struct {
		name   string
		code   codes.Code
		reason pb.CoreErrorReason
		typed  bool
		want   error
	}{
		{"missing", codes.NotFound, pb.CoreErrorReason_CORE_ERROR_REASON_NOT_FOUND, true, account.ErrNotFound},
		{"bare missing", codes.NotFound, pb.CoreErrorReason_CORE_ERROR_REASON_NOT_FOUND, false, account.ErrUnavailable},
		{"peer denial", codes.PermissionDenied, pb.CoreErrorReason_CORE_ERROR_REASON_FORBIDDEN, false, account.ErrUnavailable},
		{"business denial", codes.PermissionDenied, pb.CoreErrorReason_CORE_ERROR_REASON_FORBIDDEN, true, account.ErrForbidden},
		{"conflict", codes.AlreadyExists, pb.CoreErrorReason_CORE_ERROR_REASON_IDEMPOTENCY_CONFLICT, true, account.ErrReceiptMismatch},
		{"deadline", codes.DeadlineExceeded, pb.CoreErrorReason_CORE_ERROR_REASON_UNSPECIFIED, false, context.DeadlineExceeded},
		{"cancel", codes.Canceled, pb.CoreErrorReason_CORE_ERROR_REASON_UNSPECIFIED, false, context.Canceled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := status.New(tt.code, "PRIVATE_DIAGNOSTIC")
			if tt.typed {
				var err error
				s, err = s.WithDetails(&pb.CoreErrorDetail{Reason: tt.reason})
				if err != nil {
					t.Fatal(err)
				}
			}
			cause := s.Err()
			got := dependencyError(cause)
			if !errors.Is(got, tt.want) || !errors.Is(got, cause) || strings.Contains(got.Error(), "PRIVATE_DIAGNOSTIC") {
				t.Fatal(got)
			}
		})
	}
}
