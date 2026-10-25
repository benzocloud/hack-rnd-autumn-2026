package payments

import (
	"context"
	"errors"
	"regexp"
	"unicode/utf8"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	company "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/company"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
	authpb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/auth/v1"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/handler/grpc/rpcerror"
	service "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/payments"
)

type operations interface {
	CreatePayment(context.Context, service.CreateCommand) (service.CreatePaymentResult, error)
	MatchPayment(context.Context, service.MatchCommand) (service.MatchPaymentResult, error)
	ReadPayment(context.Context, ledger.Scope, uuid.UUID) (domain.ExpectedPayment, error)
	ListPayments(context.Context, service.ListPaymentsCommand) (service.ListPaymentsResult, error)
	ListPaymentMatches(context.Context, service.ListMatchesCommand) (service.ListMatchesResult, error)
	UpdatePayment(context.Context, service.UpdatePaymentCommand) (service.ChangePaymentResult, error)
	CancelPayment(context.Context, service.CancelPaymentCommand) (service.ChangePaymentResult, error)
	RemovePaymentMatch(context.Context, service.RemoveMatchCommand) (service.ChangePaymentResult, error)
}
type companies interface {
	ReadProfile(context.Context, uuid.UUID) (company.Profile, error)
}
type authorizer interface {
	AuthorizeCompany(context.Context, *authpb.AuthorizeCompanyRequest) (*authpb.AuthorizeCompanyResponse, error)
}
type handler struct {
	pb.UnimplementedPaymentsServiceServer
	payments  operations
	companies companies
	auth      authorizer
}

