package payments

import (
	"context"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
)

type lifecycleStore interface {
	SavePayment(context.Context, domain.ExpectedPayment, int64) error
	FindMatch(context.Context, ledger.Scope, uuid.UUID, uuid.UUID) (domain.PaymentMatch, error)
	RemoveMatch(context.Context, ledger.Scope, uuid.UUID) error
}

type UpdatePaymentCommand struct {
	Scope                   ledger.Scope
	PaymentID               uuid.UUID
	ExpectedVersion         int64
	Change                  domain.PaymentChange
	ActorID, IdempotencyKey string
}
type CancelPaymentCommand struct {
	Scope                   ledger.Scope
	PaymentID               uuid.UUID
	ExpectedVersion         int64
	ActorID, IdempotencyKey string
}
type RemoveMatchCommand struct {
	Scope                   ledger.Scope
	PaymentID, MatchID      uuid.UUID
	ExpectedVersion         int64
	ActorID, IdempotencyKey string
}
type ChangePaymentResult struct {
	Payment     domain.ExpectedPayment
	DataVersion int64
}

func (s *service) UpdatePayment(ctx context.Context, cmd UpdatePaymentCommand) (ChangePaymentResult, error) {
	return s.changePayment(ctx, cmd.Scope, cmd.PaymentID, cmd.ExpectedVersion, cmd.ActorID, cmd.IdempotencyKey, "update_payment", updateFingerprint(cmd), func(_ context.Context, _ lifecycleStore, p domain.ExpectedPayment) (domain.ExpectedPayment, error) {
		return p.Update(cmd.Change)
	})
}
func (s *service) CancelPayment(ctx context.Context, cmd CancelPaymentCommand) (ChangePaymentResult, error) {
	return s.changePayment(ctx, cmd.Scope, cmd.PaymentID, cmd.ExpectedVersion, cmd.ActorID, cmd.IdempotencyKey, "cancel_payment", cmd, func(_ context.Context, _ lifecycleStore, p domain.ExpectedPayment) (domain.ExpectedPayment, error) {
		return p.Cancel()
	})
}
func (s *service) RemovePaymentMatch(ctx context.Context, cmd RemoveMatchCommand) (ChangePaymentResult, error) {
	if cmd.MatchID == uuid.Nil {
		return ChangePaymentResult{}, ErrInvalid
	}
	return s.changePayment(ctx, cmd.Scope, cmd.PaymentID, cmd.ExpectedVersion, cmd.ActorID, cmd.IdempotencyKey, "remove_payment_match", cmd, func(ctx context.Context, st lifecycleStore, p domain.ExpectedPayment) (domain.ExpectedPayment, error) {
		m, err := st.FindMatch(ctx, cmd.Scope, cmd.PaymentID, cmd.MatchID)
		if err != nil {
			return domain.ExpectedPayment{}, err
		}
		changed, err := p.RemoveMatch(m)
		if err != nil {
			return domain.ExpectedPayment{}, err
		}
		if err := st.RemoveMatch(ctx, cmd.Scope, cmd.MatchID); err != nil {
			return domain.ExpectedPayment{}, err
		}
		return changed, nil
	})
}

func (s *service) changePayment(ctx context.Context, scope ledger.Scope, id uuid.UUID, version int64, actor, key, operation string, body any, change func(context.Context, lifecycleStore, domain.ExpectedPayment) (domain.ExpectedPayment, error)) (ChangePaymentResult, error) {
	if err := validateChangePayment(scope, id, version, actor); err != nil {
		return ChangePaymentResult{}, err
	}
	if s.withinTx == nil {
		return ChangePaymentResult{}, ErrUnavailable
	}
	var result ChangePaymentResult
	err := s.withinTx(ctx, func(ctx context.Context, st store, commands commandStore) error {
		// Receipts contain immutable payment state, so replay never reads a newer row.
		var saved changeReceipt
		replay, err := replayCommand(ctx, commands, scope, actor, operation, id.String(), key, body, &saved)
		if err != nil {
			return err
		}
		if replay {
			result, err = saved.result()
			return err
		}
		storage, ok := st.(lifecycleStore)
		if !ok {
			return ErrUnavailable
		}
		account, err := st.LoadAccount(ctx, scope)
		if err != nil {
			return err
		}
		payment, _, found, err := st.FindPayment(ctx, scope, id, ledger.Source{})
		if err != nil {
			return err
		}
		if !found {
			return ErrNotFound
		}
		if payment.Version() != version {
			return ErrStaleVersion
		}
		changed, err := change(ctx, storage, payment)
		if err != nil {
			return err
		}
		if changed.Data().ReconciliationGroupID != payment.Data().ReconciliationGroupID {
			if err := validateGroupLink(ctx, st, changed); err != nil {
				return err
			}
		}
		if err := storage.SavePayment(ctx, changed, version); err != nil {
			return err
		}
		previous := account.DataVersion()
		if err := account.AdvanceDataVersion(); err != nil {
			return err
		}
		if err := st.SaveAccount(ctx, account); err != nil {
			return err
		}
		if err := st.CarryHistoryCoverage(ctx, scope, previous, account.DataVersion()); err != nil {
			return err
		}
		result = ChangePaymentResult{Payment: changed, DataVersion: account.DataVersion()}
		return completeCommand(ctx, commands, scope, actor, operation, id.String(), key, newChangeReceipt(result))
	})
	if err != nil {
		return ChangePaymentResult{}, err
	}
	return result, nil
}

func updateFingerprint(cmd UpdatePaymentCommand) any {
	var amount *int64
	var currency string
	if cmd.Change.Amount != nil {
		n := cmd.Change.Amount.Minor()
		currency = cmd.Change.Amount.Currency()
		amount = &n
	}
	return struct {
		Command  UpdatePaymentCommand
		Amount   *int64
		Currency string
	}{cmd, amount, currency}
}

func validateChangePayment(scope ledger.Scope, id uuid.UUID, version int64, actor string) error {
	if scope.Validate() != nil || id == uuid.Nil || version < 1 || actor == "" {
		return ErrInvalid
	}
	return nil
}

func validateGroupLink(ctx context.Context, st any, p domain.ExpectedPayment) error {
	id := p.Data().ReconciliationGroupID
	if id == uuid.Nil {
		return nil
	}
	validator, ok := st.(interface {
		ValidatePaymentGroup(context.Context, ledger.Scope, uuid.UUID) error
	})
	if !ok {
		return ErrUnavailable
	}
	return validator.ValidatePaymentGroup(ctx, p.Data().Scope, id)
}
