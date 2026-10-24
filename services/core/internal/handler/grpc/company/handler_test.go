package company

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/company"
	authpb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/auth/v1"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/company"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/testutil/pki"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/mtls"
)

type fakeCompanies struct {
	reads   atomic.Int32
	profile domain.Profile
	receipt company.BootstrapResult
	readErr error
}

func (s *fakeCompanies) Bootstrap(_ context.Context, cmd company.BootstrapCommand) (company.BootstrapResult, error) {
	s.reads.Add(1)
	return s.receipt, nil
}
func (s *fakeCompanies) GetBootstrapResult(context.Context, uuid.UUID) (company.BootstrapResult, error) {
	s.reads.Add(1)
	return s.receipt, nil
}
func (s *fakeCompanies) ReadProfile(context.Context, uuid.UUID) (domain.Profile, error) {
	s.reads.Add(1)
	return s.profile, s.readErr
}
func (s *fakeCompanies) Update(_ context.Context, cmd company.UpdateCommand) (domain.Profile, error) {
	s.reads.Add(1)
	if cmd.Name == nil || *cmd.Name != "Updated" || !cmd.ClearINN || cmd.ExpectedVersion != 1 {
		return domain.Profile{}, company.ErrInvalid
	}
	profile := s.profile
	profile.Spec.Name = *cmd.Name
	profile.Spec.INN = ""
	profile.Version++
	return profile, nil
}

type fakeAuth struct {
	calls atomic.Int32
	err   error
}

func (a *fakeAuth) AuthorizeCompany(_ context.Context, r *authpb.AuthorizeCompanyRequest) (*authpb.AuthorizeCompanyResponse, error) {
	a.calls.Add(1)
	return &authpb.AuthorizeCompanyResponse{ActorId: "server-actor", Role: authpb.CompanyRole_COMPANY_ROLE_OWNER}, a.err
}

