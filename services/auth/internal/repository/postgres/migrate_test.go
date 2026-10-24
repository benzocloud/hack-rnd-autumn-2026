package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

type stalledSessionLocker struct{}

func (stalledSessionLocker) SessionLock(context.Context, *sql.Conn) error { return nil }

func (stalledSessionLocker) SessionUnlock(ctx context.Context, _ *sql.Conn) error {
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("unlock received no deadline")
	}
	<-ctx.Done()
	return ctx.Err()
}

func TestSessionUnlockHasDeadlineAfterGooseDetachesContext(t *testing.T) {
	locker := boundedSessionLocker{SessionLocker: stalledSessionLocker{}, timeout: 20 * time.Millisecond}
	parent, cancel := context.WithCancel(t.Context())
	cancel()
	started := time.Now()
	err := locker.SessionUnlock(context.WithoutCancel(parent), nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unlock did not use bounded cleanup: %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("stalled unlock exceeded its cleanup bound")
	}
}
