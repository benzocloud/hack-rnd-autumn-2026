package invitation

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/domain/account"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/invitation/record"
)

func fromRecord(row record.Invitation) domain.Invitation {
	out := domain.Invitation{
		ID:                  row.ID,
		CompanyID:           row.CompanyID,
		Email:               row.Email,
		CompanyNameSnapshot: row.CompanyNameSnapshot,
		Role:                row.Role,
		Status:              row.Status,
		Version:             row.Version,
		CreatedAt:           row.CreatedAt,
		ExpiresAt:           row.ExpiresAt,
		CreatedBy:           row.CreatedBy,
	}
	if row.TerminalAt.Valid {
		t := row.TerminalAt.Time
		out.TerminalAt = &t
	}
	if row.CreateIdempotencyKey.Valid {
		v := row.CreateIdempotencyKey.String
		out.CreateIdempotencyKey = &v
	}
	if row.CreateRequestSHA256.Valid {
		v := row.CreateRequestSHA256.String
		out.CreateRequestSHA256 = &v
	}
	if row.AcceptIdempotencyKey.Valid {
		v := row.AcceptIdempotencyKey.String
		out.AcceptIdempotencyKey = &v
	}
	if row.AcceptRequestSHA256.Valid {
		v := row.AcceptRequestSHA256.String
		out.AcceptRequestSHA256 = &v
	}
	return out
}

func nullString(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}

func nullTime(value *time.Time) sql.NullTime {
	if value == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *value, Valid: true}
}

func (r *repository) Insert(ctx context.Context, invitation domain.Invitation) error {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, insertSQL,
		invitation.ID, invitation.CompanyID, invitation.Email, invitation.CompanyNameSnapshot, invitation.Role, invitation.Status, invitation.Version,
		invitation.CreatedAt, invitation.ExpiresAt, nullTime(invitation.TerminalAt), invitation.CreatedBy,
		nullString(invitation.CreateIdempotencyKey), nullString(invitation.CreateRequestSHA256),
		nullString(invitation.AcceptIdempotencyKey), nullString(invitation.AcceptRequestSHA256),
	)
	return storageError(err)
}

func (r *repository) Find(ctx context.Context, id uuid.UUID, locked bool) (domain.Invitation, error) {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	query := findSQL
	if locked {
		query = findLockedSQL
	}
	var row record.Invitation
	if err := sqlx.GetContext(ctx, r.db, &row, query, id); err != nil {
		return domain.Invitation{}, storageError(err)
	}
	return fromRecord(row), nil
}

func (r *repository) FindCreateIdempotency(ctx context.Context, actor, company uuid.UUID, key string) (domain.Invitation, error) {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	var row record.Invitation
	if err := sqlx.GetContext(ctx, r.db, &row, findCreateIdempotencySQL, actor, company, key); err != nil {
		return domain.Invitation{}, storageError(err)
	}
	return fromRecord(row), nil
}

func (r *repository) ListOutgoing(ctx context.Context, company, afterID uuid.UUID, retentionCutoff time.Time, afterCreated *time.Time, limit int) ([]domain.Invitation, error) {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	var rows []record.Invitation
	if err := sqlx.SelectContext(ctx, r.db, &rows, listOutgoingSQL, company, retentionCutoff, afterCreated, afterID, limit); err != nil {
		return nil, storageError(err)
	}
	return mapRows(rows), nil
}

func (r *repository) ListIncoming(ctx context.Context, email string, afterID uuid.UUID, retentionCutoff time.Time, afterCreated *time.Time, limit int) ([]domain.Invitation, error) {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	var rows []record.Invitation
	if err := sqlx.SelectContext(ctx, r.db, &rows, listIncomingSQL, email, retentionCutoff, afterCreated, afterID, limit); err != nil {
		return nil, storageError(err)
	}
	return mapRows(rows), nil
}

func (r *repository) Save(ctx context.Context, invitation domain.Invitation, expectedVersion int64) error {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	result, err := r.db.ExecContext(ctx, saveSQL,
		invitation.ID, invitation.Status, invitation.Version, invitation.ExpiresAt, nullTime(invitation.TerminalAt),
		nullString(invitation.AcceptIdempotencyKey), nullString(invitation.AcceptRequestSHA256), expectedVersion,
	)
	if err != nil {
		return storageError(err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return storageError(err)
	}
	if count != 1 {
		return domain.ErrConflict
	}
	return nil
}

func mapRows(rows []record.Invitation) []domain.Invitation {
	result := make([]domain.Invitation, 0, len(rows))
	for _, row := range rows {
		result = append(result, fromRecord(row))
	}
	return result
}
