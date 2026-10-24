package app

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

type recoveryFixture struct {
	call func(context.Context) (bool, error)
}

func (f recoveryFixture) ReconcileOne(ctx context.Context) (bool, error) { return f.call(ctx) }

func TestRecoveryShutdownJoinsCancelledWork(t *testing.T) {
	started := make(chan struct{})
	worker := recoveryFixture{call: func(ctx context.Context) (bool, error) {
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 10*time.Second {
			t.Error("worker deadline missing")
		}
		close(started)
		<-ctx.Done()
		return true, ctx.Err()
	}}
	stop := startRecovery(t.Context(), worker, slog.New(slog.DiscardHandler))
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not start")
	}
	done := make(chan struct{})
	go func() { stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker outlived shutdown")
	}
}

func TestRecoveryBackoffDoesNotStopAuthOrSpin(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		worker := recoveryFixture{call: func(context.Context) (bool, error) { calls.Add(1); return true, errors.New("dependency unavailable") }}
		stop := startRecovery(t.Context(), worker, slog.New(slog.DiscardHandler))
		synctest.Wait()
		if calls.Load() != 1 {
			t.Fatal(calls.Load())
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if calls.Load() != 2 {
			t.Fatal(calls.Load())
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if calls.Load() != 2 {
			t.Fatal("worker ignored backoff", calls.Load())
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if calls.Load() != 3 {
			t.Fatal(calls.Load())
		}
		stop()
		before := calls.Load()
		time.Sleep(time.Minute)
		synctest.Wait()
		if calls.Load() != before {
			t.Fatal("stopped worker retried")
		}
	})
}
