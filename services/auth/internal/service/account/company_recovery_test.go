package account

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/domain/account"
)

func TestRecoveryCompletesCommittedReceiptAfterSessionExpiryAndReset(t *testing.T) {
	f, input := creationFixture(t)
	command, err := f.service.BeginCompanyCreation(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	for hash, c := range f.store.contexts {
		c.ExpiresAt = f.now.Add(-time.Second)
		f.store.contexts[hash] = c
	}
	user := f.store.users[f.user.ID]
	user.CredentialVersion++
	f.store.users[user.ID] = user
	var bootstraps int
	f.service.core = &fakeCreationCore{receipt: receiptFor(command), bootstrap: func(context.Context, CreationCommand) error { bootstraps++; return nil }}
	found, err := f.service.ReconcileOne(t.Context())
	if err != nil || !found || bootstraps != 0 || f.store.creations[command.ID].State != domain.CreationCompleted || len(f.store.memberships) != 1 {
		t.Fatal(found, err, bootstraps, f.store.creations)
	}
	if _, err := f.service.BeginCompanyCreation(t.Context(), input); !errors.Is(err, ErrAuthRequired) {
		t.Fatal("expired browser accepted", err)
	}
	if found, err := f.service.ReconcileOne(t.Context()); found || err != nil {
		t.Fatal(found, err)
	}
}

func TestRecoveryBootstrapLostReplyAndRestartReusesCommittedReceipt(t *testing.T) {
	f, input := creationFixture(t)
	command, err := f.service.BeginCompanyCreation(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	var committed *CreationReceipt
	bootstraps := 0
	core := &fakeCreationCore{read: func(context.Context, uuid.UUID) (CreationReceipt, error) {
		if committed == nil {
			return CreationReceipt{}, ErrNotFound
		}
		return *committed, nil
	}, bootstrap: func(_ context.Context, got CreationCommand) error {
		if got.ID != command.ID || got.CompanyID != command.CompanyID || got.Spec != command.Spec || got.RequestSHA256 != command.RequestSHA256 {
			t.Error("bootstrap changed durable spec", got)
		}
		bootstraps++
		receipt := receiptFor(got)
		committed = &receipt
		return context.DeadlineExceeded
	}}
	f.service.core = core
	if found, err := f.service.ReconcileOne(t.Context()); !found || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(found, err)
	}
	saved := f.store.creations[command.ID]
	if saved.State != domain.CreationPrepared || len(f.store.memberships) != 0 || saved.LeaseToken != uuid.Nil || !saved.NextAttemptAt.After(time.Now()) {
		t.Fatal(saved)
	}
	saved.NextAttemptAt = time.Now().Add(-time.Second)
	f.store.creations[command.ID] = saved
	options := f.options
	options.Core = core
	restarted, err := New(f.store.work, options)
	if err != nil {
		t.Fatal(err)
	}
	if found, err := restarted.ReconcileOne(t.Context()); !found || err != nil {
		t.Fatal(found, err)
	}
	if bootstraps != 1 || f.store.creations[command.ID].CompanyID != command.CompanyID || len(f.store.memberships) != 1 {
		t.Fatal("restart duplicated bootstrap", bootstraps, f.store.creations)
	}
}

func TestRecoveryRechecksReceiptAndInitiator(t *testing.T) {
	for _, mode := range []string{"bootstrap then read", "wrong receipt", "disabled", "deleted", "invalid response"} {
		t.Run(mode, func(t *testing.T) {
			f, input := creationFixture(t)
			command, err := f.service.BeginCompanyCreation(t.Context(), input)
			if err != nil {
				t.Fatal(err)
			}
			calls, bootstraps := 0, 0
			receipt := receiptFor(command)
			available := false
			core := &fakeCreationCore{read: func(context.Context, uuid.UUID) (CreationReceipt, error) {
				calls++
				if !f.store.mu.TryLock() {
					t.Error("transaction held across recovery RPC")
					return CreationReceipt{}, ErrUnavailable
				}
				f.store.mu.Unlock()
				if mode == "invalid response" {
					return CreationReceipt{}, ErrReceiptMismatch
				}
				if mode == "wrong receipt" {
					receipt.AccountID = uuid.Nil
					return receipt, nil
				}
				if !available {
					return CreationReceipt{}, ErrNotFound
				}
				return receipt, nil
			}, bootstrap: func(context.Context, CreationCommand) error { bootstraps++; available = true; return nil }}
			f.service.core = core
			if mode == "disabled" {
				user := f.store.users[f.user.ID]
				user.State = "disabled"
				f.store.users[user.ID] = user
			}
			if mode == "deleted" {
				delete(f.store.users, f.user.ID)
			}
			if found, err := f.service.ReconcileOne(t.Context()); !found || err != nil {
				t.Fatal(found, err)
			}
			if mode == "bootstrap then read" {
				if calls != 2 || bootstraps != 1 || len(f.store.memberships) != 1 || f.store.creations[command.ID].State != domain.CreationCompleted {
					t.Fatal(calls, bootstraps, f.store.creations)
				}
			} else {
				if len(f.store.memberships) != 0 || f.store.creations[command.ID].State != domain.CreationQuarantined {
					t.Fatal(f.store.creations)
				}
				if (mode == "disabled" || mode == "deleted") && calls != 0 {
					t.Fatal("disabled initiator contacted Core")
				}
			}
		})
	}
}

func TestRecoveryTwoWorkersFenceOldClaimBeforeMembership(t *testing.T) {
	f, input := creationFixture(t)
	command, err := f.service.BeginCompanyCreation(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	oldStarted, newStarted := make(chan struct{}), make(chan struct{})
	releaseOld, releaseNew := make(chan struct{}), make(chan struct{})
	receipt := receiptFor(command)
	var calls atomic.Int32
	core := &fakeCreationCore{read: func(ctx context.Context, _ uuid.UUID) (CreationReceipt, error) {
		var release <-chan struct{}
		if calls.Add(1) == 1 {
			close(oldStarted)
			release = releaseOld
		} else {
			close(newStarted)
			release = releaseNew
		}
		select {
		case <-ctx.Done():
			return CreationReceipt{}, ctx.Err()
		case <-release:
			return receipt, nil
		}
	}}
	f.service.core = core
	options := f.options
	options.Core = core
	other, err := New(f.store.work, options)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	oldDone, newDone := make(chan error, 1), make(chan error, 1)
	go func() { _, err := f.service.ReconcileOne(ctx); oldDone <- err }()
	select {
	case <-oldStarted:
	case <-t.Context().Done():
		t.Fatal("old worker did not start")
	}
	if found, err := other.ReconcileOne(ctx); found || err != nil {
		t.Fatal("two workers acquired same live lease", found, err)
	}
	f.store.mu.Lock()
	saved := f.store.creations[command.ID]
	expired := time.Now().Add(-time.Second)
	saved.LeaseExpiresAt = &expired
	f.store.creations[command.ID] = saved
	f.store.mu.Unlock()
	go func() { _, err := other.ReconcileOne(ctx); newDone <- err }()
	select {
	case <-newStarted:
	case <-t.Context().Done():
		t.Fatal("new worker did not reclaim")
	}
	close(releaseOld)
	if err := <-oldDone; err != nil {
		t.Fatal(err)
	}
	f.store.mu.Lock()
	members := len(f.store.memberships)
	state := f.store.creations[command.ID].State
	f.store.mu.Unlock()
	if members != 0 || state != domain.CreationPrepared {
		t.Fatal("stale worker granted access", members, state)
	}
	close(releaseNew)
	if err := <-newDone; err != nil {
		t.Fatal(err)
	}
	if len(f.store.memberships) != 1 || f.store.creations[command.ID].State != domain.CreationCompleted {
		t.Fatal(f.store.creations)
	}
}

func TestRecoveryCancelledRPCLeavesDurableClaim(t *testing.T) {
	f, input := creationFixture(t)
	command, err := f.service.BeginCompanyCreation(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	f.service.core = &fakeCreationCore{read: func(ctx context.Context, _ uuid.UUID) (CreationReceipt, error) {
		cancel()
		<-ctx.Done()
		return CreationReceipt{}, ctx.Err()
	}}
	if found, err := f.service.ReconcileOne(ctx); !found || !errors.Is(err, context.Canceled) {
		t.Fatal(found, err)
	}
	if saved := f.store.creations[command.ID]; saved.State != domain.CreationPrepared || saved.LeaseToken == uuid.Nil || len(f.store.memberships) != 0 {
		t.Fatal(saved)
	}
	for _, attempt := range []int64{1, 2, 8, 1 << 50} {
		delay := creationRetryDelay(attempt)
		if delay < time.Second || delay > time.Minute {
			t.Fatal(attempt, delay)
		}
	}
}

func TestRecoveryLeaseExpiryUsesDatabaseClock(t *testing.T) {
	f, input := creationFixture(t)
	command, err := f.service.BeginCompanyCreation(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	// The accepted command predates an Auth host clock jump. Recovery cannot
	// compare a new DB lease to these application clocks or require its cookie.
	f.now = time.Now().UTC().Add(2 * time.Minute)
	f.service.core = &fakeCreationCore{receipt: receiptFor(command)}
	if found, err := f.service.ReconcileOne(t.Context()); !found || err != nil {
		t.Fatal(found, err)
	}
	if f.store.creations[command.ID].State != domain.CreationCompleted || len(f.store.memberships) != 1 {
		t.Fatal("app clock rejected a valid DB lease", f.store.creations)
	}
}
