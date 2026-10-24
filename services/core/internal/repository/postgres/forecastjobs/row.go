package forecastjobs

import (
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

type jobRow struct {
	ID          uuid.UUID     `db:"id"`
	CompanyID   uuid.UUID     `db:"company_id"`
	AccountID   uuid.UUID     `db:"account_id"`
	DataVersion int64         `db:"data_version"`
	Status      string        `db:"status"`
	CreatedAt   time.Time     `db:"created_at"`
	CompletedAt *time.Time    `db:"completed_at"`
	LeaseToken  uuid.NullUUID `db:"lease_token"`
	Input       []byte        `db:"input"`
	Result      []byte        `db:"result"`
	ErrorCode   string        `db:"error_code"`
	RequestID   string        `db:"request_id"`
}

func (r jobRow) job() domain.Job {
	var completed *time.Time
	if r.CompletedAt != nil {
		at := r.CompletedAt.UTC()
		completed = &at
	}
	return domain.Job{ID: r.ID, Scope: ledger.Scope{CompanyID: r.CompanyID, AccountID: r.AccountID}, DataVersion: r.DataVersion,
		Status: r.Status, CreatedAt: r.CreatedAt.UTC(), CompletedAt: completed, LeaseToken: r.LeaseToken.UUID,
		Input: r.Input, Result: r.Result, ErrorCode: r.ErrorCode, RequestID: r.RequestID}
}
