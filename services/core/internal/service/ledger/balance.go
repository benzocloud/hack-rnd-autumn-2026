package ledger

import (
	"context"
	"time"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

type BalanceCommand struct {
	Scope               domain.Scope
	Date                domain.Date
	Amount              money.Money
	Source              domain.BalanceSource
	Note                string
	ExpectedDataVersion int64
	ActorID             string
	IdempotencyKey      string
}

type RecordBalanceResult struct {
	Date        domain.Date
	DataVersion int64
}

func (s *service) RecordBalance(ctx context.Context, cmd BalanceCommand) (RecordBalanceResult, error) {
	if err := validateRecordBalance(cmd); err != nil {
		return RecordBalanceResult{}, err
	}
	if cmd.Source.RecordedAt.IsZero() {
		cmd.Source.RecordedAt = time.Now().UTC()
	}
	record, err := domain.NewBalanceObservation(cmd.Scope, cmd.Date, cmd.Amount, cmd.Source)
	if err != nil {
		return RecordBalanceResult{}, err
	}
	return s.store.RecordBalance(ctx, cmd, record)
}

func validateRecordBalance(cmd BalanceCommand) error {
	if err := validateActorAndKey(cmd.ActorID, cmd.IdempotencyKey, cmd.Source.Kind == domain.ManualEOD); err != nil {
		return err
	}
	if cmd.ExpectedDataVersion < 1 || (cmd.Source.Kind == domain.ManualEOD && cmd.Note == "") {
		return ErrInvalid
	}
	return nil
}
