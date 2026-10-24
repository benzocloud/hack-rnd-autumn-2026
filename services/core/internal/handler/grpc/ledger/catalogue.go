package ledger

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	authpb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/auth/v1"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/rpcerror"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
)

type catalogueOperations interface {
	ListTransactions(context.Context, ledger.ListTransactionsCommand) (ledger.ListTransactionsResult, error)
	ListCategories(context.Context, ledger.ListCategoriesCommand) (ledger.ListCategoriesResult, error)
	UpdateTransactionCategory(context.Context, ledger.UpdateTransactionCategoryCommand) (ledger.UpdateTransactionCategoryResult, error)
}

func catalogError(err error) error {
	if errors.Is(err, ledger.ErrCursor) {
		return rpcerror.New(codes.InvalidArgument, pb.CoreErrorReason_CORE_ERROR_REASON_INVALID_CURSOR)
	}
	return rpcerror.From(err)
}
func catalogID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil || id.String() != raw {
		return uuid.Nil, ledger.ErrInvalid
	}
	return id, nil
}
func catalogScope(raw *pb.AccountScope) (domain.Scope, error) {
	if raw == nil {
		return domain.Scope{}, ledger.ErrInvalid
	}
	company, err := catalogID(raw.CompanyId)
	if err != nil {
		return domain.Scope{}, err
	}
	account, err := catalogID(raw.AccountId)
	return domain.Scope{CompanyID: company, AccountID: account}, err
}
func catalogPage(raw *pb.PageRequest) (int, string, error) {
	if raw == nil {
		return 50, "", nil
	}
	if raw.Limit == nil {
		return 50, raw.Cursor, nil
	}
	if *raw.Limit < 1 || *raw.Limit > 100 {
		return 0, "", ledger.ErrInvalid
	}
	return int(*raw.Limit), raw.Cursor, nil
}
func (h *handler) catalogAuthorization(ctx context.Context, scope domain.Scope, browser *authpb.BrowserContext, write *authpb.WriteContext) (string, error) {
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
		return "", ledger.ErrUnavailable
	}
	profile, err := h.companies.ReadProfile(ctx, scope.CompanyID)
	if err != nil {
		return "", err
	}
	if profile.AccountID != scope.AccountID {
		return "", ledger.ErrNotFound
	}
	return grant.ActorId, nil
}
func (h *handler) ListTransactions(ctx context.Context, r *pb.ListTransactionsRequest) (*pb.ListTransactionsResponse, error) {
	if r == nil || r.Context == nil {
		return nil, catalogError(ledger.ErrInvalid)
	}
	scope, err := catalogScope(r.Scope)
	if err != nil {
		return nil, catalogError(err)
	}
	actor, err := h.catalogAuthorization(ctx, scope, r.Context, nil)
	if err != nil {
		return nil, catalogError(err)
	}
	operations, ok := h.accounts.(catalogueOperations)
	if !ok {
		return nil, catalogError(ledger.ErrUnavailable)
	}
	limit, cursor, err := catalogPage(r.Page)
	if err != nil {
		return nil, catalogError(err)
	}
	cmd := ledger.ListTransactionsCommand{Scope: scope, ActorID: actor, Limit: limit, Cursor: cursor}
	if err := applyTransactionDates(&cmd, r.Dates); err != nil {
		return nil, catalogError(err)
	}
	if r.Direction != nil {
		switch *r.Direction {
		case pb.Direction_DIRECTION_INFLOW:
			cmd.Direction = domain.Inflow
		case pb.Direction_DIRECTION_OUTFLOW:
			cmd.Direction = domain.Outflow
		case pb.Direction_DIRECTION_UNSPECIFIED:
			return nil, catalogError(ledger.ErrInvalid)
		default:
			return nil, catalogError(ledger.ErrInvalid)
		}
	}
	if r.CategoryId != nil {
		cmd.CategoryID, err = catalogID(*r.CategoryId)
		if err != nil {
			return nil, catalogError(err)
		}
	}
	result, err := operations.ListTransactions(ctx, cmd)
	if err != nil {
		return nil, catalogError(err)
	}
	out := &pb.ListTransactionsResponse{Items: []*pb.Transaction{}, Pagination: &pb.PageResponse{NextCursor: result.Cursor}}
	for _, item := range result.Items {
		out.Items = append(out.Items, encodeStoredTransaction(item))
	}
	return out, nil
}
func (h *handler) ListCategories(ctx context.Context, r *pb.ListCategoriesRequest) (*pb.ListCategoriesResponse, error) {
	if r == nil || r.Context == nil {
		return nil, catalogError(ledger.ErrInvalid)
	}
	companyID, err := catalogID(r.CompanyId)
	if err != nil {
		return nil, catalogError(err)
	}
	grant, err := h.auth.AuthorizeCompany(ctx, &authpb.AuthorizeCompanyRequest{CompanyId: r.CompanyId, Permission: authpb.CompanyPermission_COMPANY_PERMISSION_FINANCIAL_READ, Context: &authpb.AuthorizeCompanyRequest_Browser{Browser: r.Context}})
	if err != nil {
		return nil, catalogError(err)
	}
	if grant == nil || grant.ActorId == "" {
		return nil, catalogError(ledger.ErrUnavailable)
	}
	profile, err := h.companies.ReadProfile(ctx, companyID)
	if err != nil {
		return nil, catalogError(err)
	}
	operations, ok := h.accounts.(catalogueOperations)
	if !ok {
		return nil, catalogError(ledger.ErrUnavailable)
	}
	limit, cursor, err := catalogPage(r.Page)
	if err != nil {
		return nil, catalogError(err)
	}
	result, err := operations.ListCategories(ctx, ledger.ListCategoriesCommand{Scope: domain.Scope{CompanyID: companyID, AccountID: profile.AccountID}, ActorID: grant.ActorId, Limit: limit, Cursor: cursor})
	if err != nil {
		return nil, catalogError(err)
	}
	out := &pb.ListCategoriesResponse{Items: []*pb.Category{}, Pagination: &pb.PageResponse{NextCursor: result.Cursor}}
	for _, c := range result.Items {
		direction := pb.Direction_DIRECTION_UNSPECIFIED
		switch c.Direction {
		case domain.Inflow:
			direction = pb.Direction_DIRECTION_INFLOW
		case domain.Outflow:
			direction = pb.Direction_DIRECTION_OUTFLOW
		case "":
		default:
			return nil, catalogError(ledger.ErrUnavailable)
		}
		out.Items = append(out.Items, &pb.Category{Id: c.ID.String(), Name: c.Name, Direction: direction, SemanticKind: c.SemanticKind})
	}
	return out, nil
}
func applyTransactionDates(cmd *ledger.ListTransactionsCommand, dates *pb.DateRange) error {
	if dates == nil {
		return nil
	}
	if dates.From != nil {
		cmd.From = domain.Date(*dates.From)
		if cmd.From.Validate() != nil {
			return ledger.ErrInvalid
		}
	}
	if dates.To != nil {
		cmd.Through = domain.Date(*dates.To)
		if cmd.Through.Validate() != nil {
			return ledger.ErrInvalid
		}
	}
	return nil
}
func (h *handler) UpdateTransactionCategory(ctx context.Context, r *pb.UpdateTransactionCategoryRequest) (*pb.UpdateTransactionCategoryResponse, error) {
	if r == nil || r.Context == nil {
		return nil, catalogError(ledger.ErrInvalid)
	}
	scope, err := catalogScope(r.Scope)
	if err != nil {
		return nil, catalogError(err)
	}
	actor, err := h.catalogAuthorization(ctx, scope, nil, r.Context)
	if err != nil {
		return nil, catalogError(err)
	}
	id, err := catalogID(r.TransactionId)
	if err != nil {
		return nil, catalogError(err)
	}
	cmd := ledger.UpdateTransactionCategoryCommand{Scope: scope, ActorID: actor, TransactionID: id, ExpectedVersion: r.ExpectedVersion}
	switch change := r.CategoryChange.(type) {
	case *pb.UpdateTransactionCategoryRequest_CategoryId:
		cmd.CategoryID, err = catalogID(change.CategoryId)
		if err != nil {
			return nil, catalogError(err)
		}
	case *pb.UpdateTransactionCategoryRequest_ClearCategory:
		if change.ClearCategory == nil {
			return nil, catalogError(ledger.ErrInvalid)
		}
	default:
		return nil, catalogError(ledger.ErrInvalid)
	}
	operations, ok := h.accounts.(catalogueOperations)
	if !ok {
		return nil, catalogError(ledger.ErrUnavailable)
	}
	result, err := operations.UpdateTransactionCategory(ctx, cmd)
	if err != nil {
		return nil, catalogError(err)
	}
	return &pb.UpdateTransactionCategoryResponse{Transaction: encodeStoredTransaction(result.Transaction), DataVersion: result.DataVersion}, nil
}
func encodeStoredTransaction(stored domain.StoredTransaction) *pb.Transaction {
	d := stored.Transaction.Data()
	direction := pb.Direction_DIRECTION_INFLOW
	if d.Direction == domain.Outflow {
		direction = pb.Direction_DIRECTION_OUTFLOW
	}
	out := &pb.Transaction{Id: d.ID.String(), AccountId: d.Scope.AccountID.String(), OperationDate: string(d.Date), Direction: direction, Amount: &pb.Money{AmountMinor: d.Amount.Minor(), Currency: d.Amount.Currency()}, CounterpartyName: d.CounterpartyName, PaymentPurpose: d.Purpose, Version: stored.Transaction.Version()}
	if stored.CreatedAt != nil {
		out.CreatedAt = timestamppb.New(*stored.CreatedAt)
	}
	if d.CounterpartyINN != "" {
		out.CounterpartyInn = &d.CounterpartyINN
	}
	if d.Category.ID != uuid.Nil {
		id := d.Category.ID.String()
		out.CategoryId = &id
	}
	source := &pb.RecordSource{Kind: pb.RecordSourceKind_RECORD_SOURCE_KIND_MANUAL, SourceSystem: "manual_v1"}
	if d.Source.Kind == domain.Imported {
		source.Kind = pb.RecordSourceKind_RECORD_SOURCE_KIND_FILE
		source.SourceSystem = d.Source.Namespace.System
		source.SourceCompanyId = &d.Source.Namespace.CompanyID
		source.ExternalOperationId = &d.Source.ExternalID
	}
	if stored.ImportID != uuid.Nil {
		id := stored.ImportID.String()
		source.ImportId = &id
	}
	out.Source = source
	return out
}