func testService(t *testing.T, identity string, companies *fakeCompanies, auth *fakeAuth) pb.CompanyServiceClient {
	t.Helper()
	ca := pki.New(t)
	serverTLS, err := mtls.Server(ca.Files(t, mtls.Core))
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(credentials.NewTLS(serverTLS), companies, auth)
	if err != nil {
		t.Fatal(err)
	}
	l := bufconn.Listen(1 << 20)
	go func() { _ = s.Serve(l) }()
	t.Cleanup(s.Stop)
	clientTLS, err := mtls.Client(ca.Files(t, identity), "saldo-core", identity, mtls.Core)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := grpc.NewClient("passthrough:///company", grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return l.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return pb.NewCompanyServiceClient(conn)
}

func companyFixture(t *testing.T) *fakeCompanies {
	t.Helper()
	id := uuid.MustParse("019a0c65-9800-7000-8000-000000000001")
	profile, err := domain.NewProfile(id, uuid.New(), domain.Spec{Name: "Original", Currency: "RUB", Timezone: "Europe/Moscow"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := domain.Fingerprint(profile.Spec)
	return &fakeCompanies{profile: profile, receipt: company.BootstrapResult{CommandID: uuid.New(), CompanyID: id, AccountID: profile.AccountID, Spec: profile.Spec, RequestSHA256: hash, CommittedAt: profile.CreatedAt}}
}

func TestMethodPeerAllowlist(t *testing.T) {
	for _, identity := range []string{mtls.API, mtls.Auth, mtls.Core, "spiffe://saldo.local/stranger"} {
		t.Run(identity, func(t *testing.T) {
			companies := companyFixture(t)
			auth := &fakeAuth{}
			client := testService(t, identity, companies, auth)
			ctx, cancel := context.WithTimeout(metadata.AppendToOutgoingContext(t.Context(), "x-service-identity", mtls.Auth), time.Second)
			defer cancel()
			_, err := client.GetCompanyCreationResult(ctx, &pb.GetCompanyCreationResultRequest{CommandId: companies.receipt.CommandID.String()})
			if identity == mtls.Auth {
				if err != nil {
					t.Fatal(err)
				}
			} else if status.Code(err) != codes.PermissionDenied {
				t.Fatal(err)
			}
			_, err = client.ReadCompany(ctx, &pb.ReadCompanyRequest{CompanyId: companies.profile.ID.String(), Context: &authpb.BrowserContext{CookieToken: "opaque"}})
			if identity == mtls.API {
				if err != nil {
					t.Fatal(err)
				}
			} else if status.Code(err) != codes.PermissionDenied {
				t.Fatal(err)
			}
			want := int32(0)
			if identity == mtls.Auth || identity == mtls.API {
				want = 1
			}
			if companies.reads.Load() != want {
				t.Fatal("peer denial reached business storage", companies.reads.Load())
			}
		})
	}
}

func TestReadAndUpdateUseAuthBeforeStorage(t *testing.T) {
	companies := companyFixture(t)
	auth := &fakeAuth{}
	client := testService(t, mtls.API, companies, auth)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	r, err := client.ReadCompany(ctx, &pb.ReadCompanyRequest{CompanyId: companies.profile.ID.String(), Context: &authpb.BrowserContext{CookieToken: "opaque"}})
	if err != nil || r.Role != authpb.CompanyRole_COMPANY_ROLE_OWNER || r.Company.Inn != nil || r.Company.Version != 1 {
		t.Fatal(r, err)
	}
	name := "Updated"
	u, err := client.UpdateCompany(ctx, &pb.UpdateCompanyRequest{CompanyId: companies.profile.ID.String(), ExpectedVersion: 1, Name: &name,
		InnChange: &pb.UpdateCompanyRequest_ClearInn{ClearInn: &emptypb.Empty{}}, Context: &authpb.WriteContext{Browser: &authpb.BrowserContext{CookieToken: "opaque"}, CsrfToken: "csrf"}})
	if err != nil || u.Company.Name != name || u.Company.Inn != nil || u.Company.Version != 2 || auth.calls.Load() != 2 {
		t.Fatal(u, err)
	}
}

func TestDeniedAuthNeverReadsStorage(t *testing.T) {
	for _, reason := range []authpb.AuthErrorReason{authpb.AuthErrorReason_AUTH_ERROR_REASON_CSRF_INVALID, authpb.AuthErrorReason_AUTH_ERROR_REASON_MEMBERSHIP_REVOKED} {
		t.Run(reason.String(), func(t *testing.T) {
			companies := companyFixture(t)
			code := codes.PermissionDenied
			if reason == authpb.AuthErrorReason_AUTH_ERROR_REASON_MEMBERSHIP_REVOKED {
				code = codes.Aborted
			}
			s, _ := status.New(code, "private cookie=secret").WithDetails(&authpb.AuthErrorDetail{Reason: reason})
			client := testService(t, mtls.API, companies, &fakeAuth{err: s.Err()})
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			_, err := client.ReadCompany(ctx, &pb.ReadCompanyRequest{CompanyId: companies.profile.ID.String(), Context: &authpb.BrowserContext{CookieToken: "revoked"}})
			if status.Code(err) != codes.PermissionDenied || strings.Contains(err.Error(), "secret") || companies.reads.Load() != 0 {
				t.Fatal(err)
			}
		})
	}
}

func TestSafeErrorMapping(t *testing.T) {
	for _, test := range []struct {
		err    error
		code   codes.Code
		reason pb.CoreErrorReason
	}{
		{company.ErrInvalid, codes.InvalidArgument, pb.CoreErrorReason_CORE_ERROR_REASON_VALIDATION_ERROR},
		{company.ErrConflict, codes.Aborted, pb.CoreErrorReason_CORE_ERROR_REASON_VERSION_CONFLICT},
		{company.ErrNotFound, codes.NotFound, pb.CoreErrorReason_CORE_ERROR_REASON_NOT_FOUND},
		{company.ErrUnavailable, codes.Unavailable, pb.CoreErrorReason_CORE_ERROR_REASON_DEPENDENCY_UNAVAILABLE},
		{errors.New("private password=secret"), codes.Internal, pb.CoreErrorReason_CORE_ERROR_REASON_INTERNAL_ERROR},
	} {
		err := mapError(test.err)
		if status.Code(err) != test.code || strings.Contains(err.Error(), "secret") {
			t.Fatal(err)
		}
		details := status.Convert(err).Details()
		if len(details) != 1 {
			t.Fatal(err)
		}
		detail, ok := details[0].(*pb.CoreErrorDetail)
		if !ok || detail.Reason != test.reason {
			t.Fatal(details)
		}
	}
}
