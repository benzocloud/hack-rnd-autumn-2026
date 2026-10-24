package company

import (
	"context"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/company"
)

// ReadProfile needs Auth READ at its future private transport; it does not look
// up membership or treat existence of an ID as authorization.
func (s *service) ReadProfile(ctx context.Context, id uuid.UUID) (domain.Profile, error) {
	if id == uuid.Nil {
		return domain.Profile{}, ErrInvalid
	}
	if s.withinTx == nil {
		return domain.Profile{}, ErrUnavailable
	}
	var result domain.Profile
	err := s.withinTx(ctx, func(ctx context.Context, storage store) error {
		var err error
		result, err = storage.ReadProfile(ctx, id)
		if err != nil {
			return err
		}
		if result.ID != id || result.Validate() != nil {
			return ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return domain.Profile{}, err
	}
	return result, nil
}

// GetBootstrapResult is only for a future authenticated Auth recovery caller.
func (s *service) GetBootstrapResult(ctx context.Context, id uuid.UUID) (BootstrapResult, error) {
	if id == uuid.Nil {
		return BootstrapResult{}, ErrInvalid
	}
	if s.withinTx == nil {
		return BootstrapResult{}, ErrUnavailable
	}
	var result BootstrapResult
	err := s.withinTx(ctx, func(ctx context.Context, storage store) error {
		record, err := storage.ReadBootstrap(ctx, id)
		if err != nil {
			return err
		}
		if record.CommandID != id || record.Validate() != nil {
			return ErrUnavailable
		}
		result = bootstrapResult(record)
		return nil
	})
	if err != nil {
		return BootstrapResult{}, err
	}
	return result, nil
}
