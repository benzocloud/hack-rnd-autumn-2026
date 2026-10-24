package company

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/company"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

type fakeStore struct {
	receipt        domain.BootstrapRecord
	profile        domain.Profile
	created        bool
	inserts, reads int
	err            error
}

func (s *fakeStore) ReserveBootstrap(_ context.Context, r domain.BootstrapRecord) (domain.BootstrapRecord, bool, error) {
	if s.err != nil {
		return domain.BootstrapRecord{}, false, s.err
	}
	if s.receipt.CommandID != uuid.Nil {
		return s.receipt, false, nil
	}
	s.receipt = r
	s.created = true
	return r, true, nil
}
func (s *fakeStore) InsertCompany(_ context.Context, _ uuid.UUID) error           { return s.err }
func (s *fakeStore) InsertAccount(_ context.Context, _ *ledger.BankAccount) error { return s.err }
func (s *fakeStore) InsertProfile(_ context.Context, p domain.Profile) error {
	s.inserts++
	s.profile = p
	return s.err
}
func (s *fakeStore) ReadProfile(context.Context, uuid.UUID) (domain.Profile, error) {
	s.reads++
	return s.profile, s.err
}
func (s *fakeStore) ReadBootstrap(context.Context, uuid.UUID) (domain.BootstrapRecord, error) {
	s.reads++
	return s.receipt, s.err
}

