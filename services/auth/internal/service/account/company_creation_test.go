package account

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/domain/account"
)

func (m *memoryStore) FindCreation(_ context.Context, id uuid.UUID, _ bool) (domain.CompanyCreation, error) {
	c, ok := m.creations[id]
	if !ok {
		return domain.CompanyCreation{}, ErrNotFound
	}
	return c, nil
}
func (m *memoryStore) FindCreationByKey(_ context.Context, user uuid.UUID, key string) (domain.CompanyCreation, error) {
	for _, c := range m.creations {
		if c.InitiatorID == user && c.Key == key {
			return c, nil
		}
	}
	return domain.CompanyCreation{}, ErrNotFound
}
func (m *memoryStore) InsertCreation(_ context.Context, c domain.CompanyCreation) error {
	for _, old := range m.creations {
		if old.ID == c.ID || old.CompanyID == c.CompanyID || old.InitiatorID == c.InitiatorID && old.Key == c.Key {
			return domain.ErrConflict
		}
	}
	m.creations[c.ID] = c
	return nil
}
func (m *memoryStore) CompleteCreation(_ context.Context, id, account uuid.UUID, committed time.Time, lease uuid.UUID) error {
	if m.failComplete {
		return ErrUnavailable
	}
	c, ok := m.creations[id]
	if !ok || c.State != domain.CreationPrepared || !memoryLeaseMatches(c, lease) {
		return domain.ErrLeaseLost
	}
	c.State = domain.CreationCompleted
	c.AccountID = account
	c.CommittedAt = &committed
	c.LeaseToken = uuid.Nil
	c.LeaseExpiresAt = nil
	m.creations[id] = c
	return nil
}
func memoryLeaseMatches(c domain.CompanyCreation, lease uuid.UUID) bool {
	return lease == uuid.Nil || (c.LeaseToken == lease && c.LeaseExpiresAt != nil && time.Now().Before(*c.LeaseExpiresAt))
}
func (m *memoryStore) QuarantineCreation(_ context.Context, id, lease uuid.UUID) error {
	c, ok := m.creations[id]
	if !ok || c.State != domain.CreationPrepared || !memoryLeaseMatches(c, lease) {
		return domain.ErrLeaseLost
	}
	c.State = domain.CreationQuarantined
	c.LeaseToken = uuid.Nil
	c.LeaseExpiresAt = nil
	m.creations[id] = c
	return nil
}
func (m *memoryStore) ClaimCreation(_ context.Context, token uuid.UUID) (domain.CompanyCreation, error) {
	for id, c := range m.creations {
		if c.State == domain.CreationPrepared && !time.Now().Before(c.NextAttemptAt) && (c.LeaseExpiresAt == nil || !time.Now().Before(*c.LeaseExpiresAt)) {
			expiry := time.Now().Add(time.Minute)
			c.LeaseToken = token
			c.LeaseExpiresAt = &expiry
			c.Attempts++
			m.creations[id] = c
			return c, nil
		}
	}
	return domain.CompanyCreation{}, ErrNotFound
}
func (m *memoryStore) RetryCreation(_ context.Context, id, lease uuid.UUID, delay time.Duration) error {
	c, ok := m.creations[id]
	if !ok || c.State != domain.CreationPrepared || !memoryLeaseMatches(c, lease) {
		return domain.ErrLeaseLost
	}
	c.LeaseToken = uuid.Nil
	c.LeaseExpiresAt = nil
	c.NextAttemptAt = time.Now().Add(delay)
	m.creations[id] = c
	return nil
}
func (m *memoryStore) InsertOwner(_ context.Context, company, user uuid.UUID, created time.Time) error {
	key := membershipKey(company, user)
	if old, ok := m.memberships[key]; ok {
		if old.Role == "owner" {
			return nil
		}
		return domain.ErrConflict
	}
	for _, old := range m.memberships {
		if old.CompanyID == company && old.Role == "owner" && old.UserID != user {
			return domain.ErrConflict
		}
	}
	m.memberships[key] = domain.Membership{CompanyID: company, UserID: user, Role: "owner", CreatedAt: created}
	return nil
}

type fakeCreationCore struct {
	receipt   CreationReceipt
	err       error
	read      func(context.Context, uuid.UUID) (CreationReceipt, error)
	bootstrap func(context.Context, CreationCommand) error
}

func (c *fakeCreationCore) GetCreationResult(ctx context.Context, id uuid.UUID) (CreationReceipt, error) {
	if c.read != nil {
		return c.read(ctx, id)
	}
	return c.receipt, c.err
}
func (c *fakeCreationCore) BootstrapCompany(ctx context.Context, cmd CreationCommand) error {
	if c.bootstrap != nil {
		return c.bootstrap(ctx, cmd)
	}
	return c.err
}

