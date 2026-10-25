//go:build integration

package integration_test

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/app"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/gen/auth/v1"
	handler "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/handler/grpc"
)

// This in-memory listener exercises protobuf adapters with real PostgreSQL.
// The production listener's CA/SAN enforcement is tested with real TLS separately.
func TestAccountRPCWireFlow(t *testing.T) {
	_, db := scratch(t)
	cfg := accountConfiguration(t, db, true)
	if err := app.Migrate(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	application, err := app.New(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	listener := bufconn.Listen(1 << 20)
	srv := grpcgo.NewServer()
	pb.RegisterAuthServiceServer(srv, handler.New(application.Account))
	served := make(chan error, 1)
	go func() { served <- srv.Serve(listener) }()
	t.Cleanup(func() {
		srv.Stop()
		_ = listener.Close()
		if err := <-served; err != nil && !errors.Is(err, grpcgo.ErrServerStopped) {
			t.Error("test RPC server failed")
		}
	})
	connection, err := grpcgo.NewClient("passthrough:///auth-test", grpcgo.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }), grpcgo.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	client := pb.NewAuthServiceClient(connection)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	document, err := client.GetPersonalDataConsent(ctx, &pb.GetPersonalDataConsentRequest{})
	if err != nil {
		t.Fatal(err)
	}
	browser, err := client.GetSecurityContext(ctx, &pb.GetSecurityContextRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if browser.GetEmailVerificationMode() != "demo" {
		t.Fatal("server mode not exposed to API")
	}
	write := &pb.WriteContext{Browser: &pb.BrowserContext{CookieToken: browser.GetContext().GetCookieToken()}, CsrfToken: browser.GetContext().GetCsrfToken()}
	consent := &pb.ConsentAcceptance{Accepted: true, DocumentVersion: document.GetDocument().GetDocumentVersion(), DocumentSha256: document.GetDocument().GetDocumentSha256()}
	registered, err := client.Register(ctx, &pb.RegisterRequest{Context: write, Email: "alice@example.test", Password: accountPassword, DisplayName: "Тест", PersonalDataConsent: consent})
	if err != nil {
		t.Fatal(err)
	}
	ack, err := client.VerifyEmail(ctx, &pb.VerifyEmailRequest{Context: write, ChallengeId: registered.GetChallenge().GetChallengeId(), Code: "000000"})
	if err != nil || ack == nil {
		t.Fatalf("verification acknowledgement: %v", err)
	}
	if _, err := client.GetCurrentAccount(ctx, &pb.GetCurrentAccountRequest{Context: write.Browser}); status.Code(err) != codes.Unauthenticated {
		t.Fatal("verification unexpectedly logged in")
	}
	logged, err := client.Login(ctx, &pb.LoginRequest{Context: write, Email: "alice@example.test", Password: accountPassword})
	if err != nil {
		t.Fatal(err)
	}
	if logged.GetContext().GetCookieToken() == write.Browser.GetCookieToken() || !logged.GetUser().GetEmailVerified() {
		t.Fatal("login failed to rotate real context")
	}
	company, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO memberships(company_id,user_id,role) VALUES($1,$2,'owner')", company, logged.GetUser().GetId()); err != nil {
		t.Fatal(err)
	}
	live := &pb.BrowserContext{CookieToken: logged.GetContext().GetCookieToken()}
	creationWrite := &pb.WriteContext{Browser: live, CsrfToken: logged.GetContext().GetCsrfToken()}
	creationRequest := &pb.BeginCompanyCreationRequest{Context: creationWrite, IdempotencyKey: "rpc-company-creation-key", Company: &pb.CompanyCreateSpec{Name: " Кофейня ", Currency: "RUB", Timezone: "Europe/Moscow"}}
	prepared, err := client.BeginCompanyCreation(ctx, creationRequest)
	if err != nil || prepared.GetCommand().GetCompany().GetName() != "Кофейня" || prepared.GetCommand().GetInitiatorId() != logged.GetUser().GetId() {
		t.Fatal(prepared, err)
	}
	replay, err := client.BeginCompanyCreation(ctx, creationRequest)
	if err != nil || replay.GetCommand().GetId() != prepared.GetCommand().GetId() || replay.GetCommand().GetCompanyId() != prepared.GetCommand().GetCompanyId() {
		t.Fatal(replay, err)
	}
	creationRequest.Company.Name = "Different"
	if _, err := client.BeginCompanyCreation(ctx, creationRequest); status.Code(err) != codes.Aborted {
		t.Fatal("body conflict not mapped", err)
	}
	if _, err := client.CompleteCompanyCreation(ctx, &pb.CompleteCompanyCreationRequest{Context: creationWrite, CommandId: prepared.GetCommand().GetId()}); status.Code(err) != codes.Unavailable {
		t.Fatal("missing Core fabricated completion", err)
	}
	grant, err := client.AuthorizeCompanyBootstrap(ctx, &pb.AuthorizeCompanyBootstrapRequest{CommandId: prepared.GetCommand().GetId(), CompanyId: prepared.GetCommand().GetCompanyId(), RequestSha256: prepared.GetCommand().GetRequestSha256()})
	if err != nil || grant.GetCommand().GetId() != prepared.GetCommand().GetId() {
		t.Fatal(grant, err)
	}
	allowed, err := client.AuthorizeCompany(ctx, &pb.AuthorizeCompanyRequest{Context: &pb.AuthorizeCompanyRequest_Browser{Browser: live}, CompanyId: company.String(), Permission: pb.CompanyPermission_COMPANY_PERMISSION_FINANCIAL_READ})
	if err != nil || allowed.GetActorId() != logged.GetUser().GetId() {
		t.Fatalf("company read: %v", err)
	}
	if _, err := client.AuthorizeCompany(ctx, &pb.AuthorizeCompanyRequest{Context: &pb.AuthorizeCompanyRequest_Browser{Browser: live}, CompanyId: company.String(), Permission: pb.CompanyPermission_COMPANY_PERMISSION_FINANCIAL_WRITE}); status.Code(err) != codes.PermissionDenied {
		t.Fatal("financial write bypassed CSRF")
	}
	if _, err := client.AuthorizeCompany(ctx, &pb.AuthorizeCompanyRequest{Context: &pb.AuthorizeCompanyRequest_Browser{Browser: live}, CompanyId: uuid.NewString(), Permission: pb.CompanyPermission_COMPANY_PERMISSION_FINANCIAL_READ}); status.Code(err) != codes.NotFound {
		t.Fatal("foreign company scope accepted")
	}
	stale := &pb.ConsentAcceptance{Accepted: true, DocumentVersion: consent.GetDocumentVersion(), DocumentSha256: strings.Repeat("0", 64)}
	fresh, err := client.GetSecurityContext(ctx, &pb.GetSecurityContextRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Register(ctx, &pb.RegisterRequest{Context: &pb.WriteContext{Browser: &pb.BrowserContext{CookieToken: fresh.GetContext().GetCookieToken()}, CsrfToken: fresh.GetContext().GetCsrfToken()}, Email: "bob@example.test", Password: accountPassword, DisplayName: "Тест", PersonalDataConsent: stale}); status.Code(err) != codes.Aborted {
		t.Fatal("consent version conflict lost status mapping")
	}
}
