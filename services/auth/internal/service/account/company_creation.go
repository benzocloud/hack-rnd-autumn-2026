package account

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/domain/account"
)

var (
	ErrIdempotencyConflict = domain.ErrConflict
	ErrCreationInProgress  = errors.New("company creation in progress")
	ErrReceiptMismatch     = errors.New("company creation receipt mismatch")
)

type creationCore interface {
	GetCreationResult(context.Context, uuid.UUID) (CreationReceipt, error)
	BootstrapCompany(context.Context, CreationCommand) error
}

type CompanySpec = domain.CompanySpec

// CreationCommand is a feature-local result; recovery metadata stays in storage.
type CreationCommand struct {
	ID, CompanyID, InitiatorID uuid.UUID
	Spec                       CompanySpec
	RequestSHA256, State       string
	CreatedAt                  time.Time
}

type CreationReceipt struct {
	CommandID, CompanyID, AccountID uuid.UUID
	Spec                            CompanySpec
	RequestSHA256                   string
	CommittedAt                     time.Time
}

type BeginCompanyCreationInput struct {
	Context WriteContext
	Key     string
	Spec    CompanySpec
}
type AuthorizeCompanyBootstrapInput struct {
	CommandID, CompanyID uuid.UUID
	RequestSHA256        string
}
type CompleteCompanyCreationInput struct {
	Context   WriteContext
	CommandID uuid.UUID
}

func (s *service) BeginCompanyCreation(ctx context.Context, input BeginCompanyCreationInput) (CreationCommand, error) {
	if err := validateBeginCompanyCreation(input); err != nil {
		return CreationCommand{}, err
	}
	spec, err := input.Spec.Normalize()
	if err != nil {
		return CreationCommand{}, ErrValidation
	}
	hash, err := spec.Fingerprint()
	if err != nil {
		return CreationCommand{}, ErrValidation
	}
	var result domain.CompanyCreation
	err = s.withinTx(ctx, func(ctx context.Context, users userStore, contexts contextStore, _ challengeStore, _ consentStore, _ membershipStore, commands creationStore) error {
		_, user, err := s.checkContext(ctx, users, contexts, input.Context, true, true)
		if err != nil {
			return err
		}
		if !s.emailVerified(user) {
			return ErrEmailUnverified
		}
		stored, err := commands.FindCreationByKey(ctx, user.ID, input.Key)
		if err == nil {
			if stored.Spec != spec || stored.RequestSHA256 != hash {
				return ErrIdempotencyConflict
			}
			result = stored
			return nil
		}
		if !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		id, err := uuid.NewRandom()
		if err != nil {
			return ErrUnavailable
		}
		companyID, err := uuid.NewV7()
		if err != nil {
			return ErrUnavailable
		}
		now := s.now().UTC().Truncate(time.Microsecond)
		result = domain.CompanyCreation{ID: id, CompanyID: companyID, InitiatorID: user.ID, Key: input.Key, Spec: spec, RequestSHA256: hash, State: domain.CreationPrepared, CreatedAt: now, UpdatedAt: now, NextAttemptAt: now}
		return commands.InsertCreation(ctx, result)
	})
	if err != nil {
		return CreationCommand{}, err
	}
	return publicCreation(result), nil
}

func validateBeginCompanyCreation(input BeginCompanyCreationInput) error {
	if len(input.Key) < 8 || len(input.Key) > 128 {
		return ErrValidation
	}
	for _, c := range input.Key {
		letter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
		digit := c >= '0' && c <= '9'
		separator := c == '.' || c == '_' || c == ':' || c == '-'
		if !letter && !digit && !separator {
			return ErrValidation
		}
	}
	if _, err := input.Spec.Normalize(); err != nil {
		return ErrValidation
	}
	return nil
}

func (s *service) AuthorizeCompanyBootstrap(ctx context.Context, input AuthorizeCompanyBootstrapInput) (CreationCommand, error) {
	if input.CommandID == uuid.Nil || input.CommandID.Version() != 4 || input.CompanyID == uuid.Nil || input.CompanyID.Version() != 7 || len(input.RequestSHA256) != 64 {
		return CreationCommand{}, ErrValidation
	}
	var result domain.CompanyCreation
	denied := false
	err := s.withinTx(ctx, func(ctx context.Context, users userStore, _ contextStore, _ challengeStore, _ consentStore, _ membershipStore, commands creationStore) error {
		command, user, err := lockCreation(ctx, users, commands, input.CommandID)
		if err != nil {
			return err
		}
		if command.CompanyID != input.CompanyID || command.RequestSHA256 != input.RequestSHA256 {
			return ErrForbidden
		}
		if !s.creationInitiatorEnabled(user) || command.State == domain.CreationQuarantined {
			denied = true
			if command.State == domain.CreationPrepared {
				return commands.QuarantineCreation(ctx, command.ID, uuid.Nil)
			}
			return nil
		}
		if command.State != domain.CreationPrepared && command.State != domain.CreationCompleted {
			return ErrUnavailable
		}
		result = command
		return nil
	})
	if err != nil {
		return CreationCommand{}, err
	}
	if denied {
		return CreationCommand{}, ErrForbidden
	}
	return publicCreation(result), nil
}

