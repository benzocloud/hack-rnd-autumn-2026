package ledger

import (
	"context"
	"time"
	"unicode/utf8"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

const manualBalanceOperation = "record_manual_balance_v1"

type ManualBalanceCommand struct {
	Scope                   domain.Scope
	Date                    domain.Date
	Amount                  money.Money
	Note                    string
	ExpectedDataVersion     int64
	ActorID, IdempotencyKey string
	// ViewDate is supplied by the trusted adapter's Moscow clock, not the browser.
	ViewDate domain.Date
}

type RecordManualBalanceResult struct {
	Account domain.AccountState
	Balance *domain.BalanceObservation
}

func (s *service) RecordManualBalance(ctx context.Context, cmd ManualBalanceCommand) (RecordManualBalanceResult, error) {
	if err := validateManualBalance(cmd); err != nil {
		return RecordManualBalanceResult{}, err
	}
	if s.withinTx == nil {
		return RecordManualBalanceResult{}, ErrUnavailable
	}
	record, err := domain.NewBalanceObservation(cmd.Scope, cmd.Date, cmd.Amount, domain.BalanceSource{Kind: domain.ManualEOD, RecordedAt: time.Now().UTC()})
	if err != nil {
		return RecordManualBalanceResult{}, ErrInvalid
	}
	var result RecordManualBalanceResult
	err = s.withinTx(ctx, func(ctx context.Context, storage store, commands commandStore) error {
		var receipt manualBalanceReceipt
		replay, err := replayCommand(ctx, commands, cmd.Scope, cmd.ActorID, manualBalanceOperation, "", cmd.IdempotencyKey, manualBalanceRequest(cmd), &receipt)
		if err != nil {
			return err
		}
		if replay {
			result, err = restoreManualBalance(cmd, receipt)
			return err
		}
		account, err := storage.LoadAccount(ctx, cmd.Scope)
		if err != nil {
			return err
		}
		if account == nil || account.Scope() != cmd.Scope {
			return ErrUnavailable
		}
		if account.DataVersion() != cmd.ExpectedDataVersion {
			return ErrStaleVersion
		}
		legacy := BalanceCommand{Scope: cmd.Scope, Date: cmd.Date, Amount: cmd.Amount, Source: record.Source(), Note: cmd.Note, ExpectedDataVersion: cmd.ExpectedDataVersion}
		if _, err := recordBalanceOnAccount(ctx, storage, account, legacy, record); err != nil {
			return err
		}
		latest, revision, found, err := storage.FindLatestBalance(ctx, cmd.Scope, cmd.ViewDate)
		if err != nil {
			return err
		}
		if !found || latest.Scope() != cmd.Scope {
			return ErrUnavailable
		}
		receipt = saveManualBalance(cmd.ViewDate, account.State(), latest, revision)
		result, err = restoreManualBalance(cmd, receipt)
		if err != nil {
			return err
		}
		return completeCommand(ctx, commands, cmd.Scope, cmd.ActorID, manualBalanceOperation, "", cmd.IdempotencyKey, receipt)
	})
	if err != nil {
		return RecordManualBalanceResult{}, err
	}
	return result, nil
}

func validateManualBalance(cmd ManualBalanceCommand) error {
	if cmd.ActorID == "" || !validManualBalanceKey(cmd.IdempotencyKey) || cmd.ExpectedDataVersion < 1 || cmd.ViewDate.Validate() != nil || cmd.Date > cmd.ViewDate ||
		!utf8.ValidString(cmd.Note) || utf8.RuneCountInString(cmd.Note) < 1 || utf8.RuneCountInString(cmd.Note) > 500 {
		return ErrInvalid
	}
	for _, r := range cmd.Note {
		if r == 0 {
			return ErrInvalid
		}
	}
	return nil
}

func validManualBalanceKey(key string) bool {
	if len(key) < 8 || len(key) > 128 {
		return false
	}
	for _, c := range []byte(key) {
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '.' || c == '_' || c == ':' || c == '-' {
			continue
		}
		return false
	}
	return true
}
