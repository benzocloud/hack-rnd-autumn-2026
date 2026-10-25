package imports

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	company "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/company"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/imports"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	authpb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/auth/v1"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/rpcerror"
	service "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/imports"
)

const importListBudget = 1536 << 10

type operations interface {
	Commit(context.Context, service.CommitCommand) (service.CommitResult, error)
	Upload(context.Context, service.UploadCommand, func() ([]byte, error)) (domain.Accepted, error)
	Read(context.Context, ledger.Scope, uuid.UUID) (domain.Record, error)
	List(context.Context, service.ListCommand) (service.ListResult, error)
	Issues(context.Context, service.IssuesCommand) (service.IssuesResult, error)
}
type companies interface {
	ReadProfile(context.Context, uuid.UUID) (company.Profile, error)
}
type authorizer interface {
	AuthorizeCompany(context.Context, *authpb.AuthorizeCompanyRequest) (*authpb.AuthorizeCompanyResponse, error)
}
type handler struct {
	pb.UnimplementedImportsServiceServer
	imports   operations
	companies companies
	auth      authorizer
}

func Register(server grpc.ServiceRegistrar, operations operations, companies companies, auth authorizer) error {
	if server == nil || operations == nil || companies == nil || auth == nil {
		return domain.ErrUnavailable
	}
	pb.RegisterImportsServiceServer(server, &handler{imports: operations, companies: companies, auth: auth})
	return nil
}
func scope(raw *pb.AccountScope) (ledger.Scope, error) {
	if raw == nil {
		return ledger.Scope{}, domain.ErrInvalid
	}
	company, cerr := uuid.Parse(raw.CompanyId)
	account, aerr := uuid.Parse(raw.AccountId)
	if cerr != nil || aerr != nil || company == uuid.Nil || account == uuid.Nil || company.String() != raw.CompanyId || account.String() != raw.AccountId {
		return ledger.Scope{}, domain.ErrInvalid
	}
	return ledger.Scope{CompanyID: company, AccountID: account}, nil
}
func parseID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil || id.String() != raw {
		return uuid.Nil, domain.ErrInvalid
	}
	return id, nil
}
func (h *handler) authorize(ctx context.Context, scope ledger.Scope, browser *authpb.BrowserContext, write *authpb.WriteContext) (string, error) {
	request := &authpb.AuthorizeCompanyRequest{CompanyId: scope.CompanyID.String(), Permission: authpb.CompanyPermission_COMPANY_PERMISSION_FINANCIAL_READ, Context: &authpb.AuthorizeCompanyRequest_Browser{Browser: browser}}
	if write != nil {
		request.Permission = authpb.CompanyPermission_COMPANY_PERMISSION_FINANCIAL_WRITE
		request.Context = &authpb.AuthorizeCompanyRequest_Write{Write: write}
	}
	grant, err := h.auth.AuthorizeCompany(ctx, request)
	if err != nil {
		return "", err
	}
	if grant == nil || grant.ActorId == "" {
		return "", domain.ErrUnavailable
	}
	profile, err := h.companies.ReadProfile(ctx, scope.CompanyID)
	if err != nil {
		return "", err
	}
	if profile.AccountID != scope.AccountID {
		return "", domain.ErrNotFound
	}
	return grant.ActorId, nil
}
func (h *handler) UploadImport(stream grpc.ClientStreamingServer[pb.UploadImportRequest, pb.UploadImportResponse]) error {
	first, err := stream.Recv()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return fromError(domain.ErrInvalid)
		}
		return fromError(err)
	}
	metadata := first.GetMetadata()
	if metadata == nil || metadata.Context == nil || proto.Size(metadata) > 64<<10 {
		return fromError(domain.ErrInvalid)
	}
	scope, err := scope(metadata.Scope)
	if err != nil {
		return fromError(err)
	}
	actor, err := h.authorize(stream.Context(), scope, nil, metadata.Context)
	if err != nil {
		return fromError(err)
	}
	if metadata.ByteSize > domain.MaxBytes {
		return fromError(domain.ErrTooLarge)
	}
	cmd := service.UploadCommand{Scope: scope, ActorID: actor, Key: metadata.IdempotencyKey, Adapter: metadata.Adapter, SourceCompanyID: metadata.SourceCompanyId, Currency: metadata.Currency, Digest: metadata.FileSha256, ByteSize: int64(metadata.ByteSize)}
	receive := func() ([]byte, error) {
		frame, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			current, authErr := h.authorize(stream.Context(), scope, nil, metadata.Context)
			if authErr != nil {
				return nil, authErr
			}
			if current != actor {
				return nil, domain.ErrState
			}
			return nil, io.EOF
		}
		if err != nil {
			return nil, err
		}
		chunk, ok := frame.Frame.(*pb.UploadImportRequest_Chunk)
		if !ok {
			return nil, domain.ErrInvalid
		}
		return chunk.Chunk, nil
	}
	accepted, err := h.imports.Upload(stream.Context(), cmd, receive)
	if err != nil {
		return fromError(err)
	}
	return stream.SendAndClose(&pb.UploadImportResponse{Accepted: &pb.ImportAccepted{Id: accepted.ID.String(), Status: pb.ImportStatus_IMPORT_STATUS_QUEUED, PollAfterMs: 1000}})
}
func (h *handler) GetImport(ctx context.Context, r *pb.GetImportRequest) (*pb.GetImportResponse, error) {
	if r == nil {
		return nil, fromError(domain.ErrInvalid)
	}
	scope, err := scope(r.Scope)
	if err != nil {
		return nil, fromError(err)
	}
	if _, err := h.authorize(ctx, scope, r.Context, nil); err != nil {
		return nil, fromError(err)
	}
	id, err := parseID(r.ImportId)
	if err != nil {
		return nil, fromError(err)
	}
	record, err := h.imports.Read(ctx, scope, id)
	if err != nil {
		return nil, fromError(err)
	}
	return &pb.GetImportResponse{Import: encode(record)}, nil
}
func page(p *pb.PageRequest) (int, string) {
	if p == nil {
		return 50, ""
	}
	if p.Limit == nil {
		return 50, p.Cursor
	}
	return int(*p.Limit), p.Cursor
}
func (h *handler) ListImports(ctx context.Context, r *pb.ListImportsRequest) (*pb.ListImportsResponse, error) {
	if r == nil {
		return nil, fromError(domain.ErrInvalid)
	}
	scope, err := scope(r.Scope)
	if err != nil {
		return nil, fromError(err)
	}
	actor, err := h.authorize(ctx, scope, r.Context, nil)
	if err != nil {
		return nil, fromError(err)
	}
	limit, cursor := page(r.Page)
	if limit == 0 {
		limit = 50
	}
	for {
		result, err := h.imports.List(ctx, service.ListCommand{Scope: scope, ActorID: actor, Limit: limit, Cursor: cursor})
		if err != nil {
			return nil, fromError(err)
		}
		response := &pb.ListImportsResponse{Items: []*pb.Import{}, Pagination: &pb.PageResponse{NextCursor: result.Cursor}}
		for _, record := range result.Items {
			response.Items = append(response.Items, encode(record))
		}
		if proto.Size(response) <= importListBudget {
			return response, nil
		}
		if limit <= 1 {
			return nil, fromError(domain.ErrTooLarge)
		}
		// Query from the original cursor again so the smaller page owns its
		// continuation; slicing a larger response would skip omitted records.
		limit = max(1, limit/2)
	}
}
func (h *handler) ListImportIssues(ctx context.Context, r *pb.ListImportIssuesRequest) (*pb.ListImportIssuesResponse, error) {
	if r == nil {
		return nil, fromError(domain.ErrInvalid)
	}
	scope, err := scope(r.Scope)
	if err != nil {
		return nil, fromError(err)
	}
	actor, err := h.authorize(ctx, scope, r.Context, nil)
	if err != nil {
		return nil, fromError(err)
	}
	id, err := parseID(r.ImportId)
	if err != nil {
		return nil, fromError(err)
	}
	limit, cursor := page(r.Page)
	result, err := h.imports.Issues(ctx, service.IssuesCommand{Scope: scope, ActorID: actor, ID: id, Limit: limit, Cursor: cursor})
	if err != nil {
		return nil, fromError(err)
	}
	response := &pb.ListImportIssuesResponse{Items: []*pb.ImportIssue{}, Pagination: &pb.PageResponse{NextCursor: result.Cursor}}
	for _, issue := range result.Items {
		response.Items = append(response.Items, encodeIssue(issue))
	}
	return response, nil
}
func fromError(err error) error {
	code, reason := codes.Unavailable, pb.CoreErrorReason_CORE_ERROR_REASON_DEPENDENCY_UNAVAILABLE
	switch {
	case errors.Is(err, domain.ErrInvalid):
		code, reason = codes.InvalidArgument, pb.CoreErrorReason_CORE_ERROR_REASON_VALIDATION_ERROR
	case errors.Is(err, domain.ErrCursor):
		code, reason = codes.InvalidArgument, pb.CoreErrorReason_CORE_ERROR_REASON_INVALID_CURSOR
	case errors.Is(err, domain.ErrTooLarge):
		code, reason = codes.ResourceExhausted, pb.CoreErrorReason_CORE_ERROR_REASON_PAYLOAD_TOO_LARGE
	case errors.Is(err, domain.ErrQuota):
		code, reason = codes.ResourceExhausted, pb.CoreErrorReason_CORE_ERROR_REASON_RATE_LIMITED
	case errors.Is(err, domain.ErrNotFound):
		code, reason = codes.NotFound, pb.CoreErrorReason_CORE_ERROR_REASON_NOT_FOUND
	case errors.Is(err, domain.ErrConflict):
		code, reason = codes.AlreadyExists, pb.CoreErrorReason_CORE_ERROR_REASON_IDEMPOTENCY_CONFLICT
	case errors.Is(err, domain.ErrPreviewStale):
		code, reason = codes.Aborted, pb.CoreErrorReason_CORE_ERROR_REASON_IMPORT_PREVIEW_STALE
	case errors.Is(err, domain.ErrWarnings):
		code, reason = codes.FailedPrecondition, pb.CoreErrorReason_CORE_ERROR_REASON_IMPORT_WARNINGS_NOT_ACKNOWLEDGED
	case errors.Is(err, domain.ErrSource):
		code, reason = codes.FailedPrecondition, pb.CoreErrorReason_CORE_ERROR_REASON_FACTUAL_SOURCE_CONFLICT
	case errors.Is(err, domain.ErrSourceBinding):
		code, reason = codes.FailedPrecondition, pb.CoreErrorReason_CORE_ERROR_REASON_SOURCE_BINDING_CONFLICT
	case errors.Is(err, domain.ErrState):
		code, reason = codes.FailedPrecondition, pb.CoreErrorReason_CORE_ERROR_REASON_IMPORT_STATE_CONFLICT
	case errors.Is(err, domain.ErrUnavailable):
	default:
		return rpcerror.From(err)
	}
	return rpcerror.New(code, reason)
}

func (h *handler) CommitImport(ctx context.Context, r *pb.CommitImportRequest) (*pb.CommitImportResponse, error) {
	if r == nil || r.Context == nil {
		return nil, fromError(domain.ErrInvalid)
	}
	scoped, err := scope(r.Scope)
	if err != nil {
		return nil, fromError(err)
	}
	actor, err := h.authorize(ctx, scoped, nil, r.Context)
	if err != nil {
		return nil, fromError(err)
	}
	id, err := parseID(r.ImportId)
	if err != nil {
		return nil, fromError(err)
	}
	location, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		return nil, fromError(domain.ErrUnavailable)
	}
	result, err := h.imports.Commit(ctx, service.CommitCommand{Scope: scoped, ActorID: actor, Key: r.IdempotencyKey, ID: id, ExpectedVersion: r.ExpectedVersion, ExpectedDataVersion: r.ExpectedDataVersion, AcknowledgeWarnings: r.AcknowledgeWarnings, ViewDate: ledger.Date(time.Now().In(location).Format(time.DateOnly))})
	if err != nil {
		return nil, fromError(err)
	}
	return &pb.CommitImportResponse{Import: encode(result.Import)}, nil
}