func (s *service) CompleteCompanyCreation(ctx context.Context, input CompleteCompanyCreationInput) (CreationCommand, error) {
	if input.CommandID == uuid.Nil || input.CommandID.Version() != 4 {
		return CreationCommand{}, ErrValidation
	}
	var snapshot domain.CompanyCreation
	err := s.withinTx(ctx, func(ctx context.Context, users userStore, contexts contextStore, _ challengeStore, _ consentStore, _ membershipStore, commands creationStore) error {
		_, user, err := s.checkContext(ctx, users, contexts, input.Context, true, true)
		if err != nil {
			return err
		}
		if !s.emailVerified(user) {
			return ErrEmailUnverified
		}
		snapshot, err = commands.FindCreation(ctx, input.CommandID, true)
		if err != nil {
			return err
		}
		if snapshot.InitiatorID != user.ID || snapshot.State == domain.CreationQuarantined {
			return ErrForbidden
		}
		return nil
	})
	if err != nil {
		return CreationCommand{}, err
	}
	if snapshot.State == domain.CreationCompleted {
		return publicCreation(snapshot), nil
	}
	if s.core == nil {
		return CreationCommand{}, ErrUnavailable
	}
	// The first transaction has committed. No SQL lock crosses the Core RPC.
	receipt, err := s.core.GetCreationResult(ctx, snapshot.ID)
	if errors.Is(err, ErrNotFound) {
		return CreationCommand{}, ErrCreationInProgress
	}
	if errors.Is(err, ErrReceiptMismatch) {
		return s.finalizeCreation(ctx, snapshot, CreationReceipt{}, uuid.Nil)
	}
	if err != nil {
		return CreationCommand{}, err
	}
	return s.finalizeCreation(ctx, snapshot, receipt, uuid.Nil)
}

// lockCreation keeps the same user→command order as browser session operations.
// Missing users remain observable because durable commands have no cascading FK.
func lockCreation(ctx context.Context, users userStore, commands creationStore, id uuid.UUID) (domain.CompanyCreation, domain.User, error) {
	peek, err := commands.FindCreation(ctx, id, false)
	if err != nil {
		return domain.CompanyCreation{}, domain.User{}, err
	}
	user, err := users.FindUserByID(ctx, peek.InitiatorID, true)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return domain.CompanyCreation{}, domain.User{}, err
	}
	command, err := commands.FindCreation(ctx, id, true)
	if err != nil {
		return domain.CompanyCreation{}, domain.User{}, err
	}
	if command.InitiatorID != peek.InitiatorID {
		return domain.CompanyCreation{}, domain.User{}, ErrUnavailable
	}
	return command, user, nil
}

func (s *service) creationInitiatorEnabled(user domain.User) bool {
	return user.ID != uuid.Nil && user.State == "active" && s.emailVerified(user)
}

func (s *service) finalizeCreation(ctx context.Context, snapshot domain.CompanyCreation, receipt CreationReceipt, lease uuid.UUID) (CreationCommand, error) {
	var result domain.CompanyCreation
	quarantined := false
	err := s.withinTx(ctx, func(ctx context.Context, users userStore, _ contextStore, _ challengeStore, _ consentStore, memberships membershipStore, commands creationStore) error {
		command, user, err := lockCreation(ctx, users, commands, snapshot.ID)
		if err != nil {
			return err
		}
		if command.State == domain.CreationCompleted {
			result = command
			return nil
		}
		if command.State == domain.CreationQuarantined {
			return ErrForbidden
		}
		if command.State != domain.CreationPrepared {
			return ErrUnavailable
		}
		// PostgreSQL owns lease expiry. The following SQL CAS also rolls back
		// InsertOwner if this token expired while its RPC was in flight.
		if lease != uuid.Nil && (command.LeaseToken != lease || command.LeaseExpiresAt == nil) {
			return domain.ErrLeaseLost
		}
		if !s.creationInitiatorEnabled(user) || !matchingCreationReceipt(command, receipt) {
			quarantined = true
			return commands.QuarantineCreation(ctx, command.ID, lease)
		}
		if err := memberships.InsertOwner(ctx, command.CompanyID, command.InitiatorID, s.now().UTC()); err != nil {
			return err
		}
		if err := commands.CompleteCreation(ctx, command.ID, receipt.AccountID, receipt.CommittedAt, lease); err != nil {
			return err
		}
		command.State = domain.CreationCompleted
		command.AccountID = receipt.AccountID
		command.CommittedAt = &receipt.CommittedAt
		result = command
		return nil
	})
	if err != nil {
		return CreationCommand{}, err
	}
	if quarantined {
		return CreationCommand{}, ErrForbidden
	}
	return publicCreation(result), nil
}

func matchingCreationReceipt(command domain.CompanyCreation, receipt CreationReceipt) bool {
	if receipt.CommandID != command.ID || receipt.CompanyID != command.CompanyID || receipt.Spec != command.Spec || receipt.RequestSHA256 != command.RequestSHA256 || receipt.AccountID == uuid.Nil || receipt.AccountID.Version() != 7 || receipt.AccountID == receipt.CompanyID || receipt.CommittedAt.IsZero() {
		return false
	}
	hash, err := receipt.Spec.Fingerprint()
	return err == nil && hash == receipt.RequestSHA256
}

func publicCreation(command domain.CompanyCreation) CreationCommand {
	return CreationCommand{ID: command.ID, CompanyID: command.CompanyID, InitiatorID: command.InitiatorID, Spec: command.Spec, RequestSHA256: command.RequestSHA256, State: command.State, CreatedAt: command.CreatedAt}
}