func Register(server grpc.ServiceRegistrar, payments operations, companies companies, auth authorizer) {
	pb.RegisterPaymentsServiceServer(server, &handler{payments: payments, companies: companies, auth: auth})
}
func paymentError(err error) error {
	switch {
	case errors.Is(err, service.ErrInvalid), errors.Is(err, domain.ErrInvalid), errors.Is(err, ledger.ErrScope), errors.Is(err, ledger.ErrInvalid), errors.Is(err, money.ErrAmount), errors.Is(err, money.ErrCurrency):
		return rpcerror.New(codes.InvalidArgument, pb.CoreErrorReason_CORE_ERROR_REASON_VALIDATION_ERROR)
	case errors.Is(err, service.ErrCursor):
		return rpcerror.New(codes.InvalidArgument, pb.CoreErrorReason_CORE_ERROR_REASON_INVALID_CURSOR)
	case errors.Is(err, service.ErrNotFound):
		return rpcerror.New(codes.NotFound, pb.CoreErrorReason_CORE_ERROR_REASON_NOT_FOUND)
	case errors.Is(err, service.ErrStaleVersion), errors.Is(err, service.ErrConflict):
		return rpcerror.New(codes.Aborted, pb.CoreErrorReason_CORE_ERROR_REASON_VERSION_CONFLICT)
	case errors.Is(err, service.ErrIdempotencyConflict):
		return rpcerror.New(codes.AlreadyExists, pb.CoreErrorReason_CORE_ERROR_REASON_IDEMPOTENCY_CONFLICT)
	case errors.Is(err, domain.ErrState), errors.Is(err, domain.ErrAllocation), errors.Is(err, domain.ErrDirection):
		return rpcerror.New(codes.FailedPrecondition, pb.CoreErrorReason_CORE_ERROR_REASON_PAYMENT_STATE_CONFLICT)
	case errors.Is(err, service.ErrUnavailable):
		return rpcerror.New(codes.Unavailable, pb.CoreErrorReason_CORE_ERROR_REASON_DEPENDENCY_UNAVAILABLE)
	default:
		return rpcerror.From(err)
	}
}
func parseID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil || id.String() != raw {
		return uuid.Nil, service.ErrInvalid
	}
	return id, nil
}
func parseScope(raw *pb.AccountScope) (ledger.Scope, error) {
	if raw == nil {
		return ledger.Scope{}, service.ErrInvalid
	}
	c, err := parseID(raw.CompanyId)
	if err != nil {
		return ledger.Scope{}, err
	}
	a, err := parseID(raw.AccountId)
	return ledger.Scope{CompanyID: c, AccountID: a}, err
}
func (h *handler) authorize(ctx context.Context, raw *pb.AccountScope, browser *authpb.BrowserContext, write *authpb.WriteContext) (ledger.Scope, string, error) {
	scope, err := parseScope(raw)
	if err != nil {
		return scope, "", err
	}
	request := &authpb.AuthorizeCompanyRequest{CompanyId: raw.CompanyId, Permission: authpb.CompanyPermission_COMPANY_PERMISSION_FINANCIAL_READ, Context: &authpb.AuthorizeCompanyRequest_Browser{Browser: browser}}
	if write != nil {
		request.Permission = authpb.CompanyPermission_COMPANY_PERMISSION_FINANCIAL_WRITE
		request.Context = &authpb.AuthorizeCompanyRequest_Write{Write: write}
	}
	grant, err := h.auth.AuthorizeCompany(ctx, request)
	if err != nil {
		return scope, "", err
	}
	if grant == nil || grant.ActorId == "" {
		return scope, "", service.ErrUnavailable
	}
	profile, err := h.companies.ReadProfile(ctx, scope.CompanyID)
	if err != nil {
		return scope, "", err
	}
	if profile.AccountID != scope.AccountID {
		return scope, "", service.ErrNotFound
	}
	return scope, grant.ActorId, nil
}
func parsePage(p *pb.PageRequest) (int, string, error) {
	if p == nil {
		return 50, "", nil
	}
	n := 50
	if p.Limit != nil {
		n = int(*p.Limit)
	}
	if n < 1 || n > 100 {
		return 0, "", service.ErrInvalid
	}
	return n, p.Cursor, nil
}
func parseDirection(d pb.Direction) (ledger.Direction, error) {
	switch d {
	case pb.Direction_DIRECTION_UNSPECIFIED:
		return "", service.ErrInvalid
	case pb.Direction_DIRECTION_INFLOW:
		return ledger.Inflow, nil
	case pb.Direction_DIRECTION_OUTFLOW:
		return ledger.Outflow, nil
	default:
		return "", service.ErrInvalid
	}
}
func parseMoney(m *pb.Money) (money.Money, error) {
	if m == nil {
		return money.Money{}, service.ErrInvalid
	}
	amount, err := money.New(m.AmountMinor, m.Currency)
	if err != nil || amount.Positive() != nil {
		return money.Money{}, service.ErrInvalid
	}
	return amount, nil
}
func validText(s string, n int) bool { return utf8.ValidString(s) && utf8.RuneCountInString(s) <= n }
func (h *handler) GetPayment(ctx context.Context, r *pb.GetPaymentRequest) (*pb.GetPaymentResponse, error) {
	if r == nil || r.Context == nil {
		return nil, paymentError(service.ErrInvalid)
	}
	scope, _, err := h.authorize(ctx, r.Scope, r.Context, nil)
	if err != nil {
		return nil, paymentError(err)
	}
	id, err := parseID(r.PaymentId)
	if err != nil {
		return nil, paymentError(err)
	}
	result, err := h.payments.ReadPayment(ctx, scope, id)
	if err != nil {
		return nil, paymentError(err)
	}
	return &pb.GetPaymentResponse{Payment: encodePayment(result)}, nil
}
func (h *handler) ListPayments(ctx context.Context, r *pb.ListPaymentsRequest) (*pb.ListPaymentsResponse, error) {
	if r == nil || r.Context == nil {
		return nil, paymentError(service.ErrInvalid)
	}
	scope, actor, err := h.authorize(ctx, r.Scope, r.Context, nil)
	if err != nil {
		return nil, paymentError(err)
	}
	n, cursor, err := parsePage(r.Page)
	if err != nil {
		return nil, paymentError(err)
	}
	cmd := service.ListPaymentsCommand{Scope: scope, ActorID: actor, Limit: n, Cursor: cursor}
	if err := applyDates(&cmd, r.Dates); err != nil {
		return nil, paymentError(err)
	}
	if r.Status != nil {
		var ok bool
		cmd.Status, ok = statusFromProto(*r.Status)
		if !ok {
			return nil, paymentError(service.ErrInvalid)
		}
	}
	result, err := h.payments.ListPayments(ctx, cmd)
	if err != nil {
		return nil, paymentError(err)
	}
	out := &pb.ListPaymentsResponse{Items: []*pb.ExpectedPayment{}, Pagination: &pb.PageResponse{NextCursor: result.Cursor}}
	for _, p := range result.Items {
		out.Items = append(out.Items, encodePayment(p))
	}
	return out, nil
}
func (h *handler) CreateManualPayment(ctx context.Context, r *pb.CreateManualPaymentRequest) (*pb.CreateManualPaymentResponse, error) {
	if r == nil || r.Context == nil || !validText(r.CounterpartyName, 500) || !validText(r.Description, 2000) || !validKey(r.IdempotencyKey) || ledger.Date(r.ScheduledDate).Validate() != nil {
		return nil, paymentError(service.ErrInvalid)
	}
	scope, actor, err := h.authorize(ctx, r.Scope, nil, r.Context)
	if err != nil {
		return nil, paymentError(err)
	}
	direction, err := parseDirection(r.Direction)
	if err != nil {
		return nil, paymentError(err)
	}
	amount, err := parseMoney(r.Amount)
	if err != nil {
		return nil, paymentError(err)
	}
	data := domain.PaymentData{Scope: scope, Direction: direction, Amount: amount, ScheduledDate: ledger.Date(r.ScheduledDate), CounterpartyName: r.CounterpartyName, Description: r.Description, Source: ledger.Source{Kind: ledger.Manual}}
	if r.ReconciliationGroupId != nil {
		id, err := parseID(*r.ReconciliationGroupId)
		if err != nil {
			return nil, paymentError(err)
		}
		data.ReconciliationGroupID = id
	}

	if r.CategoryId != nil {
		id, err := parseID(*r.CategoryId)
		if err != nil {
			return nil, paymentError(err)
		}
		data.Category = ledger.Category{ID: id, CompanyID: scope.CompanyID}
	}
	result, err := h.payments.CreatePayment(ctx, service.CreateCommand{Record: data, ActorID: actor, IdempotencyKey: r.IdempotencyKey})
	if err != nil {
		return nil, paymentError(err)
	}
	if result.Payment.Version() < 1 {
		return nil, paymentError(service.ErrUnavailable)
	}
	return &pb.CreateManualPaymentResponse{Payment: encodePayment(result.Payment), DataVersion: result.DataVersion}, nil
}
func (h *handler) UpdatePayment(ctx context.Context, r *pb.UpdatePaymentRequest) (*pb.UpdatePaymentResponse, error) {
	if r == nil || r.Context == nil || r.CounterpartyName != nil && !validText(*r.CounterpartyName, 500) || r.Description != nil && !validText(*r.Description, 2000) {
		return nil, paymentError(service.ErrInvalid)
	}
	scope, actor, err := h.authorize(ctx, r.Scope, nil, r.Context)
	if err != nil {
		return nil, paymentError(err)
	}
	id, err := parseID(r.PaymentId)
	if err != nil {
		return nil, paymentError(err)
	}
	change := domain.PaymentChange{CounterpartyName: r.CounterpartyName, Description: r.Description}
	if r.Direction != nil {
		d, err := parseDirection(*r.Direction)
		if err != nil {
			return nil, paymentError(err)
		}
		change.Direction = &d
	}
	if r.Amount != nil {
		m, err := parseMoney(r.Amount)
		if err != nil {
			return nil, paymentError(err)
		}
		change.Amount = &m
	}
	if r.ScheduledDate != nil {
		d := ledger.Date(*r.ScheduledDate)
		if d.Validate() != nil {
			return nil, paymentError(service.ErrInvalid)
		}
		change.ScheduledDate = &d
	}
	switch c := r.CategoryChange.(type) {
	case nil:
	case *pb.UpdatePaymentRequest_CategoryId:
		id, err := parseID(c.CategoryId)
		if err != nil {
			return nil, paymentError(err)
		}
		change.Category = &ledger.Category{ID: id, CompanyID: scope.CompanyID}
	case *pb.UpdatePaymentRequest_ClearCategory:
		if c.ClearCategory == nil {
			return nil, paymentError(service.ErrInvalid)
		}
		change.Category = &ledger.Category{}
	default:
		return nil, paymentError(service.ErrInvalid)
	}
	switch c := r.ReconciliationChange.(type) {
	case nil:
	case *pb.UpdatePaymentRequest_ReconciliationGroupId:
		id, err := parseID(c.ReconciliationGroupId)
		if err != nil {
			return nil, paymentError(err)
		}
		change.ReconciliationGroupID = &id
	case *pb.UpdatePaymentRequest_ClearReconciliationGroup:
		if c.ClearReconciliationGroup == nil {
			return nil, paymentError(service.ErrInvalid)
		}
		id := uuid.Nil
		change.ReconciliationGroupID = &id
	default:
		return nil, paymentError(service.ErrInvalid)
	}

	result, err := h.payments.UpdatePayment(ctx, service.UpdatePaymentCommand{Scope: scope, PaymentID: id, ExpectedVersion: r.ExpectedVersion, Change: change, ActorID: actor})
	if err != nil {
		return nil, paymentError(err)
	}
	return &pb.UpdatePaymentResponse{Payment: encodePayment(result.Payment), DataVersion: result.DataVersion}, nil
}
func (h *handler) CancelPayment(ctx context.Context, r *pb.CancelPaymentRequest) (*pb.CancelPaymentResponse, error) {
	if r == nil || r.Context == nil {
		return nil, paymentError(service.ErrInvalid)
	}
	scope, actor, err := h.authorize(ctx, r.Scope, nil, r.Context)
	if err != nil {
		return nil, paymentError(err)
	}
	id, err := parseID(r.PaymentId)
	if err != nil {
		return nil, paymentError(err)
	}
	result, err := h.payments.CancelPayment(ctx, service.CancelPaymentCommand{Scope: scope, PaymentID: id, ExpectedVersion: r.ExpectedVersion, ActorID: actor})
	if err != nil {
		return nil, paymentError(err)
	}
	return &pb.CancelPaymentResponse{Payment: encodePayment(result.Payment), DataVersion: result.DataVersion}, nil
}
func (h *handler) MatchPayment(ctx context.Context, r *pb.MatchPaymentRequest) (*pb.MatchPaymentResponse, error) {
	if r == nil || r.Context == nil || !validKey(r.IdempotencyKey) || r.ExpectedTransactionVersion < 1 {
		return nil, paymentError(service.ErrInvalid)
	}
	scope, actor, err := h.authorize(ctx, r.Scope, nil, r.Context)
	if err != nil {
		return nil, paymentError(err)
	}
	payment, err := parseID(r.PaymentId)
	if err != nil {
		return nil, paymentError(err)
	}
	transaction, err := parseID(r.TransactionId)
	if err != nil {
		return nil, paymentError(err)
	}
	amount, err := parseMoney(r.Amount)
	if err != nil {
		return nil, paymentError(err)
	}
	result, err := h.payments.MatchPayment(ctx, service.MatchCommand{Scope: scope, PaymentID: payment, TransactionID: transaction, ExpectedVersion: r.ExpectedPaymentVersion, ExpectedTransactionVersion: r.ExpectedTransactionVersion, Amount: amount, ActorID: actor, IdempotencyKey: r.IdempotencyKey})
	if err != nil {
		return nil, paymentError(err)
	}
	if result.Match.ID() == uuid.Nil {
		return nil, paymentError(service.ErrUnavailable)
	}
	return &pb.MatchPaymentResponse{Match: encodeMatch(result.Match), PaymentVersion: result.PaymentVersion, DataVersion: result.DataVersion}, nil
}
func (h *handler) ListPaymentMatches(ctx context.Context, r *pb.ListPaymentMatchesRequest) (*pb.ListPaymentMatchesResponse, error) {
	if r == nil || r.Context == nil {
		return nil, paymentError(service.ErrInvalid)
	}
	scope, actor, err := h.authorize(ctx, r.Scope, r.Context, nil)
	if err != nil {
		return nil, paymentError(err)
	}
	id, err := parseID(r.PaymentId)
	if err != nil {
		return nil, paymentError(err)
	}
	n, cursor, err := parsePage(r.Page)
	if err != nil {
		return nil, paymentError(err)
	}
	result, err := h.payments.ListPaymentMatches(ctx, service.ListMatchesCommand{Scope: scope, PaymentID: id, ActorID: actor, Limit: n, Cursor: cursor})
	if err != nil {
		return nil, paymentError(err)
	}
	out := &pb.ListPaymentMatchesResponse{Items: []*pb.PaymentMatch{}, Pagination: &pb.PageResponse{NextCursor: result.Cursor}}
	for _, m := range result.Items {
		out.Items = append(out.Items, encodeMatch(m))
	}
	return out, nil
}
func (h *handler) RemovePaymentMatch(ctx context.Context, r *pb.RemovePaymentMatchRequest) (*pb.RemovePaymentMatchResponse, error) {
	if r == nil || r.Context == nil {
		return nil, paymentError(service.ErrInvalid)
	}
	scope, actor, err := h.authorize(ctx, r.Scope, nil, r.Context)
	if err != nil {
		return nil, paymentError(err)
	}
	id, err := parseID(r.PaymentId)
	if err != nil {
		return nil, paymentError(err)
	}
	match, err := parseID(r.MatchId)
	if err != nil {
		return nil, paymentError(err)
	}
	result, err := h.payments.RemovePaymentMatch(ctx, service.RemoveMatchCommand{Scope: scope, PaymentID: id, MatchID: match, ExpectedVersion: r.ExpectedPaymentVersion, ActorID: actor})
	if err != nil {
		return nil, paymentError(err)
	}
	return &pb.RemovePaymentMatchResponse{Payment: encodePayment(result.Payment), DataVersion: result.DataVersion}, nil
}
func encodePayment(p domain.ExpectedPayment) *pb.ExpectedPayment {
	d := p.Data()
	direction := pb.Direction_DIRECTION_INFLOW
	if d.Direction == ledger.Outflow {
		direction = pb.Direction_DIRECTION_OUTFLOW
	}
	status := pb.PaymentStatus_PAYMENT_STATUS_UNSPECIFIED
	switch p.Status() {
	case domain.Planned:
		status = pb.PaymentStatus_PAYMENT_STATUS_PLANNED
	case domain.PartiallyMatched:
		status = pb.PaymentStatus_PAYMENT_STATUS_PARTIALLY_MATCHED
	case domain.Matched:
		status = pb.PaymentStatus_PAYMENT_STATUS_MATCHED
	case domain.Cancelled:
		status = pb.PaymentStatus_PAYMENT_STATUS_CANCELLED
	}
	source := &pb.RecordSource{Kind: pb.RecordSourceKind_RECORD_SOURCE_KIND_MANUAL, SourceSystem: "manual_v1"}
	if d.Source.Kind == ledger.Imported {
		source.Kind = pb.RecordSourceKind_RECORD_SOURCE_KIND_FILE
		source.SourceSystem = d.Source.Namespace.System
		source.SourceCompanyId = &d.Source.Namespace.CompanyID
		source.ExternalPaymentId = &d.Source.ExternalID
	}
	out := &pb.ExpectedPayment{Id: d.ID.String(), AccountId: d.Scope.AccountID.String(), Direction: direction, Amount: &pb.Money{AmountMinor: d.Amount.Minor(), Currency: d.Amount.Currency()}, RemainingAmount: &pb.Money{AmountMinor: p.Remaining().Minor(), Currency: p.Remaining().Currency()}, ScheduledDate: string(d.ScheduledDate), CounterpartyName: d.CounterpartyName, Description: d.Description, Status: status, Source: source, Version: p.Version(), CreatedAt: timestamppb.New(d.CreatedAt)}
	if d.Category.ID != uuid.Nil {
		id := d.Category.ID.String()
		out.CategoryId = &id
	}
	if d.ReconciliationGroupID != uuid.Nil {
		id := d.ReconciliationGroupID.String()
		out.ReconciliationGroupId = &id
	}
	return out
}
func encodeMatch(m domain.PaymentMatch) *pb.PaymentMatch {
	return &pb.PaymentMatch{Id: m.ID().String(), PaymentId: m.PaymentID().String(), TransactionId: m.TransactionID().String(), Amount: &pb.Money{AmountMinor: m.Amount().Minor(), Currency: m.Amount().Currency()}, CreatedAt: timestamppb.New(m.CreatedAt()), RemovedAt: matchRemovedAt(m)}
}
func statusFromProto(s pb.PaymentStatus) (domain.Status, bool) {
	switch s {
	case pb.PaymentStatus_PAYMENT_STATUS_UNSPECIFIED:
		return "", false
	case pb.PaymentStatus_PAYMENT_STATUS_PLANNED:
		return domain.Planned, true
	case pb.PaymentStatus_PAYMENT_STATUS_PARTIALLY_MATCHED:
		return domain.PartiallyMatched, true
	case pb.PaymentStatus_PAYMENT_STATUS_MATCHED:
		return domain.Matched, true
	case pb.PaymentStatus_PAYMENT_STATUS_CANCELLED:
		return domain.Cancelled, true
	default:
		return "", false
	}
}

var keyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,128}$`)

func validKey(key string) bool { return keyPattern.MatchString(key) }
func applyDates(cmd *service.ListPaymentsCommand, dates *pb.DateRange) error {
	if dates == nil {
		return nil
	}
	if dates.From != nil {
		cmd.From = ledger.Date(*dates.From)
		if cmd.From.Validate() != nil {
			return service.ErrInvalid
		}
	}
	if dates.To != nil {
		cmd.Through = ledger.Date(*dates.To)
		if cmd.Through.Validate() != nil {
			return service.ErrInvalid
		}
	}
	return nil
}
func matchRemovedAt(m domain.PaymentMatch) *timestamppb.Timestamp {
	if at := m.RemovedAt(); at != nil {
		return timestamppb.New(*at)
	}
	return nil
}
