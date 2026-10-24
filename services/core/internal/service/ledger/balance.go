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

type balanceStore interface {
	accountStore
	FindBalance(context.Context, domain.Scope, domain.Date) (domain.BalanceObservation, int64, bool, error)
	InsertBalance(context.Context, domain.BalanceObservation, string, int64) error
}

func (s *service) RecordBalance(ctx context.Context, cmd BalanceCommand) (RecordBalanceResult, error) {
	if err := validateRecordBalance(cmd); err != nil {
		return RecordBalanceResult{}, err
	}
	source := cmd.Source
	if source.RecordedAt.IsZero() {
		source.RecordedAt = time.Now().UTC()
	}
	record, err := domain.NewBalanceObservation(cmd.Scope, cmd.Date, cmd.Amount, source)
	if err != nil {
		return RecordBalanceResult{}, err
	}
	var result RecordBalanceResult
	txErr := s.withinTx(ctx, func(ctx context.Context, store store, commands commandStore) error {
		var saved balanceResult
		replay, err := replayCommand(ctx, commands, cmd.Scope, cmd.ActorID, "record_balance", "", cmd.IdempotencyKey, balanceRequest(cmd), &saved)
		if err != nil {
			return err
		}
		if replay {
			if err := validateBalanceResult(saved); err != nil {
				return err
			}
			result = RecordBalanceResult(saved)
			return nil
		}
		result, err = recordBalance(ctx, store, cmd, record)
		if err != nil {
			return err
		}
		return completeCommand(ctx, commands, cmd.Scope, cmd.ActorID, "record_balance", "", cmd.IdempotencyKey, balanceResult(result))
	})
	if txErr != nil {
		return RecordBalanceResult{}, txErr
	}
	return result, nil
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

func recordBalance(ctx context.Context, store balanceStore, cmd BalanceCommand, record domain.BalanceObservation) (RecordBalanceResult, error) {
	account, err := store.LoadAccount(ctx, record.Scope())
	if err != nil {
		return RecordBalanceResult{}, err
	}
	return recordBalanceOnAccount(ctx, store, account, cmd, record)
}

func recordBalanceOnAccount(ctx context.Context, store balanceStore, account *domain.BankAccount, cmd BalanceCommand, record domain.BalanceObservation) (RecordBalanceResult, error) {
	if account == nil || account.Scope() != record.Scope() {
		return RecordBalanceResult{}, ErrUnavailable
	}
	stored, revision, found, err := store.FindBalance(ctx, record.Scope(), record.Date())
	if err != nil {
		return RecordBalanceResult{}, err
	}
	if found {
		if _, err := record.CheckReplay(stored); err != nil {
			return RecordBalanceResult{}, err
		}
		return RecordBalanceResult{Date: stored.Date(), DataVersion: revision}, nil
	}
	if account.DataVersion() != cmd.ExpectedDataVersion {
		return RecordBalanceResult{}, ErrStaleVersion
	}
	// External EOD is factual input; manual EOD does not choose history authority.
	if record.Source().Kind == domain.StatementEOD {
		if err := account.BindStatementFacts(record.Source().Namespace); err != nil {
			return RecordBalanceResult{}, err
		}
	}
	if err := account.AdvanceDataVersion(); err != nil {
		return RecordBalanceResult{}, err
	}
	if err := store.SaveAccount(ctx, account); err != nil {
		return RecordBalanceResult{}, err
	}
	if err := store.InsertBalance(ctx, record, cmd.Note, account.DataVersion()); err != nil {
		return RecordBalanceResult{}, err
	}
	return RecordBalanceResult{Date: record.Date(), DataVersion: account.DataVersion()}, nil
}
