package company

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/company"
)

func TestUpdateProfilePreservesReceipt(t *testing.T) {
	cmd := commandFixture()
	storage := &fakeStore{}
	s := New(func(ctx context.Context, work Work) error { return work(ctx, storage) }, func(_ context.Context, _, _ uuid.UUID, _ string) (Authorization, error) { return grant(cmd), nil })
	first, err := s.Bootstrap(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	name := "Новое имя"
	updated, err := s.Update(t.Context(), UpdateCommand{CompanyID: cmd.CompanyID, ExpectedVersion: 1, Name: &name, ClearINN: true})
	if err != nil || updated.Version != 2 || updated.Spec.Name != name || updated.Spec.INN != "" || updated.AccountID != first.AccountID {
		t.Fatal(updated, err)
	}
	result, err := s.GetBootstrapResult(t.Context(), cmd.CommandID)
	if err != nil || result != first {
		t.Fatal("creation receipt changed", result, err)
	}
	if _, err := s.Update(t.Context(), UpdateCommand{CompanyID: cmd.CompanyID, ExpectedVersion: 1, Name: &name}); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
}

func TestUpdateValidationAndCommit(t *testing.T) {
	cmd := commandFixture()
	name, empty := "New name", ""
	for label, input := range map[string]UpdateCommand{
		"empty":            {CompanyID: cmd.CompanyID, ExpectedVersion: 1},
		"empty name":       {CompanyID: cmd.CompanyID, ExpectedVersion: 1, Name: &empty},
		"empty inn":        {CompanyID: cmd.CompanyID, ExpectedVersion: 1, INN: &empty},
		"both inn changes": {CompanyID: cmd.CompanyID, ExpectedVersion: 1, INN: &name, ClearINN: true},
	} {
		t.Run(label, func(t *testing.T) {
			s := New(func(context.Context, Work) error { t.Error("invalid patch entered transaction"); return nil }, nil)
			if _, err := s.Update(t.Context(), input); !errors.Is(err, ErrInvalid) {
				t.Fatal(err)
			}
		})
	}
	profile, err := domain.NewProfile(cmd.CompanyID, uuid.New(), cmd.Spec, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, overflow := range []bool{false, true} {
		storage := &fakeStore{profile: profile}
		version := int64(1)
		if overflow {
			storage.profile.Version = math.MaxInt64
			version = math.MaxInt64
		}
		s := New(func(ctx context.Context, w Work) error {
			if err := w(ctx, storage); err != nil {
				return err
			}
			return ErrUnavailable
		}, nil)
		result, err := s.Update(t.Context(), UpdateCommand{CompanyID: cmd.CompanyID, ExpectedVersion: version, Name: &name})
		if err == nil || result != (domain.Profile{}) {
			t.Fatal("uncommitted/overflow result returned", result, err)
		}
	}
}
