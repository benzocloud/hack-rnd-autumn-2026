//go:build integration

package server

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	appPostgres "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/app/postgres"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
	authpb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/auth/v1"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/testutil/pki"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/mtls"
)

type readAuthServer struct {
	authpb.UnimplementedAuthServiceServer
}

func (s *readAuthServer) AuthorizeCompany(ctx context.Context, r *authpb.AuthorizeCompanyRequest) (*authpb.AuthorizeCompanyResponse, error) {
	if mtls.Identity(ctx) != mtls.Core || r.Permission != authpb.CompanyPermission_COMPANY_PERMISSION_READ || r.GetBrowser().CookieToken != "live-session" {
		return nil, status.Error(codes.PermissionDenied, "wrong peer or context")
	}
	return &authpb.AuthorizeCompanyResponse{ActorId: "auth-user", Role: authpb.CompanyRole_COMPANY_ROLE_OWNER}, nil
}

func TestRuntimeAuthAndShutdown(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Postgres.Host != "127.0.0.1" || cfg.Postgres.Database != "saldo_core_test" {
		t.Fatal("test requires isolated loopback PostgreSQL")
	}
	db, err := appPostgres.Open(t.Context(), cfg.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var marker string
	if err := db.GetContext(t.Context(), &marker, "SELECT coalesce(current_setting('saldo.test_database',true),'')"); err != nil || marker != "saldo-core-integration" {
		t.Fatal("test database marker missing", err)
	}
	ca := pki.New(t)
	authFiles, coreFiles := ca.Files(t, mtls.Auth), ca.Files(t, mtls.Core)
	authServer := grpc.NewServer(grpc.Creds(credentials.NewTLS(pki.Server(t, authFiles))))
	authpb.RegisterAuthServiceServer(authServer, &readAuthServer{})
	authListener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = authServer.Serve(authListener) }()
	t.Cleanup(authServer.Stop)
	reservation, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := reservation.Addr().String()
	if err := reservation.Close(); err != nil {
		t.Fatal(err)
	}
	rpc := config.RPC{Address: address, AuthAddress: authListener.Addr().String(), AuthServerName: "saldo-auth", CAFile: coreFiles.CA, CertificateFile: coreFiles.Certificate, KeyFile: coreFiles.Key, AuthTimeout: time.Second}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() { defer close(result); result <- Run(ctx, cfg, rpc) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-result:
		case <-time.After(3 * time.Second):
			t.Error("runtime goroutine did not stop")
		}
	})
	clientTLS, err := mtls.Client(ca.Files(t, mtls.API), "saldo-core", mtls.API, mtls.Core)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	callCtx, callCancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer callCancel()
	_, err = pb.NewCompanyServiceClient(conn).ReadCompany(callCtx, &pb.ReadCompanyRequest{CompanyId: uuid.NewString(), Context: &authpb.BrowserContext{CookieToken: "live-session"}}, grpc.WaitForReady(true))
	if status.Code(err) != codes.NotFound {
		t.Fatal("real runtime did not authorize and read Core storage", err)
	}
	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runtime shutdown did not finish")
	}
}
