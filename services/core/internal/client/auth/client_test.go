package auth

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/company"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/auth/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/company"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/testutil/pki"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/mtls"
)

type testServer struct {
	pb.UnimplementedAuthServiceServer
	bootstrap func(context.Context, *pb.AuthorizeCompanyBootstrapRequest) (*pb.AuthorizeCompanyBootstrapResponse, error)
	authorize func(context.Context, *pb.AuthorizeCompanyRequest) (*pb.AuthorizeCompanyResponse, error)
}

func (s *testServer) AuthorizeCompanyBootstrap(ctx context.Context, r *pb.AuthorizeCompanyBootstrapRequest) (*pb.AuthorizeCompanyBootstrapResponse, error) {
	return s.bootstrap(ctx, r)
}
func (s *testServer) AuthorizeCompany(ctx context.Context, r *pb.AuthorizeCompanyRequest) (*pb.AuthorizeCompanyResponse, error) {
	return s.authorize(ctx, r)
}

func testClient(t *testing.T, server *testServer, timeout time.Duration) *client {
	t.Helper()
	ca := pki.New(t)
	authFiles, coreFiles := ca.Files(t, mtls.Auth), ca.Files(t, mtls.Core)
	s := grpc.NewServer(grpc.Creds(credentials.NewTLS(pki.Server(t, authFiles))))
	pb.RegisterAuthServiceServer(s, server)
	l := bufconn.Listen(1 << 20)
	go func() { _ = s.Serve(l) }()
	t.Cleanup(s.Stop)
	tlsConfig, err := mtls.Client(coreFiles, "saldo-auth", mtls.Core, mtls.Auth)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := grpc.NewClient("passthrough:///auth-test", grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return l.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	c, err := New(conn, timeout)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func bootstrapFixture() (*pb.CompanyCreationCommand, company.BootstrapCommand) {
	spec := domain.Spec{Name: "Кофейня", INN: "001234567890", Currency: "RUB", Timezone: "Europe/Moscow"}
	hash, _ := domain.Fingerprint(spec)
	id := uuid.MustParse("019a0c65-9800-7000-8000-000000000001")
	cmd := company.BootstrapCommand{CommandID: uuid.New(), CompanyID: id, RequestSHA256: hash, Spec: spec}
	return &pb.CompanyCreationCommand{Id: cmd.CommandID.String(), CompanyId: id.String(), InitiatorId: "auth-user", Company: &pb.CompanyCreateSpec{Name: spec.Name, Inn: &spec.INN, Currency: spec.Currency, Timezone: spec.Timezone}, RequestSha256: hash,
		State: pb.CompanyCreationState_COMPANY_CREATION_STATE_PREPARED, CreatedAt: timestamppb.Now()}, cmd
}

func TestAuthorizeBootstrap(t *testing.T) {
	grant, cmd := bootstrapFixture()
	c := testClient(t, &testServer{bootstrap: func(ctx context.Context, r *pb.AuthorizeCompanyBootstrapRequest) (*pb.AuthorizeCompanyBootstrapResponse, error) {
		if mtls.Identity(ctx) != mtls.Core || r.CommandId != cmd.CommandID.String() || r.CompanyId != cmd.CompanyID.String() || r.RequestSha256 != cmd.RequestSHA256 {
			t.Error("lost peer or durable command scope")
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > time.Second {
			t.Error("missing bounded Auth deadline")
		}
		return &pb.AuthorizeCompanyBootstrapResponse{Command: grant}, nil
	}}, time.Second)
	result, err := c.AuthorizeBootstrap(t.Context(), cmd.CommandID, cmd.CompanyID, cmd.RequestSHA256)
	if err != nil || result != company.Authorization(cmd) {
		t.Fatal(result, err)
	}
}

func TestRejectInvalidBootstrapGrant(t *testing.T) {
	for name, edit := range map[string]func(*pb.CompanyCreationCommand){
		"command": func(c *pb.CompanyCreationCommand) { c.Id = uuid.NewString() },
		"company": func(c *pb.CompanyCreationCommand) { c.CompanyId = uuid.NewString() },
		"hash":    func(c *pb.CompanyCreationCommand) { c.RequestSha256 = "other" },
		"spec":    func(c *pb.CompanyCreationCommand) { c.Company.Name = "Other" },
		"quarantined": func(c *pb.CompanyCreationCommand) {
			c.State = pb.CompanyCreationState_COMPANY_CREATION_STATE_QUARANTINED
		},
		"unknown state":     func(c *pb.CompanyCreationCommand) { c.State = 99 },
		"missing spec":      func(c *pb.CompanyCreationCommand) { c.Company = nil },
		"missing initiator": func(c *pb.CompanyCreationCommand) { c.InitiatorId = "" },
		"missing timestamp": func(c *pb.CompanyCreationCommand) { c.CreatedAt = nil },
	} {
		t.Run(name, func(t *testing.T) {
			grant, cmd := bootstrapFixture()
			edit(grant)
			c := testClient(t, &testServer{bootstrap: func(context.Context, *pb.AuthorizeCompanyBootstrapRequest) (*pb.AuthorizeCompanyBootstrapResponse, error) {
				return &pb.AuthorizeCompanyBootstrapResponse{Command: grant}, nil
			}}, time.Second)
			if _, err := c.AuthorizeBootstrap(t.Context(), cmd.CommandID, cmd.CompanyID, cmd.RequestSHA256); !errors.Is(err, company.ErrUnauthorized) {
				t.Fatal(err)
			}
		})
	}
}

func TestAuthorizeCompanyPreservesWriteContext(t *testing.T) {
	_, cmd := bootstrapFixture()
	write := &pb.WriteContext{Browser: &pb.BrowserContext{CookieToken: "opaque"}, CsrfToken: "csrf", Client: &pb.ClientContext{ClientIp: "127.0.0.1", RequestId: "trace"}}
	c := testClient(t, &testServer{authorize: func(_ context.Context, r *pb.AuthorizeCompanyRequest) (*pb.AuthorizeCompanyResponse, error) {
		if r.GetWrite().GetBrowser().GetCookieToken() != write.Browser.CookieToken || r.GetWrite().CsrfToken != write.CsrfToken || r.GetWrite().Client.RequestId != "trace" || r.Permission != pb.CompanyPermission_COMPANY_PERMISSION_UPDATE {
			t.Error("lost original write context")
		}
		return &pb.AuthorizeCompanyResponse{ActorId: "server-actor", Role: pb.CompanyRole_COMPANY_ROLE_OWNER}, nil
	}}, time.Second)
	r, err := c.AuthorizeCompany(t.Context(), &pb.AuthorizeCompanyRequest{CompanyId: cmd.CompanyID.String(), Permission: pb.CompanyPermission_COMPANY_PERMISSION_UPDATE, Context: &pb.AuthorizeCompanyRequest_Write{Write: write}})
	if err != nil || r.ActorId != "server-actor" {
		t.Fatal(r, err)
	}
	if _, err := c.AuthorizeCompany(t.Context(), &pb.AuthorizeCompanyRequest{CompanyId: cmd.CompanyID.String(), Permission: pb.CompanyPermission_COMPANY_PERMISSION_UPDATE, Context: &pb.AuthorizeCompanyRequest_Browser{Browser: write.Browser}}); !errors.Is(err, company.ErrInvalid) {
		t.Fatal(err)
	}
}

func TestAuthErrorsAndDeadline(t *testing.T) {
	_, cmd := bootstrapFixture()
	for _, typed := range []bool{false, true} {
		s := status.New(codes.PermissionDenied, "private cookie=secret")
		if typed {
			s, _ = s.WithDetails(&pb.AuthErrorDetail{Reason: pb.AuthErrorReason_AUTH_ERROR_REASON_CSRF_INVALID, FieldViolations: []*pb.FieldViolation{{Field: "private", Description: "secret"}}})
		}
		c := testClient(t, &testServer{bootstrap: func(context.Context, *pb.AuthorizeCompanyBootstrapRequest) (*pb.AuthorizeCompanyBootstrapResponse, error) {
			return nil, s.Err()
		}}, time.Second)
		_, err := c.AuthorizeBootstrap(t.Context(), cmd.CommandID, cmd.CompanyID, cmd.RequestSHA256)
		want := codes.Unavailable
		if typed {
			want = codes.PermissionDenied
		}
		if status.Code(err) != want || strings.Contains(err.Error(), "secret") {
			t.Fatal(err)
		}
		for _, raw := range status.Convert(err).Details() {
			if d, ok := raw.(*pb.AuthErrorDetail); ok && len(d.FieldViolations) > 0 {
				t.Fatal("private Auth fields exposed")
			}
		}
	}
	c := testClient(t, &testServer{bootstrap: func(ctx context.Context, _ *pb.AuthorizeCompanyBootstrapRequest) (*pb.AuthorizeCompanyBootstrapResponse, error) {
		<-ctx.Done()
		return nil, status.FromContextError(ctx.Err()).Err()
	}}, time.Second)
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if _, err := c.AuthorizeBootstrap(ctx, cmd.CommandID, cmd.CompanyID, cmd.RequestSHA256); status.Code(err) != codes.DeadlineExceeded {
		t.Fatal(err)
	}
}

func TestRejectInvalidCompanyGrant(t *testing.T) {
	_, cmd := bootstrapFixture()
	for name, grant := range map[string]*pb.AuthorizeCompanyResponse{
		"missing actor":           {Role: pb.CompanyRole_COMPANY_ROLE_OWNER},
		"unknown role":            {ActorId: "actor", Role: 99},
		"financier cannot update": {ActorId: "actor", Role: pb.CompanyRole_COMPANY_ROLE_FINANCIER},
	} {
		t.Run(name, func(t *testing.T) {
			c := testClient(t, &testServer{authorize: func(context.Context, *pb.AuthorizeCompanyRequest) (*pb.AuthorizeCompanyResponse, error) {
				return grant, nil
			}}, time.Second)
			result, err := c.AuthorizeCompany(t.Context(), &pb.AuthorizeCompanyRequest{CompanyId: cmd.CompanyID.String(), Permission: pb.CompanyPermission_COMPANY_PERMISSION_UPDATE,
				Context: &pb.AuthorizeCompanyRequest_Write{Write: &pb.WriteContext{Browser: &pb.BrowserContext{CookieToken: "live"}, CsrfToken: "csrf"}}})
			if !errors.Is(err, company.ErrUnavailable) || result != nil {
				t.Fatal(result, err)
			}
		})
	}
}

func TestRateLimitRetainsSafeRetryHint(t *testing.T) {
	s, _ := status.New(codes.ResourceExhausted, "private").WithDetails(&pb.AuthErrorDetail{Reason: pb.AuthErrorReason_AUTH_ERROR_REASON_RATE_LIMITED, RetryAfterSeconds: 12})
	err := safeError(s.Err())
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatal(err)
	}
	details := status.Convert(err).Details()
	if len(details) != 1 {
		t.Fatal(err)
	}
	detail, ok := details[0].(*pb.AuthErrorDetail)
	if !ok || detail.RetryAfterSeconds != 12 {
		t.Fatal(details)
	}
}
