package core

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/securetls"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/testauth"
)

type coreServer struct {
	corev1.UnimplementedCompanyServiceServer
	calls atomic.Int32
	delay atomic.Bool
}

func (s *coreServer) ReadCompany(ctx context.Context, _ *corev1.ReadCompanyRequest) (*corev1.ReadCompanyResponse, error) {
	s.calls.Add(1)
	if s.delay.Load() {
		<-ctx.Done()
		return nil, status.FromContextError(ctx.Err()).Err()
	}
	return &corev1.ReadCompanyResponse{}, nil
}
func TestCoreMTLSIdentityDeadlineAndCancellation(t *testing.T) {
	p, err := testauth.GeneratePKI(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, peer := range []string{"core", "auth"} {
		t.Run(peer, func(t *testing.T) {
			cert, key := p.CoreCert, p.CoreKey
			if peer == "auth" {
				cert, key = p.AuthCert, p.AuthKey
			}
			tlsConfig, err := securetls.Load(p.CA, cert, key, "spiffe://saldo.local/api", true)
			if err != nil {
				t.Fatal(err)
			}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			fixture := &coreServer{}
			server := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConfig)))
			corev1.RegisterCompanyServiceServer(server, fixture)
			go func() { _ = server.Serve(listener) }()
			t.Cleanup(func() { server.Stop(); _ = listener.Close() })
			conn, err := Dial(listener.Addr().String(), "localhost", p.CA, p.APICert, p.APIKey, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = conn.Close() })
			client := corev1.NewCompanyServiceClient(conn)
			_, err = client.ReadCompany(t.Context(), &corev1.ReadCompanyRequest{})
			if peer == "auth" {
				if err == nil || fixture.calls.Load() != 0 {
					t.Fatal("Auth accepted as Core", err)
				}
				return
			}
			if err != nil || fixture.calls.Load() != 1 {
				t.Fatal(err, fixture.calls.Load())
			}
			fixture.delay.Store(true)
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
			defer cancel()
			if _, err := client.ReadCompany(ctx, &corev1.ReadCompanyRequest{}); status.Code(err) != codes.DeadlineExceeded {
				t.Fatal(err)
			}
			ctx, cancel = context.WithCancel(t.Context())
			cancel()
			if _, err := client.ReadCompany(ctx, &corev1.ReadCompanyRequest{}); status.Code(err) != codes.Canceled {
				t.Fatal(err)
			}
		})
	}
	if _, err := Dial("", "", "", "", "", time.Second); err == nil {
		t.Fatal("empty address")
	}
	if _, err := Dial("127.0.0.1:1", "", p.CA, p.APICert, p.APIKey, 0); err == nil {
		t.Fatal("zero timeout")
	}
	if _, err := Dial("127.0.0.1:1", "", "missing", p.APICert, p.APIKey, time.Second); err == nil {
		t.Fatal("missing CA")
	}
}
