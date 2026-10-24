package company

import (
	"context"
	"math"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/company"
)

type UpdateCommand struct {
	CompanyID       uuid.UUID
	ExpectedVersion int64
	Name, INN       *string
	ClearINN        bool
}

// Update requires prior Auth UPDATE with the original WriteContext at transport.
// Profile edits never rewrite the immutable creation receipt or financial version.
func (s *service) Update(ctx context.Context, cmd UpdateCommand) (domain.Profile, error) {
	if err := validateUpdate(cmd); err != nil {
		return domain.Profile{}, err
	}
	if s.withinTx == nil {
		return domain.Profile{}, ErrUnavailable
	}
	var result domain.Profile
	err := s.withinTx(ctx, func(ctx context.Context, storage store) error {
		profile, err := storage.ReadProfile(ctx, cmd.CompanyID)
		if err != nil {
			return err
		}
		if profile.ID != cmd.CompanyID || profile.Validate() != nil {
			return ErrUnavailable
		}
		if profile.Version != cmd.ExpectedVersion || profile.Version == math.MaxInt64 {
			return ErrConflict
		}
		spec := profile.Spec
		if cmd.Name != nil {
			spec.Name = *cmd.Name
		}
		if cmd.INN != nil {
			spec.INN = *cmd.INN
		} else if cmd.ClearINN {
			spec.INN = ""
		}
		updated, err := profile.Update(spec)
		if err != nil {
			return ErrInvalid
		}
		if err = storage.UpdateProfile(ctx, updated, cmd.ExpectedVersion); err != nil {
			return err
		}
		result = updated
		return nil
	})
	if err != nil {
		return domain.Profile{}, err
	}
	return result, nil
}

func validateUpdate(cmd UpdateCommand) error {
	if cmd.CompanyID == uuid.Nil || cmd.ExpectedVersion < 1 || (cmd.Name == nil && cmd.INN == nil && !cmd.ClearINN) || (cmd.INN != nil && cmd.ClearINN) {
		return ErrInvalid
	}
	if cmd.Name != nil && domain.ValidateName(*cmd.Name) != nil {
		return ErrInvalid
	}
	if cmd.INN != nil && (*cmd.INN == "" || domain.ValidateINN(*cmd.INN) != nil) {
		return ErrInvalid
	}
	return nil
}
