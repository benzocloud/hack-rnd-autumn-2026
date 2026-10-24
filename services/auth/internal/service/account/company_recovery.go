package account

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/domain/account"
)

// ReconcileOne processes at most one durable claim. The bool means a claim was
// found, not that a company was created. No browser cookie participates in recovery.
func (s *service) ReconcileOne(ctx context.Context) (bool, error) {
	if s.core == nil {
		return false, ErrUnavailable
	}
	lease, err := uuid.NewRandom()
	if err != nil {
		return false, ErrUnavailable
	}
	var command domain.CompanyCreation
	err = s.withinTx(ctx, func(ctx context.Context, _ userStore, _ contextStore, _ challengeStore, _ consentStore, _ membershipStore, commands creationStore) error {
		var err error
		command, err = commands.ClaimCreation(ctx, lease)
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	terminal, err := s.prepareRecovery(ctx, command)
	if terminal || errors.Is(err, domain.ErrLeaseLost) {
		return true, nil
	}
	if err != nil {
		return true, s.retryCreation(ctx, command, err)
	}
	// Claim/preflight transactions have committed before any outgoing RPC.
	receipt, err := s.core.GetCreationResult(ctx, command.ID)
	if errors.Is(err, ErrNotFound) {
		err = s.core.BootstrapCompany(ctx, publicCreation(command))
		if err == nil {
			receipt, err = s.core.GetCreationResult(ctx, command.ID)
		}
	}
	if errors.Is(err, ErrReceiptMismatch) {
		receipt = CreationReceipt{}
		err = nil
	}
	if err != nil {
		return true, s.retryCreation(ctx, command, err)
	}
	_, err = s.finalizeCreation(ctx, command, receipt, lease)
	if errors.Is(err, ErrForbidden) || errors.Is(err, domain.ErrLeaseLost) {
		return true, nil
	}
	if err != nil {
		return true, s.retryCreation(ctx, command, err)
	}
	return true, nil
}

func (s *service) prepareRecovery(ctx context.Context, snapshot domain.CompanyCreation) (bool, error) {
	terminal := false
	err := s.withinTx(ctx, func(ctx context.Context, users userStore, _ contextStore, _ challengeStore, _ consentStore, _ membershipStore, commands creationStore) error {
		command, user, err := lockCreation(ctx, users, commands, snapshot.ID)
		if err != nil {
			return err
		}
		if command.State == domain.CreationCompleted || command.State == domain.CreationQuarantined {
			terminal = true
			return nil
		}
		if command.State != domain.CreationPrepared {
			return ErrUnavailable
		}
		if command.LeaseToken != snapshot.LeaseToken || command.LeaseExpiresAt == nil {
			return domain.ErrLeaseLost
		}
		if !s.creationInitiatorEnabled(user) {
			terminal = true
			return commands.QuarantineCreation(ctx, command.ID, snapshot.LeaseToken)
		}
		return nil
	})
	return terminal && err == nil, err
}

func (s *service) retryCreation(ctx context.Context, command domain.CompanyCreation, cause error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	err := s.withinTx(ctx, func(ctx context.Context, _ userStore, _ contextStore, _ challengeStore, _ consentStore, _ membershipStore, commands creationStore) error {
		return commands.RetryCreation(ctx, command.ID, command.LeaseToken, creationRetryDelay(command.Attempts))
	})
	if errors.Is(err, domain.ErrLeaseLost) {
		return nil
	}
	return errors.Join(cause, err)
}

func creationRetryDelay(attempt int64) time.Duration {
	delay := time.Second
	for n := int64(1); n < attempt && delay < time.Minute; n++ {
		delay *= 2
	}
	return min(delay, time.Minute)
}
