package payments

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
)

type MatchCommand struct {
	Scope                                ledger.Scope
	ID, PaymentID, TransactionID         uuid.UUID
	Amount                               money.Money
	ExpectedVersion, ExpectedDataVersion int64
	// Required by the public match contract. Zero is retained only for internal
	// commands with a positive account CAS, including existing receipt replays.
	ExpectedTransactionVersion int64
	ActorID                    string
	IdempotencyKey             string
	At                         time.Time
}

type MatchPaymentResult struct {
	MatchID        uuid.UUID
	PaymentVersion int64
	DataVersion    int64
}

type matchStore interface {
	accountStore
	CarryHistoryCoverage(context.Context, ledger.Scope, int64, int64) error
	FindPayment(context.Context, ledger.Scope, uuid.UUID, ledger.Source) (domain.ExpectedPayment, int64, bool, error)
	FindTransaction(context.Context, ledger.Scope, uuid.UUID, ledger.Source) (ledger.Transaction, int64, bool, error)
	FactAllocated(context.Context, ledger.Scope, uuid.UUID) (money.Money, error)
	SaveMatch(context.Context, domain.ExpectedPayment, domain.PaymentMatch, int64) error
}

func (s *service) MatchPayment(ctx context.Context, cmd MatchCommand) (MatchPaymentResult, error) {
	if err := validateMatchPayment(cmd); err != nil {
		return MatchPaymentResult{}, err
	}
	id, at := cmd.ID, cmd.At
	if id == uuid.Nil {
		generated, err := uuid.NewV7()
		if err != nil {
			return MatchPaymentResult{}, fmt.Errorf("generate match id: %w", err)
		}
		id = generated
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	var result MatchPaymentResult
	txErr := s.withinTx(ctx, func(ctx context.Context, store store, commands commandStore) error {
		var saved matchResult
		replay, err := replayCommand(ctx, commands, cmd.Scope, cmd.ActorID, "match_payment", cmd.PaymentID.String(), cmd.IdempotencyKey, matchRequest(cmd), &saved)
		if err != nil {
			return err
		}
		if replay {
			if err := validateMatchResult(saved); err != nil {
				return err
			}
			result = MatchPaymentResult{MatchID: saved.ID, PaymentVersion: saved.PaymentVersion, DataVersion: saved.DataVersion}
			return nil
		}
		result, err = matchPayment(ctx, store, cmd, id, at)
		if err != nil {
			return err
		}
		return completeCommand(ctx, commands, cmd.Scope, cmd.ActorID, "match_payment", cmd.PaymentID.String(), cmd.IdempotencyKey, matchResult{ID: result.MatchID, PaymentVersion: result.PaymentVersion, DataVersion: result.DataVersion})
	})
	if txErr != nil {
		return MatchPaymentResult{}, txErr
	}
	return result, nil
}

func validateMatchPayment(cmd MatchCommand) error {
	if err := validateActorAndKey(cmd.ActorID, cmd.IdempotencyKey, true); err != nil {
		return err
	}
	if err := cmd.Scope.Validate(); err != nil {
		return err
	}
	if err := cmd.Amount.Positive(); err != nil {
		return err
	}
	if cmd.ExpectedVersion < 1 || cmd.ExpectedDataVersion < 0 || cmd.ExpectedTransactionVersion < 0 ||
		(cmd.ExpectedDataVersion == 0 && cmd.ExpectedTransactionVersion == 0) || cmd.PaymentID == uuid.Nil || cmd.TransactionID == uuid.Nil {
		return ErrInvalid
	}
	return nil
}

func matchPayment(ctx context.Context, store matchStore, cmd MatchCommand, id uuid.UUID, at time.Time) (MatchPaymentResult, error) {
	account, err := store.LoadAccount(ctx, cmd.Scope)
	if err != nil {
		return MatchPaymentResult{}, err
	}
	if cmd.ExpectedDataVersion > 0 && account.DataVersion() != cmd.ExpectedDataVersion {
		return MatchPaymentResult{}, ErrStaleVersion
	}
	payment, _, found, err := store.FindPayment(ctx, cmd.Scope, cmd.PaymentID, ledger.Source{})
	if err != nil {
		return MatchPaymentResult{}, err
	}
	if !found {
		return MatchPaymentResult{}, ErrNotFound
	}
	if payment.Version() != cmd.ExpectedVersion {
		return MatchPaymentResult{}, ErrStaleVersion
	}
	fact, _, found, err := store.FindTransaction(ctx, cmd.Scope, cmd.TransactionID, ledger.Source{})
	if err != nil {
		return MatchPaymentResult{}, err
	}
	if !found {
		return MatchPaymentResult{}, ErrNotFound
	}
	if cmd.ExpectedTransactionVersion > 0 && fact.Version() != cmd.ExpectedTransactionVersion {
		return MatchPaymentResult{}, ErrStaleVersion
	}
	allocated, err := store.FactAllocated(ctx, cmd.Scope, cmd.TransactionID)
	if err != nil {
		return MatchPaymentResult{}, err
	}
	updated, allocation, err := payment.Match(id, fact, allocated, cmd.Amount, at)
	if err != nil {
		return MatchPaymentResult{}, err
	}
	if err := store.SaveMatch(ctx, updated, allocation, cmd.ExpectedVersion); err != nil {
		return MatchPaymentResult{}, err
	}
	previousRevision := account.DataVersion()
	if err := account.AdvanceDataVersion(); err != nil {
		return MatchPaymentResult{}, err
	}
	if err := store.SaveAccount(ctx, account); err != nil {
		return MatchPaymentResult{}, err
	}
	if err := store.CarryHistoryCoverage(ctx, cmd.Scope, previousRevision, account.DataVersion()); err != nil {
		return MatchPaymentResult{}, err
	}
	return MatchPaymentResult{MatchID: allocation.ID(), PaymentVersion: updated.Version(), DataVersion: account.DataVersion()}, nil
}
