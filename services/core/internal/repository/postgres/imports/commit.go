package imports

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/imports"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

func (r *repository) CommitConflicts(ctx context.Context, record domain.Record) (bool, error) {
	var conflict bool
	err := r.tx.QueryRowxContext(ctx, queryCommitConflicts, record.ID, record.Metadata.Scope.CompanyID, record.Metadata.Scope.AccountID, record.Metadata.SourceCompanyID).Scan(&conflict)
	return conflict, classify(err)
}
func (r *repository) CountNew(ctx context.Context, record domain.Record) (int64, int64, error) {
	var facts, balances int64
	err := r.tx.QueryRowxContext(ctx, queryCountNew, record.ID, record.Metadata.Scope.CompanyID, record.Metadata.Scope.AccountID, record.Metadata.SourceCompanyID).Scan(&facts, &balances)
	return facts, balances, classify(err)
}
func (r *repository) BindSource(ctx context.Context, scope ledger.Scope, company string) error {
	_, err := r.tx.ExecContext(ctx, queryBindSource, scope.CompanyID, scope.AccountID, company)
	return classify(err)
}
func (r *repository) SetRevision(ctx context.Context, scope ledger.Scope, revision int64) error {
	var company string
	if err := r.tx.QueryRowxContext(ctx, querySourceCompany, scope.CompanyID, scope.AccountID).Scan(&company); err != nil {
		return classify(err)
	}
	_, err := r.tx.ExecContext(ctx, querySetRevision, scope.CompanyID, scope.AccountID, revision, company)
	return classify(err)
}
func (r *repository) StagedRows(ctx context.Context, id uuid.UUID, after int64, limit int) (result []domain.NormalizedRow, err error) {
	rows, err := r.tx.QueryxContext(ctx, queryStagedRows, id, after, limit)
	if err != nil {
		return nil, classify(err)
	}
	defer func() { err = classifyJoin(err, rows.Close()) }()
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, classify(err)
		}
		var row domain.NormalizedRow
		if json.Unmarshal(payload, &row) != nil {
			return nil, domain.ErrUnavailable
		}
		result = append(result, row)
	}
	return result, classify(rows.Err())
}
func (r *repository) InsertFacts(ctx context.Context, facts []ledger.Transaction, revision int64, importID uuid.UUID) error {
	const batchSize = 1000
	for start := 0; start < len(facts); start += batchSize {
		end := min(start+batchSize, len(facts))
		var values strings.Builder
		args := make([]any, 0, (end-start)*14)
		for index, fact := range facts[start:end] {
			if index > 0 {
				values.WriteByte(',')
			}
			base := index*14 + 1
			values.WriteByte('(')
			for field := 0; field < 6; field++ {
				if field > 0 {
					values.WriteByte(',')
				}
				values.WriteByte('$')
				values.WriteString(strconv.Itoa(base + field))
			}
			values.WriteString(",'RUB','imported','dataset_v1',")
			for field := 6; field < 12; field++ {
				if field > 6 {
					values.WriteByte(',')
				}
				values.WriteByte('$')
				values.WriteString(strconv.Itoa(base + field))
			}
			values.WriteString(",1,")
			for field := 12; field < 14; field++ {
				if field > 12 {
					values.WriteByte(',')
				}
				values.WriteByte('$')
				values.WriteString(strconv.Itoa(base + field))
			}
			values.WriteString(")")
			d := fact.Data()
			args = append(args, d.Scope.CompanyID, d.Scope.AccountID, d.ID, string(d.Date), d.Direction, d.Amount.Minor(), d.Source.Namespace.CompanyID, d.Source.ExternalID, d.Source.ImportedAt, d.CounterpartyINN, d.CounterpartyName, d.Purpose, revision, importID)
		}
		query := strings.Replace(queryInsertFactsBatch, "%s", values.String(), 1)
		if _, err := r.tx.ExecContext(ctx, query, args...); err != nil {
			return classify(err)
		}
	}
	return nil
}
func (r *repository) InsertBalance(ctx context.Context, b ledger.BalanceObservation, revision int64) error {
	_, err := r.tx.ExecContext(ctx, queryInsertEod, b.Scope().CompanyID, b.Scope().AccountID, string(b.Date()), b.Amount().Minor(), b.Source().Namespace.CompanyID, b.Source().RecordedAt, revision)
	return classify(err)
}
func (r *repository) VerifyCoverage(ctx context.Context, record domain.Record, from, through, cutoff ledger.Date, balance int64, rowNumbers []int64) (bool, error) {
	var valid bool
	err := r.tx.QueryRowxContext(ctx, queryVerifyCoverage, record.ID, string(from), string(through), record.Metadata.Scope.CompanyID, record.Metadata.Scope.AccountID, string(cutoff), balance, record.Metadata.SourceCompanyID, rowNumbers).Scan(&valid)
	return valid, classify(err)
}
func (r *repository) InsertCoverage(ctx context.Context, scope ledger.Scope, proof domain.HistoryCoverage, id uuid.UUID, hash string) error {
	limitations, err := json.Marshal(proof.Limitations)
	if err != nil {
		return domain.ErrUnavailable
	}
	result, err := r.tx.ExecContext(ctx, queryInsertCoverage, scope.CompanyID, scope.AccountID, proof.DataVersion, proof.FromDate, proof.ThroughDate, proof.Reference, proof.Kind, hash, id, limitations)
	if err != nil {
		return classify(err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return domain.ErrUnavailable
	}
	if changed != 1 {
		return domain.ErrState
	}
	return nil
}

func (r *repository) CoverageExists(ctx context.Context, scope ledger.Scope, revision int64) (bool, error) {
	var exists bool
	err := r.tx.QueryRowxContext(ctx, queryCoverageExists, scope.CompanyID, scope.AccountID, revision).Scan(&exists)
	return exists, classify(err)
}
