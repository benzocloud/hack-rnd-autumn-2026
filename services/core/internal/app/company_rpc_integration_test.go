//go:build integration

package app

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	authClient "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/client/auth"
	authpb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/auth/v1"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	companyHandler "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/company"
	companyService "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/company"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/testutil/pki"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/mtls"
)

// This is a contract-compatible Auth double, not the real Auth runtime.
type bootstrapAuthServer struct {
	authpb.UnimplementedAuthServiceServer
	command companyService.BootstrapCommand
}

func (s *bootstrapAuthServer) AuthorizeCompanyBootstrap(ctx context.Context, r *authpb.AuthorizeCompanyBootstrapRequest) (*authpb.AuthorizeCompanyBootstrapResponse, error) {
	if mtls.Identity(ctx) != mtls.Core {
		return nil, status.Error(codes.PermissionDenied, "wrong peer")
	}
	cmd := s.command
	if r.CommandId != cmd.CommandID.String() || r.CompanyId != cmd.CompanyID.String() || r.RequestSha256 != cmd.RequestSHA256 {
		return nil, status.Error(codes.PermissionDenied, "wrong command")
	}
	return &authpb.AuthorizeCompanyBootstrapResponse{Command: &authpb.CompanyCreationCommand{Id: r.CommandId, CompanyId: r.CompanyId, RequestSha256: r.RequestSha256,
		InitiatorId: "auth-owned-user", Company: &authpb.CompanyCreateSpec{Name: cmd.Spec.Name, Inn: &cmd.Spec.INN, Currency: cmd.Spec.Currency, Timezone: cmd.Spec.Timezone},
		State: authpb.CompanyCreationState_COMPANY_CREATION_STATE_PREPARED, CreatedAt: timestamppb.Now()}}, nil
}

func (s *bootstrapAuthServer) AuthorizeCompany(ctx context.Context, r *authpb.AuthorizeCompanyRequest) (*authpb.AuthorizeCompanyResponse, error) {
	if mtls.Identity(ctx) != mtls.Core {
		return nil, status.Error(codes.PermissionDenied, "wrong peer")
	}
	if r.CompanyId != s.command.CompanyID.String() {
		return nil, status.Error(codes.NotFound, "foreign company")
	}
	cookie := r.GetBrowser().GetCookieToken()
	if r.GetWrite() != nil {
		cookie = r.GetWrite().GetBrowser().GetCookieToken()
	}
	if cookie != "live-session" {
		s, _ := status.New(codes.Aborted, "private revoked user").WithDetails(&authpb.AuthErrorDetail{Reason: authpb.AuthErrorReason_AUTH_ERROR_REASON_MEMBERSHIP_REVOKED})
		return nil, s.Err()
	}
	if r.Permission == authpb.CompanyPermission_COMPANY_PERMISSION_UPDATE && (r.GetWrite() == nil || r.GetWrite().CsrfToken != "valid-csrf") {
		s, _ := status.New(codes.PermissionDenied, "private CSRF").WithDetails(&authpb.AuthErrorDetail{Reason: authpb.AuthErrorReason_AUTH_ERROR_REASON_CSRF_INVALID})
		return nil, s.Err()
	}
	return &authpb.AuthorizeCompanyResponse{ActorId: "auth-owned-user", Role: authpb.CompanyRole_COMPANY_ROLE_OWNER}, nil
}

