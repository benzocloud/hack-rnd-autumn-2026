package imports

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	company "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/company"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/imports"
	authpb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/auth/v1"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	service "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/imports"
)

type uploadOperations struct {
	operations
	accepted bool
	command  service.UploadCommand
}

func (s *uploadOperations) Upload(_ context.Context, cmd service.UploadCommand, receive func() ([]byte, error)) (domain.Accepted, error) {
	s.command = cmd
	for {
		_, err := receive()
		if errors.Is(err, io.EOF) {
			s.accepted = true
			return domain.Accepted{ID: uuid.New()}, nil
		}
		if err != nil {
			return domain.Accepted{}, err
		}
	}
}

type companyProfile struct{ profile company.Profile }

func (c companyProfile) ReadProfile(context.Context, uuid.UUID) (company.Profile, error) {
	return c.profile, nil
}

type uploadAuthorization struct {
	test    *testing.T
	company uuid.UUID
	context *authpb.WriteContext
	calls   int
	revoke  bool
}

func (a *uploadAuthorization) AuthorizeCompany(_ context.Context, request *authpb.AuthorizeCompanyRequest) (*authpb.AuthorizeCompanyResponse, error) {
	a.calls++
	if request.CompanyId != a.company.String() || request.Permission != authpb.CompanyPermission_COMPANY_PERMISSION_FINANCIAL_WRITE || request.GetWrite() != a.context {
		a.test.Fatal("upload lost scope, financial-write permission or original trusted context")
	}
	if a.revoke && a.calls > 1 {
		safe, err := status.New(codes.Aborted, "membership revoked").WithDetails(&authpb.AuthErrorDetail{Reason: authpb.AuthErrorReason_AUTH_ERROR_REASON_MEMBERSHIP_REVOKED})
		if err != nil {
			a.test.Fatal(err)
		}
		return nil, safe.Err()
	}
	return &authpb.AuthorizeCompanyResponse{ActorId: "original-actor", Role: authpb.CompanyRole_COMPANY_ROLE_FINANCIER}, nil
}

type uploadStream struct {
	grpc.ServerStream
	ctx    context.Context
	frames []*pb.UploadImportRequest
	index  int
	sent   bool
}

func (s *uploadStream) Context() context.Context { return s.ctx }
func (s *uploadStream) Recv() (*pb.UploadImportRequest, error) {
	if s.index == len(s.frames) {
		return nil, io.EOF
	}
	frame := s.frames[s.index]
	s.index++
	return frame, nil
}
func (s *uploadStream) SendAndClose(*pb.UploadImportResponse) error {
	s.sent = true
	return nil
}

func TestUploadRejectsRepeatedMetadataAndReauthorizesBeforeAcceptance(t *testing.T) {
	for _, test := range []struct {
		name      string
		repeated  bool
		revoked   bool
		want      codes.Code
		authCalls int
		accepted  bool
	}{
		{name: "valid stream", want: codes.OK, authCalls: 2, accepted: true},
		{name: "second metadata", repeated: true, want: codes.InvalidArgument, authCalls: 1},
		{name: "revoked at EOF", revoked: true, want: codes.PermissionDenied, authCalls: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			companyID, accountID := uuid.New(), uuid.New()
			write := &authpb.WriteContext{Browser: &authpb.BrowserContext{CookieToken: "session"}, CsrfToken: "csrf"}
			metadata := &pb.UploadImportRequest{Frame: &pb.UploadImportRequest_Metadata{Metadata: &pb.UploadImportMetadata{
				Context: write, Scope: &pb.AccountScope{CompanyId: companyID.String(), AccountId: accountID.String()}, IdempotencyKey: "upload-key",
				Adapter: "dataset_v1", SourceCompanyId: "C001", Currency: "RUB", ByteSize: 1,
			}}}
			stream := &uploadStream{ctx: t.Context(), frames: []*pb.UploadImportRequest{metadata, {Frame: &pb.UploadImportRequest_Chunk{Chunk: []byte("x")}}}}
			if test.repeated {
				stream.frames = append(stream.frames, metadata)
			}
			storage := &uploadOperations{}
			auth := &uploadAuthorization{test: t, company: companyID, context: write, revoke: test.revoked}
			h := &handler{imports: storage, companies: companyProfile{profile: company.Profile{ID: companyID, AccountID: accountID}}, auth: auth}
			err := h.UploadImport(stream)
			if status.Code(err) != test.want || storage.accepted != test.accepted || stream.sent != test.accepted || auth.calls != test.authCalls {
				t.Fatal("wrong upload acceptance boundary", err, storage.accepted, stream.sent, auth.calls)
			}
			if storage.command.ActorID != "original-actor" || storage.command.Scope.CompanyID != companyID || storage.command.Scope.AccountID != accountID {
				t.Fatal("upload passed untrusted identity or scope to service", storage.command)
			}
		})
	}
}