func creationFixture(t *testing.T) (*verifyFixture, BeginCompanyCreationInput) {
	t.Helper()
	f := fixture(t, false)
	f.user.EmailVerified = true
	f.store.users[f.user.ID] = f.user
	c, token, err := f.service.newContext(f.user)
	if err != nil {
		t.Fatal(err)
	}
	f.store.contexts[string(c.TokenHash)] = c
	f.write = WriteContext{CookieToken: token, CSRFToken: f.service.csrfToken(c)}
	return f, BeginCompanyCreationInput{Context: f.write, Key: "company-create-key", Spec: CompanySpec{Name: " Кофейня ", Currency: "RUB", Timezone: "Europe/Moscow"}}
}
func receiptFor(c CreationCommand) CreationReceipt {
	return CreationReceipt{CommandID: c.ID, CompanyID: c.CompanyID, AccountID: uuid.Must(uuid.NewV7()), Spec: c.Spec, RequestSHA256: c.RequestSHA256, CommittedAt: c.CreatedAt}
}

func TestBeginCompanyCreationSessionKeyAndReplay(t *testing.T) {
	f, input := creationFixture(t)
	var wg sync.WaitGroup
	results := make(chan CreationCommand, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Go(func() { got, err := f.service.BeginCompanyCreation(t.Context(), input); results <- got; errs <- err })
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first CreationCommand
	for result := range results {
		if first.ID == uuid.Nil {
			first = result
		} else if result != first {
			t.Fatal("replay changed", result, first)
		}
	}
	if len(f.store.creations) != 1 || first.ID.Version() != 4 || first.CompanyID.Version() != 7 || first.InitiatorID != f.user.ID || first.Spec.Name != "Кофейня" || first.State != domain.CreationPrepared {
		t.Fatal(first)
	}
	input.Spec.Name = "Different"
	if _, err := f.service.BeginCompanyCreation(t.Context(), input); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatal(err)
	}
	input.Spec.Name = "Кофейня"
	input.Context.CSRFToken = "wrong"
	if _, err := f.service.BeginCompanyCreation(t.Context(), input); !errors.Is(err, ErrCSRFInvalid) {
		t.Fatal(err)
	}
	input.Context = f.write
	f.now = f.now.Add(25 * time.Hour)
	if _, err := f.service.BeginCompanyCreation(t.Context(), input); !errors.Is(err, ErrAuthRequired) {
		t.Fatal(err)
	}
	if len(f.store.creations) != 1 {
		t.Fatal("denied attempts created command")
	}
}