func testRPCConnection(t *testing.T, ca *pki.Authority, server *grpc.Server, identity, serverIdentity, serverName string) *grpc.ClientConn {
	t.Helper()
	l := bufconn.Listen(1 << 20)
	go func() { _ = server.Serve(l) }()
	t.Cleanup(server.Stop)
	tlsConfig, err := mtls.Client(ca.Files(t, identity), serverName, identity, serverIdentity)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := grpc.NewClient("passthrough:///company-integration", grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return l.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestCompanyRPCBootstrapRecoveryAndCAS(t *testing.T) {
	application, cfg := testApplication(t)
	cmd := bootstrapCommand(t)
	cleanupCompany(t, application, cmd.CompanyID)
	ca := pki.New(t)
	authServer := grpc.NewServer(grpc.Creds(credentials.NewTLS(pki.Server(t, ca.Files(t, mtls.Auth)))))
	authpb.RegisterAuthServiceServer(authServer, &bootstrapAuthServer{command: cmd})
	authConn := testRPCConnection(t, ca, authServer, mtls.Core, mtls.Auth, "saldo-auth")
	auth, err := authClient.New(authConn, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	companies := newCompanies(application.db, cfg.Postgres, auth.AuthorizeBootstrap)
	tlsConfig, err := mtls.Server(ca.Files(t, mtls.Core))
	if err != nil {
		t.Fatal(err)
	}
	server, err := companyHandler.NewServer(credentials.NewTLS(tlsConfig), companies, auth)
	if err != nil {
		t.Fatal(err)
	}
	client := pb.NewCompanyServiceClient(testRPCConnection(t, ca, server, mtls.API, mtls.Core, "saldo-core"))
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	request := &pb.CreateCompanyBootstrapRequest{CommandId: cmd.CommandID.String(), CompanyId: cmd.CompanyID.String(), RequestSha256: cmd.RequestSHA256,
		CompanySpec: &authpb.CompanyCreateSpec{Name: cmd.Spec.Name, Inn: &cmd.Spec.INN, Currency: cmd.Spec.Currency, Timezone: cmd.Spec.Timezone}}
	first, err := client.CreateCompanyBootstrap(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := client.CreateCompanyBootstrap(ctx, request)
	if err != nil || !proto.Equal(first, replay) {
		t.Fatal("RPC retry changed committed receipt", replay, err)
	}
	for _, table := range []string{"company", "account", "profile", "receipt"} {
		if companyCount(t, application, cmd.CompanyID, table) != 1 {
			t.Fatal("bootstrap duplicated", table)
		}
	}
	if _, err := client.GetCompanyCreationResult(ctx, &pb.GetCompanyCreationResultRequest{CommandId: cmd.CommandID.String()}); status.Code(err) != codes.PermissionDenied {
		t.Fatal("API read recovery receipt", err)
	}

	// Reconstruct app/service/server with another SQL pool, then recover as Auth.
	second, _ := testApplication(t)
	recoveryServer, err := companyHandler.NewServer(credentials.NewTLS(tlsConfig), newCompanies(second.db, cfg.Postgres, auth.AuthorizeBootstrap), auth)
	if err != nil {
		t.Fatal(err)
	}
	recovery := pb.NewCompanyServiceClient(testRPCConnection(t, ca, recoveryServer, mtls.Auth, mtls.Core, "saldo-core"))
	recovered, err := recovery.GetCompanyCreationResult(ctx, &pb.GetCompanyCreationResultRequest{CommandId: cmd.CommandID.String()})
	if err != nil || !proto.Equal(first.Receipt, recovered.GetReceipt()) {
		t.Fatal("recovery did not read persisted result", recovered, err)
	}

	read := &pb.ReadCompanyRequest{CompanyId: cmd.CompanyID.String(), Context: &authpb.BrowserContext{CookieToken: "live-session"}}
	profile, err := client.ReadCompany(ctx, read)
	if err != nil || profile.Company.Version != 1 || profile.Role != authpb.CompanyRole_COMPANY_ROLE_OWNER {
		t.Fatal(profile, err)
	}
	name := "Updated coffee shop"
	patch := &pb.UpdateCompanyRequest{CompanyId: cmd.CompanyID.String(), ExpectedVersion: 1, Name: &name, InnChange: &pb.UpdateCompanyRequest_ClearInn{ClearInn: &emptypb.Empty{}},
		Context: &authpb.WriteContext{Browser: read.Context, CsrfToken: "bad-csrf"}}
	if _, err := client.UpdateCompany(ctx, patch); status.Code(err) != codes.PermissionDenied {
		t.Fatal("invalid CSRF accepted", err)
	}
	patch.Context.CsrfToken = "valid-csrf"
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Go(func() { _, errs[i] = client.UpdateCompany(ctx, patch) })
	}
	wg.Wait()
	success, conflict := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			success++
		case status.Code(err) == codes.Aborted:
			conflict++
		default:
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("profile CAS did not serialize concurrent edits", errs)
	}
	profile, err = client.ReadCompany(ctx, read)
	if err != nil || profile.Company.Version != 2 || profile.Company.Name != name || profile.Company.Inn != nil {
		t.Fatal(profile, err)
	}
	recovered, err = recovery.GetCompanyCreationResult(ctx, &pb.GetCompanyCreationResultRequest{CommandId: cmd.CommandID.String()})
	if err != nil || !proto.Equal(first.Receipt, recovered.GetReceipt()) {
		t.Fatal("profile edit rewrote immutable receipt", recovered, err)
	}
	var version int64
	if err := application.db.GetContext(ctx, &version, "SELECT data_version FROM core_accounts WHERE company_id=$1", cmd.CompanyID); err != nil || version != 1 {
		t.Fatal("profile edit changed financial data version", version, err)
	}
	read.Context = &authpb.BrowserContext{CookieToken: "revoked-session"}
	if _, err := client.ReadCompany(ctx, read); status.Code(err) != codes.PermissionDenied {
		t.Fatal("revoked membership accepted", err)
	}
}
