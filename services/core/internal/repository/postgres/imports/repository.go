package imports

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/imports"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
)

type repository struct{ tx *sqlx.Tx }

func New(tx *sqlx.Tx) *repository { return &repository{tx: tx} }
func classify(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return storage.WithCause(domain.ErrNotFound, err)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return storage.WithCause(domain.ErrUnavailable, err)
}
func nullable(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}
func token(id uuid.UUID) any {
	if id == uuid.Nil {
		return nil
	}
	return id
}
func (r *repository) LockQuota(ctx context.Context) error {
	var id int
	return classify(r.tx.QueryRowxContext(ctx, queryLockQuota).Scan(&id))
}
func (r *repository) FindKey(ctx context.Context, m domain.Metadata) (domain.Record, bool, error) {
	var payload []byte
	err := r.tx.QueryRowxContext(ctx, queryFindKey, m.Scope.CompanyID, m.Scope.AccountID, m.ActorID, m.Key).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Record{}, false, nil
	}
	if err != nil {
		return domain.Record{}, false, classify(err)
	}
	var result domain.Record
	if json.Unmarshal(payload, &result) != nil {
		return domain.Record{}, false, domain.ErrUnavailable
	}
	return result, true, nil
}
func (r *repository) Usage(ctx context.Context, scope ledger.Scope) (int64, int64, int, error) {
	var accountBytes, globalBytes int64
	var active int
	err := r.tx.QueryRowxContext(ctx, queryUsage, scope.CompanyID, scope.AccountID).Scan(&accountBytes, &globalBytes, &active)
	return accountBytes, globalBytes, active, classify(err)
}
func (r *repository) Insert(ctx context.Context, record domain.Record) error {
	payload, err := json.Marshal(record)
	if err != nil {
		return domain.ErrUnavailable
	}
	_, err = r.tx.ExecContext(ctx, queryInsert, record.ID, record.Metadata.Scope.CompanyID, record.Metadata.Scope.AccountID, record.Metadata.ActorID, record.Metadata.Key, record.Status, record.Metadata.ByteSize, record.CreatedAt, nullable(record.LeaseUntil), token(record.Token), payload)
	return classify(err)
}
func (r *repository) Read(ctx context.Context, scope ledger.Scope, id uuid.UUID) (domain.Record, error) {
	var payload []byte
	err := r.tx.QueryRowxContext(ctx, queryRead, scope.CompanyID, scope.AccountID, id).Scan(&payload)
	if err != nil {
		return domain.Record{}, classify(err)
	}
	var result domain.Record
	if json.Unmarshal(payload, &result) != nil {
		return domain.Record{}, domain.ErrUnavailable
	}
	return result, nil
}
func (r *repository) Save(ctx context.Context, record domain.Record) error {
	payload, err := json.Marshal(record)
	if err != nil {
		return domain.ErrUnavailable
	}
	_, err = r.tx.ExecContext(ctx, querySave, record.ID, record.Status, nullable(record.LeaseUntil), nullable(record.ExpiresAt), nullable(record.TerminalAt), token(record.Token), record.RawDeleted, payload)
	return classify(err)
}
func (r *repository) AppendChunk(ctx context.Context, id uuid.UUID, ordinal int, b []byte) error {
	_, err := r.tx.ExecContext(ctx, queryAppendChunk, id, ordinal, b)
	return classify(err)
}
func (r *repository) Chunk(ctx context.Context, id uuid.UUID, ordinal int) ([]byte, error) {
	var b []byte
	err := r.tx.QueryRowxContext(ctx, queryChunk, id, ordinal).Scan(&b)
	return b, classify(err)
}
func (r *repository) Account(ctx context.Context, scope ledger.Scope) (int64, domain.Binding, error) {
	var revision int64
	var b domain.Binding
	err := r.tx.QueryRowxContext(ctx, queryAccount, scope.CompanyID, scope.AccountID).Scan(&revision, &b.Mode, &b.System, &b.CompanyID)
	return revision, b, classify(err)
}
func (r *repository) Claim(ctx context.Context, claimToken uuid.UUID, now time.Time) (domain.Record, bool, error) {
	var payload []byte
	err := r.tx.QueryRowxContext(ctx, queryClaim, now).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Record{}, false, nil
	}
	if err != nil {
		return domain.Record{}, false, classify(err)
	}
	var record domain.Record
	if json.Unmarshal(payload, &record) != nil {
		return domain.Record{}, false, domain.ErrUnavailable
	}
	record.Status = "processing"
	record.Token = claimToken
	record.LeaseUntil = now.Add(domain.Lease)
	record.UpdatedAt = now
	if _, err := r.tx.ExecContext(ctx, queryResetRows, record.ID); err != nil {
		return domain.Record{}, false, classify(err)
	}
	if _, err := r.tx.ExecContext(ctx, queryResetIssues, record.ID); err != nil {
		return domain.Record{}, false, classify(err)
	}
	if err := r.Save(ctx, record); err != nil {
		return domain.Record{}, false, err
	}
	return record, true, nil
}