func TestCompleteCompanyCreationReadsOutsideTransactionAndCommitsOwner(t *testing.T) {
	f, input := creationFixture(t)
	cmd, err := f.service.BeginCompanyCreation(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	receipt := receiptFor(cmd)
	f.service.core = &fakeCreationCore{read: func(ctx context.Context, id uuid.UUID) (CreationReceipt, error) {
		if !f.store.mu.TryLock() {
			t.Error("SQL transaction held across RPC")
			return CreationReceipt{}, ErrUnavailable
		}
		f.store.mu.Unlock()
		grant, err := f.service.AuthorizeCompanyBootstrap(ctx, AuthorizeCompanyBootstrapInput{CommandID: id, CompanyID: cmd.CompanyID, RequestSHA256: cmd.RequestSHA256})
		if err != nil || grant != cmd {
			t.Error(grant, err)
		}
		return receipt, nil
	}}
	completed, err := f.service.CompleteCompanyCreation(t.Context(), CompleteCompanyCreationInput{Context: f.write, CommandID: cmd.ID})
	if err != nil || completed.ID != cmd.ID || completed.State != domain.CreationCompleted {
		t.Fatal(completed, err)
	}
	owner := f.store.memberships[membershipKey(cmd.CompanyID, f.user.ID)]
	if owner.UserID != f.user.ID || owner.Role != "owner" {
		t.Fatal(owner)
	}
	// Completed replay cannot recreate access after an independent revocation.
	delete(f.store.memberships, membershipKey(cmd.CompanyID, f.user.ID))
	f.service.core = nil
	if _, err := f.service.CompleteCompanyCreation(t.Context(), CompleteCompanyCreationInput{Context: f.write, CommandID: cmd.ID}); err != nil {
		t.Fatal(err)
	}
	if len(f.store.memberships) != 0 {
		t.Fatal("replay recreated revoked membership")
	}
}

func TestCompleteCompanyCreationFailureAndQuarantine(t *testing.T) {
	for _, mode := range []string{"missing", "unavailable", "wrong command", "wrong company", "wrong spec", "wrong hash", "nil account", "wrong account version", "same account", "missing commit", "disabled", "deleted", "rollback", "foreign"} {
		t.Run(mode, func(t *testing.T) {
			f, input := creationFixture(t)
			cmd, err := f.service.BeginCompanyCreation(t.Context(), input)
			if err != nil {
				t.Fatal(err)
			}
			receipt := receiptFor(cmd)
			want := ErrForbidden
			state := domain.CreationQuarantined
			core := &fakeCreationCore{receipt: receipt}
			f.service.core = core
			switch mode {
			case "missing":
				core.err = ErrNotFound
				want = ErrCreationInProgress
				state = domain.CreationPrepared
			case "unavailable":
				core.err = ErrUnavailable
				want = ErrUnavailable
				state = domain.CreationPrepared
			case "wrong command":
				core.receipt.CommandID = uuid.New()
			case "wrong company":
				core.receipt.CompanyID = uuid.Must(uuid.NewV7())
			case "wrong spec":
				core.receipt.Spec.Name = "Different"
			case "wrong hash":
				core.receipt.RequestSHA256 = "wrong"
			case "nil account":
				core.receipt.AccountID = uuid.Nil
			case "wrong account version":
				core.receipt.AccountID = uuid.New()
			case "same account":
				core.receipt.AccountID = cmd.CompanyID
			case "missing commit":
				core.receipt.CommittedAt = time.Time{}
			case "disabled", "deleted":
				core.read = func(context.Context, uuid.UUID) (CreationReceipt, error) {
					if mode == "deleted" {
						delete(f.store.users, f.user.ID)
					} else {
						user := f.store.users[f.user.ID]
						user.State = "disabled"
						f.store.users[user.ID] = user
					}
					return receipt, nil
				}
			case "rollback":
				f.store.failComplete = true
				want = ErrUnavailable
				state = domain.CreationPrepared
			case "foreign":
				stored := f.store.creations[cmd.ID]
				stored.InitiatorID = uuid.New()
				f.store.creations[cmd.ID] = stored
				state = domain.CreationPrepared
			}
			if got, err := f.service.CompleteCompanyCreation(t.Context(), CompleteCompanyCreationInput{Context: f.write, CommandID: cmd.ID}); !errors.Is(err, want) || got.ID != uuid.Nil {
				t.Fatal(got, err, want)
			}
			if len(f.store.memberships) != 0 || f.store.creations[cmd.ID].State != state {
				t.Fatal("bad final state", f.store.memberships, f.store.creations[cmd.ID])
			}
		})
	}
}

func TestAuthorizeBootstrapQuarantinesDisabledButNotForeignInput(t *testing.T) {
	f, input := creationFixture(t)
	cmd, err := f.service.BeginCompanyCreation(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	grant := AuthorizeCompanyBootstrapInput{CommandID: cmd.ID, CompanyID: uuid.Must(uuid.NewV7()), RequestSHA256: cmd.RequestSHA256}
	if _, err := f.service.AuthorizeCompanyBootstrap(t.Context(), grant); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
	if f.store.creations[cmd.ID].State != domain.CreationPrepared {
		t.Fatal("foreign input quarantined valid command")
	}
	grant.CompanyID = cmd.CompanyID
	user := f.store.users[f.user.ID]
	user.State = "disabled"
	f.store.users[user.ID] = user
	if _, err := f.service.AuthorizeCompanyBootstrap(t.Context(), grant); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
	if f.store.creations[cmd.ID].State != domain.CreationQuarantined {
		t.Fatal("disabled command not durably quarantined")
	}

	f2, input2 := creationFixture(t)
	cmd2, err := f2.service.BeginCompanyCreation(t.Context(), input2)
	if err != nil {
		t.Fatal(err)
	}
	f2.service.core = &fakeCreationCore{receipt: receiptFor(cmd2)}
	if _, err := f2.service.CompleteCompanyCreation(t.Context(), CompleteCompanyCreationInput{Context: f2.write, CommandID: cmd2.ID}); err != nil {
		t.Fatal(err)
	}
	disabled := f2.store.users[f2.user.ID]
	disabled.State = "disabled"
	f2.store.users[disabled.ID] = disabled
	if got, err := f2.service.AuthorizeCompanyBootstrap(t.Context(), AuthorizeCompanyBootstrapInput{CommandID: cmd2.ID, CompanyID: cmd2.CompanyID, RequestSHA256: cmd2.RequestSHA256}); err != nil || got.State != domain.CreationCompleted {
		t.Fatal(got, err)
	}
}
