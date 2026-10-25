package payments

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
)

type CreateCommand struct {
	Record domain.PaymentData
	// Zero means no account CAS for manual creates; account locking still applies.
	ExpectedDataVersion int64
	ActorID             string
	IdempotencyKey      string
}

type CreatePaymentResult struct {
	PaymentID      uuid.UUID
	PaymentVersion int64
	DataVersion    int64
	Payment        domain.ExpectedPayment
}

type paymentStore interface {
	accountStore
	CarryHistoryCoverage(context.Context, ledger.Scope, int64, int64) error
	FindPayment(context.Context, ledger.Scope, uuid.UUID, ledger.Source) (domain.ExpectedPayment, int64, bool, error)
	InsertPayment(context.Context, domain.ExpectedPayment, int64) error
}

func (s *service) CreatePayment(ctx context.Context, cmd CreateCommand) (CreatePaymentResult, error) {
	if err := validateCreatePayment(cmd); err != nil {
		return CreatePaymentResult{}, err
	}
	data := cmd.Record
	if data.ID == uuid.Nil {
		id, err := uuid.NewV7()
		if err != nil {
			return CreatePaymentResult{}, fmt.Errorf("generate payment id: %w", err)
		}
		data.ID = id
	}
	if data.CreatedAt.IsZero() {
		data.CreatedAt = time.Now().UTC()
	}
	record, err := domain.NewExpectedPayment(data)
	if err != nil {
		return CreatePaymentResult{}, err
	}
	var result CreatePaymentResult
	txErr := s.withinTx(ctx, func(ctx context.Context, store store, commands commandStore) error {
		var saved paymentResult
		replay, err := replayCommand(ctx, commands, cmd.Record.Scope, cmd.ActorID, "create_payment", "", cmd.IdempotencyKey, paymentRequest(cmd), &saved)
		if err != nil {
			return err
		}
		if replay {
			if err := validatePaymentResult(saved); err != nil {
				return err
			}
			result = CreatePaymentResult{PaymentID: saved.ID, PaymentVersion: saved.Version, DataVersion: saved.DataVersion}
			if saved.Projection != nil {
				restored, err := saved.Projection.result()
				if err != nil {
					return err
				}
				if restored.Payment.Data().ID != saved.ID || restored.Payment.Data().Scope != cmd.Record.Scope || restored.Payment.Version() != saved.Version || restored.DataVersion != saved.DataVersion {
					return ErrUnavailable
				}
				result.Payment = restored.Payment
			}
			return nil
		}
		result, err = createPayment(ctx, store, cmd, record)
		if err != nil {
			return err
		}
		return completeCommand(ctx, commands, cmd.Record.Scope, cmd.ActorID, "create_payment", "", cmd.IdempotencyKey, paymentResult{ID: result.PaymentID, Version: result.PaymentVersion, DataVersion: result.DataVersion, Projection: paymentProjection(result)})
	})
	if txErr != nil {
		return CreatePaymentResult{}, txErr
	}
	return result, nil
}

func validateCreatePayment(cmd CreateCommand) error {
	if err := validateActorAndKey(cmd.ActorID, cmd.IdempotencyKey, cmd.Record.Source.Kind == ledger.Manual); err != nil {
		return err
	}
	if cmd.ExpectedDataVersion < 0 || (cmd.ExpectedDataVersion == 0 && cmd.Record.Source.Kind != ledger.Manual) {
		return ErrInvalid
	}
	return nil
}

func createPayment(ctx context.Context, store paymentStore, cmd CreateCommand, record domain.ExpectedPayment) (CreatePaymentResult, error) {
	account, err := store.LoadAccount(ctx, record.Data().Scope)
	if err != nil {
		return CreatePaymentResult{}, err
	}
	if record.Data().Source.Kind == ledger.Imported {
		stored, revision, found, err := store.FindPayment(ctx, record.Data().Scope, uuid.Nil, record.Data().Source)
		if err != nil {
			return CreatePaymentResult{}, err
		}
		if found {
			if _, err := record.CheckReplay(stored); err != nil {
				return CreatePaymentResult{}, err
			}
			return CreatePaymentResult{PaymentID: stored.Data().ID, PaymentVersion: stored.Version(), DataVersion: revision, Payment: stored}, nil
		}
	}
	if cmd.ExpectedDataVersion > 0 && account.DataVersion() != cmd.ExpectedDataVersion {
		return CreatePaymentResult{}, ErrStaleVersion
	}
	if record.Data().Source.Kind == ledger.Imported {
		if err := account.BindSupplemental(record.Data().Source.Namespace); err != nil {
			return CreatePaymentResult{}, err
		}
	}
	previousRevision := account.DataVersion()
	if err := account.AdvanceDataVersion(); err != nil {
		return CreatePaymentResult{}, err
	}
	if err := store.SaveAccount(ctx, account); err != nil {
		return CreatePaymentResult{}, err
	}
	if err := validateGroupLink(ctx, store, record); err != nil {
		return CreatePaymentResult{}, err
	}
	if err := store.InsertPayment(ctx, record, account.DataVersion()); err != nil {
		return CreatePaymentResult{}, err
	}
	if err := store.CarryHistoryCoverage(ctx, record.Data().Scope, previousRevision, account.DataVersion()); err != nil {
		return CreatePaymentResult{}, err
	}
	return CreatePaymentResult{PaymentID: record.Data().ID, PaymentVersion: record.Version(), DataVersion: account.DataVersion(), Payment: record}, nil
}
