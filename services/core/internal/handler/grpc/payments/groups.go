package payments

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	forecast "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	service "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/payments"
)

type groupOperations interface {
	CreateReconciliationGroup(context.Context, service.CreateGroupCommand) (service.GroupResult, error)
	ReadReconciliationGroup(context.Context, ledger.Scope, uuid.UUID) (forecast.ReconciliationGroup, error)
	ListReconciliationGroups(context.Context, service.ListGroupsCommand) (service.ListGroupsResult, error)
	RevokeReconciliationGroup(context.Context, service.RevokeGroupCommand) (service.GroupResult, error)
}

func (h *handler) CreateReconciliationGroup(ctx context.Context, r *pb.CreateReconciliationGroupRequest) (*pb.CreateReconciliationGroupResponse, error) {
	if r == nil || r.Context == nil || !validKey(r.IdempotencyKey) {
		return nil, paymentError(service.ErrInvalid)
	}
	scope, actor, err := h.authorize(ctx, r.Scope, nil, r.Context)
	if err != nil {
		return nil, paymentError(err)
	}
	cmd := service.CreateGroupCommand{Scope: scope, ActorID: actor, IdempotencyKey: r.IdempotencyKey, Cutoff: ledger.Date(r.CutoffDate), HorizonStart: ledger.Date(r.HorizonStart), HorizonEnd: ledger.Date(r.HorizonEnd)}
	switch rec := r.Reconciliation.(type) {
	case *pb.CreateReconciliationGroupRequest_Additional:
		if rec.Additional == nil || !rec.Additional.Confirmed {
			return nil, paymentError(service.ErrInvalid)
		}
		cmd.Additional = true
	case *pb.CreateReconciliationGroupRequest_Replacement:
		if rec.Replacement == nil || !rec.Replacement.CompleteForHorizon || len(rec.Replacement.HistoricalOperations) == 0 || len(rec.Replacement.HistoricalOperations) > 20000 {
			return nil, paymentError(service.ErrInvalid)
		}
		for _, op := range rec.Replacement.HistoricalOperations {
			if op == nil || op.Version < 1 {
				return nil, paymentError(service.ErrInvalid)
			}
			id, err := parseID(op.TransactionId)
			if err != nil {
				return nil, paymentError(err)
			}
			cmd.Operations = append(cmd.Operations, forecast.OperationReference{ID: id, Version: op.Version})
		}
	default:
		return nil, paymentError(service.ErrInvalid)
	}
	operations, ok := h.payments.(groupOperations)
	if !ok {
		return nil, paymentError(service.ErrUnavailable)
	}
	result, err := operations.CreateReconciliationGroup(ctx, cmd)
	if err != nil {
		return nil, paymentError(err)
	}
	return &pb.CreateReconciliationGroupResponse{Group: encodeGroup(result.Group), DataVersion: result.DataVersion}, nil
}
func (h *handler) GetReconciliationGroup(ctx context.Context, r *pb.GetReconciliationGroupRequest) (*pb.GetReconciliationGroupResponse, error) {
	if r == nil || r.Context == nil {
		return nil, paymentError(service.ErrInvalid)
	}
	scope, _, err := h.authorize(ctx, r.Scope, r.Context, nil)
	if err != nil {
		return nil, paymentError(err)
	}
	id, err := parseID(r.GroupId)
	if err != nil {
		return nil, paymentError(err)
	}
	operations, ok := h.payments.(groupOperations)
	if !ok {
		return nil, paymentError(service.ErrUnavailable)
	}
	result, err := operations.ReadReconciliationGroup(ctx, scope, id)
	if err != nil {
		return nil, paymentError(err)
	}
	return &pb.GetReconciliationGroupResponse{Group: encodeGroup(result)}, nil
}
func (h *handler) ListReconciliationGroups(ctx context.Context, r *pb.ListReconciliationGroupsRequest) (*pb.ListReconciliationGroupsResponse, error) {
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
	operations, ok := h.payments.(groupOperations)
	if !ok {
		return nil, paymentError(service.ErrUnavailable)
	}
	result, err := operations.ListReconciliationGroups(ctx, service.ListGroupsCommand{Scope: scope, ActorID: actor, Limit: n, Cursor: cursor, IncludeRevoked: r.IncludeRevoked})
	if err != nil {
		return nil, paymentError(err)
	}
	out := &pb.ListReconciliationGroupsResponse{Items: []*pb.PaymentReconciliationGroup{}, Pagination: &pb.PageResponse{NextCursor: result.Cursor}}
	for _, g := range result.Items {
		out.Items = append(out.Items, encodeGroup(g))
	}
	return out, nil
}
func (h *handler) RevokeReconciliationGroup(ctx context.Context, r *pb.RevokeReconciliationGroupRequest) (*pb.RevokeReconciliationGroupResponse, error) {
	if r == nil || r.Context == nil || !validKey(r.IdempotencyKey) {
		return nil, paymentError(service.ErrInvalid)
	}
	scope, actor, err := h.authorize(ctx, r.Scope, nil, r.Context)
	if err != nil {
		return nil, paymentError(err)
	}
	id, err := parseID(r.GroupId)
	if err != nil {
		return nil, paymentError(err)
	}
	operations, ok := h.payments.(groupOperations)
	if !ok {
		return nil, paymentError(service.ErrUnavailable)
	}
	result, err := operations.RevokeReconciliationGroup(ctx, service.RevokeGroupCommand{Scope: scope, GroupID: id, ExpectedVersion: r.ExpectedVersion, ActorID: actor, IdempotencyKey: r.IdempotencyKey})
	if err != nil {
		return nil, paymentError(err)
	}
	return &pb.RevokeReconciliationGroupResponse{Group: encodeGroup(result.Group), DataVersion: result.DataVersion}, nil
}
func encodeGroup(g forecast.ReconciliationGroup) *pb.PaymentReconciliationGroup {
	status := pb.ReconciliationStatus_RECONCILIATION_STATUS_ACTIVE
	if g.Status == "revoked" {
		status = pb.ReconciliationStatus_RECONCILIATION_STATUS_REVOKED
	}
	out := &pb.PaymentReconciliationGroup{Id: g.ID.String(), Scope: &pb.AccountScope{CompanyId: g.Scope.CompanyID.String(), AccountId: g.Scope.AccountID.String()}, Version: g.Version, Status: status, CutoffDate: string(g.Cutoff), HorizonStart: string(g.HorizonStart), HorizonEnd: string(g.HorizonEnd), ConfirmedBy: g.ConfirmedBy, ConfirmedAt: timestamppb.New(g.ConfirmedAt)}
	if g.Additional {
		out.Reconciliation = &pb.PaymentReconciliationGroup_Additional{Additional: &pb.AdditionalObligationConfirmation{Confirmed: true}}
	} else {
		rec := &pb.CompleteReplacement{CompleteForHorizon: true, HistoricalOperations: []*pb.HistoricalOperationRef{}}
		for _, op := range g.Operations {
			rec.HistoricalOperations = append(rec.HistoricalOperations, &pb.HistoricalOperationRef{TransactionId: op.ID.String(), Version: op.Version})
		}
		out.Reconciliation = &pb.PaymentReconciliationGroup_Replacement{Replacement: rec}
	}
	return out
}
