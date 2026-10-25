package app

import (
	"context"
	"log/slog"
	"time"
)

type recoveryOperations interface {
	ReconcileOne(context.Context) (bool, error)
}

// The application owns this single bounded worker and joins it before closing
// RPC/DB/client resources. Failed/crashed work remains durable behind its lease.
func startRecovery(ctx context.Context, accounts recoveryOperations, logger *slog.Logger) func() {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		timer := time.NewTimer(0)
		defer timer.Stop()
		errorDelay := time.Second
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			workCtx, cancelWork := context.WithTimeout(ctx, 10*time.Second)
			worked, err := accounts.ReconcileOne(workCtx)
			cancelWork()
			if ctx.Err() != nil {
				return
			}
			delay := time.Second
			if err != nil {
				logger.WarnContext(ctx, "company creation recovery will retry")
				delay = errorDelay
				errorDelay = min(errorDelay*2, time.Minute)
			} else {
				errorDelay = time.Second
				if worked {
					delay = 0
				}
			}
			timer.Reset(delay)
		}
	}()
	return func() { cancel(); <-done }
}
