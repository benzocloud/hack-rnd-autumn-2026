package core

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authpb "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/gen/auth/v1"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/service/account"
)

type client struct {
	rpc     pb.CompanyServiceClient
	timeout time.Duration
}

// New borrows an app-owned, verified mTLS connection. No transport retries.
func New(conn *grpc.ClientConn, timeout time.Duration) (*client, error) {
	if conn == nil || timeout <= 0 || timeout > 3*time.Second {
		return nil, errors.New("invalid Core client configuration")
	}
	return &client{rpc: pb.NewCompanyServiceClient(conn), timeout: timeout}, nil
}

func (c *client) GetCreationResult(ctx context.Context, id uuid.UUID) (account.CreationReceipt, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	response, err := c.rpc.GetCompanyCreationResult(ctx, &pb.GetCompanyCreationResultRequest{CommandId: id.String()}, grpc.MaxCallSendMsgSize(1<<20), grpc.MaxCallRecvMsgSize(2<<20))
	if err != nil {
		return account.CreationReceipt{}, dependencyError(err)
	}
	return decodeReceipt(response.GetReceipt(), id)
}

func (c *client) BootstrapCompany(ctx context.Context, command account.CreationCommand) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	spec := &authpb.CompanyCreateSpec{Name: command.Spec.Name, Currency: command.Spec.Currency, Timezone: command.Spec.Timezone}
	if command.Spec.INN != "" {
		spec.Inn = &command.Spec.INN
	}
	response, err := c.rpc.CreateCompanyBootstrap(ctx, &pb.CreateCompanyBootstrapRequest{CommandId: command.ID.String(), CompanyId: command.CompanyID.String(), RequestSha256: command.RequestSHA256, CompanySpec: spec}, grpc.MaxCallSendMsgSize(1<<20), grpc.MaxCallRecvMsgSize(2<<20))
	if err != nil {
		return dependencyError(err)
	}
	receipt, err := decodeReceipt(response.GetReceipt(), command.ID)
	if err != nil {
		return err
	}
	if receipt.CompanyID != command.CompanyID || receipt.Spec != command.Spec || receipt.RequestSHA256 != command.RequestSHA256 {
		return account.ErrReceiptMismatch
	}
	return nil
}

func decodeReceipt(r *pb.CompanyCreationReceipt, expected uuid.UUID) (account.CreationReceipt, error) {
	if r == nil || r.CompanySpec == nil || r.CommittedAt == nil || r.CommittedAt.CheckValid() != nil || (r.CompanySpec.Inn != nil && *r.CompanySpec.Inn == "") {
		return account.CreationReceipt{}, account.ErrReceiptMismatch
	}
	commandID, err := receiptID(r.CommandId, 4)
	if err != nil || commandID != expected {
		return account.CreationReceipt{}, account.ErrReceiptMismatch
	}
	companyID, err := receiptID(r.CompanyId, 7)
	if err != nil {
		return account.CreationReceipt{}, err
	}
	accountID, err := receiptID(r.DefaultAccountId, 7)
	if err != nil || accountID == companyID {
		return account.CreationReceipt{}, account.ErrReceiptMismatch
	}
	spec := account.CompanySpec{Name: r.CompanySpec.Name, INN: r.CompanySpec.GetInn(), Currency: r.CompanySpec.Currency, Timezone: r.CompanySpec.Timezone}
	hash, err := spec.Fingerprint()
	if err != nil || hash != r.RequestSha256 {
		return account.CreationReceipt{}, account.ErrReceiptMismatch
	}
	committed := r.CommittedAt.AsTime()
	if committed.IsZero() {
		return account.CreationReceipt{}, account.ErrReceiptMismatch
	}
	return account.CreationReceipt{CommandID: commandID, CompanyID: companyID, AccountID: accountID, Spec: spec, RequestSHA256: hash, CommittedAt: committed}, nil
}

func receiptID(value string, version uuid.Version) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil || id == uuid.Nil || id.Version() != version || id.String() != value {
		return uuid.Nil, account.ErrReceiptMismatch
	}
	return id, nil
}

type safeDependencyError struct{ kind, cause error }

func (e *safeDependencyError) Error() string   { return e.kind.Error() }
func (e *safeDependencyError) Unwrap() []error { return []error{e.kind, e.cause} }

func dependencyError(err error) error {
	kind := account.ErrUnavailable
	switch {
	case status.Code(err) == codes.Canceled:
		kind = context.Canceled
	case status.Code(err) == codes.DeadlineExceeded:
		kind = context.DeadlineExceeded
	default:
		for _, raw := range status.Convert(err).Details() {
			detail, ok := raw.(*pb.CoreErrorDetail)
			if !ok {
				continue
			}
			if status.Code(err) == codes.NotFound && detail.Reason == pb.CoreErrorReason_CORE_ERROR_REASON_NOT_FOUND {
				kind = account.ErrNotFound
			}
			if status.Code(err) == codes.PermissionDenied && detail.Reason == pb.CoreErrorReason_CORE_ERROR_REASON_FORBIDDEN {
				kind = account.ErrForbidden
			}
			if (status.Code(err) == codes.AlreadyExists || status.Code(err) == codes.Aborted) && detail.Reason == pb.CoreErrorReason_CORE_ERROR_REASON_IDEMPOTENCY_CONFLICT {
				kind = account.ErrReceiptMismatch
			}
		}
	}
	return &safeDependencyError{kind: kind, cause: err}
}
