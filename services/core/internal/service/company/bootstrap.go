package company

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/company"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

type BootstrapCommand struct {
	CommandID, CompanyID uuid.UUID
	RequestSHA256        string
	Spec                 domain.Spec
}
type BootstrapResult struct {
	CommandID, CompanyID, AccountID uuid.UUID
	RequestSHA256                   string
	Spec                            domain.Spec
	CommittedAt                     time.Time
}

// Bootstrap is internal-only until the private mTLS/Auth transport exists.
// Every attempt, including replay, rechecks Auth before accessing storage.
func (s *service) Bootstrap(ctx context.Context, cmd BootstrapCommand) (BootstrapResult, error) {
	if err := validateBootstrap(cmd); err != nil {
		return BootstrapResult{}, err
	}
	if s.authorize == nil || s.withinTx == nil {
		return BootstrapResult{}, ErrUnavailable
	}
	grant, err := s.authorize(ctx, cmd.CommandID, cmd.CompanyID, cmd.RequestSHA256)
	if err != nil {
		return BootstrapResult{}, mapAuthorizationError(err)
	}
	if grant.CommandID != cmd.CommandID || grant.CompanyID != cmd.CompanyID || grant.RequestSHA256 != cmd.RequestSHA256 || grant.Spec != cmd.Spec {
		return BootstrapResult{}, ErrUnauthorized
	}
	accountID, err := uuid.NewV7()
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("generate default account id: %w", err)
	}
	profile, err := domain.NewProfile(cmd.CompanyID, accountID, cmd.Spec, time.Now())
	if err != nil {
		return BootstrapResult{}, err
	}
	account, err := ledger.NewBankAccount(ledger.Scope{CompanyID: cmd.CompanyID, AccountID: accountID})
	if err != nil {
		return BootstrapResult{}, err
	}
	record := domain.BootstrapRecord{CommandID: cmd.CommandID, CompanyID: cmd.CompanyID, AccountID: accountID, RequestSHA256: cmd.RequestSHA256, Spec: cmd.Spec, CommittedAt: profile.CreatedAt}
	var result BootstrapResult
	err = s.withinTx(ctx, func(ctx context.Context, storage store) error {
		stored, created, err := storage.ReserveBootstrap(ctx, record)
		if err != nil {
			return err
		}
		if err = stored.Validate(); err != nil {
			return ErrUnavailable
		}
		if stored.CommandID != cmd.CommandID || stored.CompanyID != cmd.CompanyID || stored.Spec != cmd.Spec || stored.RequestSHA256 != cmd.RequestSHA256 {
			return ErrConflict
		}
		if created {
			if stored != record {
				return ErrUnavailable
			}
			if err = storage.InsertCompany(ctx, profile.ID); err != nil {
				return err
			}
			if err = storage.InsertAccount(ctx, account); err != nil {
				return err
			}
			if err = storage.InsertProfile(ctx, profile); err != nil {
				return err
			}
		}
		result = bootstrapResult(stored)
		return nil
	})
	if err != nil {
		return BootstrapResult{}, err
	}
	return result, nil
}
func validateBootstrap(cmd BootstrapCommand) error {
	if cmd.CommandID == uuid.Nil || cmd.CompanyID == uuid.Nil || cmd.CompanyID.Version() != 7 {
		return ErrInvalid
	}
	expected, err := domain.Fingerprint(cmd.Spec)
	if err != nil || expected != cmd.RequestSHA256 {
		return ErrInvalid
	}
	return nil
}
func bootstrapResult(record domain.BootstrapRecord) BootstrapResult {
	return BootstrapResult{CommandID: record.CommandID, CompanyID: record.CompanyID, AccountID: record.AccountID, RequestSHA256: record.RequestSHA256, Spec: record.Spec, CommittedAt: record.CommittedAt}
}
