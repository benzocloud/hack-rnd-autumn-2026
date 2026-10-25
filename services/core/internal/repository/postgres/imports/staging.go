package imports

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/imports"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

func (r *repository) AddRows(ctx context.Context, record domain.Record, rows []domain.NormalizedRow) error {
	for _, row := range rows {
		payload, err := json.Marshal(row)
		if err != nil {
			return domain.ErrUnavailable
		}
		economy := row
		economy.Row = 0
		economy.Balance = 0
		economyJSON, err := json.Marshal(economy)
		if err != nil {
			return domain.ErrUnavailable
		}
		content := economy
		content.ExternalID = ""
		contentJSON, err := json.Marshal(content)
		if err != nil {
			return domain.ErrUnavailable
		}
		hash := sha256.Sum256(contentJSON)
		_, err = r.tx.ExecContext(ctx, queryAddRow, record.ID, row.Row, row.ExternalID, row.Date, row.Amount, row.Direction, row.Balance, hex.EncodeToString(hash[:]), economyJSON, payload)
		if err != nil {
			return classify(err)
		}
	}
	return nil
}
func (r *repository) AddIssues(ctx context.Context, id uuid.UUID, issues []domain.Issue) error {
	for _, issue := range issues {
		payload, err := json.Marshal(issue)
		if err != nil {
			return domain.ErrUnavailable
		}
		if _, err := r.tx.ExecContext(ctx, queryAddIssue, id, payload); err != nil {
			return classify(err)
		}
	}
	return nil
}
func (r *repository) Summary(ctx context.Context, record domain.Record) (domain.Preview, error) {
	revision, binding, err := r.Account(ctx, record.Metadata.Scope)
	if err != nil {
		return domain.Preview{}, err
	}
	p := domain.Preview{BaseVersion: revision, Source: binding}
	if binding.Mode == "" {
		p.WouldBind = &domain.Binding{Mode: "imported", System: "dataset_v1", CompanyID: record.Metadata.SourceCompanyID}
	}
	if _, err := r.tx.ExecContext(ctx, queryDiagnostics, record.ID, record.Metadata.Scope.CompanyID, record.Metadata.Scope.AccountID, record.Metadata.SourceCompanyID); err != nil {
		return p, classify(err)
	}
	err = r.tx.QueryRowxContext(ctx, querySummary, record.ID, record.Metadata.Scope.CompanyID, record.Metadata.Scope.AccountID, record.Metadata.SourceCompanyID).Scan(&p.PeriodStart, &p.PeriodEnd, &p.NewTransactions, &p.AlreadyPresent, &p.Blockers, &p.Warnings)
	if err != nil {
		return p, classify(err)
	}
	balances, err := r.tx.QueryxContext(ctx, queryBalances, record.ID)
	if err != nil {
		return p, classify(err)
	}
	for balances.Next() {
		var b domain.Balance
		if err := balances.Scan(&b.Date, &b.Minor); err != nil {
			closeErr := balances.Close()
			return p, classifyJoin(err, closeErr)
		}
		p.Balances = append(p.Balances, b)
	}
	if err := classifyJoin(balances.Err(), balances.Close()); err != nil {
		return p, err
	}
	p.Issues, _, err = r.Issues(ctx, record.ID, 0, 50)
	return p, err
}
func (r *repository) List(ctx context.Context, scope ledger.Scope, limit int, before time.Time, id uuid.UUID) (result []domain.Record, err error) {
	rows, err := r.tx.QueryxContext(ctx, queryList, scope.CompanyID, scope.AccountID, nullable(before), id, limit)
	if err != nil {
		return nil, classify(err)
	}
	defer func() { err = classifyJoin(err, rows.Close()) }()
	result = make([]domain.Record, 0)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, classify(err)
		}
		var record domain.Record
		if json.Unmarshal(payload, &record) != nil {
			return nil, domain.ErrUnavailable
		}
		result = append(result, record)
	}
	return result, classify(rows.Err())
}
func (r *repository) Issues(ctx context.Context, id uuid.UUID, after int64, limit int) (result []domain.Issue, ids []int64, err error) {
	rows, err := r.tx.QueryxContext(ctx, queryIssues, id, after, limit)
	if err != nil {
		return nil, nil, classify(err)
	}
	defer func() { err = classifyJoin(err, rows.Close()) }()
	result = make([]domain.Issue, 0)
	for rows.Next() {
		var payload []byte
		var ordinal int64
		if err := rows.Scan(&ordinal, &payload); err != nil {
			return nil, nil, classify(err)
		}
		var issue domain.Issue
		if json.Unmarshal(payload, &issue) != nil {
			return nil, nil, domain.ErrUnavailable
		}
		result = append(result, issue)
		ids = append(ids, ordinal)
	}
	return result, ids, classify(rows.Err())
}