func (s *fakeStore) UpdateProfile(_ context.Context, profile domain.Profile, version int64) error {
	if s.err != nil {
		return s.err
	}
	if s.profile.Version != version {
		return ErrConflict
	}
	s.profile = profile
	return nil
}
func commandFixture() BootstrapCommand {
	spec := domain.Spec{Name: "Кофейня", INN: "001234567890", Currency: "RUB", Timezone: "Europe/Moscow"}
	hash, _ := domain.Fingerprint(spec)
	return BootstrapCommand{CommandID: uuid.New(), CompanyID: uuid.MustParse("019a0c65-9800-7000-8000-000000000001"), Spec: spec, RequestSHA256: hash}
}
func grant(cmd BootstrapCommand) Authorization {
	return Authorization(cmd)
}
func TestBootstrapReplayReauthorizes(t *testing.T) {
	cmd := commandFixture()
	storage := &fakeStore{}
	calls := 0
	s := New(func(ctx context.Context, w Work) error { return w(ctx, storage) }, func(_ context.Context, commandID, companyID uuid.UUID, hash string) (Authorization, error) {
		calls++
		if commandID != cmd.CommandID || companyID != cmd.CompanyID || hash != cmd.RequestSHA256 {
			t.Error("wrong authorization scope")
		}
		return grant(cmd), nil
	})
	first, err := s.Bootstrap(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.Bootstrap(t.Context(), cmd)
	if err != nil || replay != first || calls != 2 || storage.inserts != 1 {
		t.Fatal(replay, err, calls, storage.inserts)
	}
	if first.AccountID.Version() != 7 || first.CompanyID != cmd.CompanyID || first.Spec != cmd.Spec {
		t.Fatal(first)
	}
	saved, err := s.GetBootstrapResult(t.Context(), cmd.CommandID)
	if err != nil || saved != first {
		t.Fatal(saved, err)
	}
	profile, err := s.ReadProfile(t.Context(), cmd.CompanyID)
	if err != nil || profile.AccountID != first.AccountID {
		t.Fatal(profile, err)
	}
}
func TestBootstrapInvalidAndUnauthorizedNeverEnterTransaction(t *testing.T) {
	for name, edit := range map[string]func(*BootstrapCommand){
		"command": func(c *BootstrapCommand) { c.CommandID = uuid.Nil }, "company": func(c *BootstrapCommand) { c.CompanyID = uuid.New() },
		"hash": func(c *BootstrapCommand) { c.RequestSHA256 = "wrong" }, "spec": func(c *BootstrapCommand) { c.Spec.Currency = "USD" },
	} {
		t.Run(name, func(t *testing.T) {
			cmd := commandFixture()
			edit(&cmd)
			calls := 0
			s := New(func(context.Context, Work) error { t.Error("entered transaction"); return nil }, func(context.Context, uuid.UUID, uuid.UUID, string) (Authorization, error) {
				calls++
				return Authorization{}, nil
			})
			result, err := s.Bootstrap(t.Context(), cmd)
			if !errors.Is(err, ErrInvalid) || result != (BootstrapResult{}) || calls != 0 {
				t.Fatal(result, err, calls)
			}
		})
	}
	for _, mode := range []string{"nil authorizer", "authorization error", "mismatched spec", "mismatched company", "mismatched command", "mismatched hash"} {
		t.Run(mode, func(t *testing.T) {
			cmd := commandFixture()
			authorize := func(context.Context, uuid.UUID, uuid.UUID, string) (Authorization, error) {
				g := grant(cmd)
				switch mode {
				case "authorization error":
					return Authorization{}, ErrUnavailable
				case "mismatched spec":
					g.Spec.Name = "Other"
				case "mismatched company":
					g.CompanyID = uuid.New()
				case "mismatched command":
					g.CommandID = uuid.New()
				case "mismatched hash":
					g.RequestSHA256 = "wrong"
				}
				return g, nil
			}
			if mode == "nil authorizer" {
				authorize = nil
			}
			s := New(func(context.Context, Work) error { t.Error("entered transaction"); return nil }, authorize)
			result, err := s.Bootstrap(t.Context(), cmd)
			if err == nil || result != (BootstrapResult{}) {
				t.Fatal(result, err)
			}
		})
	}
}
func TestBootstrapTransactionAndReceiptFailures(t *testing.T) {
	for _, mode := range []string{"commit", "storage", "wrong receipt identity", "receipt hash"} {
		t.Run(mode, func(t *testing.T) {
			cmd := commandFixture()
			storage := &fakeStore{}
			s := New(func(ctx context.Context, w Work) error {
				if mode == "storage" {
					storage.err = ErrUnavailable
				}
				if mode == "wrong receipt identity" || mode == "receipt hash" {
					valid := New(func(ctx context.Context, w Work) error { return w(ctx, storage) }, func(context.Context, uuid.UUID, uuid.UUID, string) (Authorization, error) { return grant(cmd), nil })
					if _, err := valid.Bootstrap(ctx, cmd); err != nil {
						t.Fatal(err)
					}
					storage.inserts = 0
					if mode == "wrong receipt identity" {
						storage.receipt.CommandID = uuid.New()
					} else {
						storage.receipt.RequestSHA256 = "broken"
					}
				}
				if err := w(ctx, storage); err != nil {
					return err
				}
				return ErrUnavailable
			}, func(context.Context, uuid.UUID, uuid.UUID, string) (Authorization, error) { return grant(cmd), nil })
			result, err := s.Bootstrap(t.Context(), cmd)
			if err == nil || result != (BootstrapResult{}) {
				t.Fatal("published failed result", result, err)
			}
			if mode != "commit" && storage.inserts != 0 {
				t.Fatal("failure inserted company")
			}
		})
	}
}

func TestAuthorizationErrorIsSafeAndInspectable(t *testing.T) {
	cmd := commandFixture()
	cause := errors.New("private upstream cookie=do-not-expose")
	for _, underlying := range []error{cause, fmt.Errorf("private: %w", ErrUnauthorized), fmt.Errorf("private: %w", context.Canceled)} {
		s := New(func(context.Context, Work) error { t.Error("entered transaction"); return nil }, func(context.Context, uuid.UUID, uuid.UUID, string) (Authorization, error) {
			return Authorization{}, underlying
		})
		_, err := s.Bootstrap(t.Context(), cmd)
		if err == nil || !errors.Is(err, underlying) || strings.Contains(err.Error(), "private") {
			t.Fatal(err)
		}
	}
}
