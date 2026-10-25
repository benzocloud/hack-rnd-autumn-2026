package imports

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/imports"
)

func classifyJoin(a, b error) error {
	if a == nil && b == nil {
		return nil
	}
	return classify(errors.Join(a, b))
}

// Caller holds quota lock. Physical deletion and quota visibility change in the
// same transaction, so retry cannot free a reservation twice or before bytes.
func (r *repository) Cleanup(ctx context.Context, now time.Time) error {
	rows, err := r.tx.QueryxContext(ctx, queryCleanup, now)
	if err != nil {
		return classify(err)
	}
	var records []domain.Record
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return classifyJoin(err, rows.Close())
		}
		var record domain.Record
		if json.Unmarshal(payload, &record) != nil {
			return classifyJoin(domain.ErrUnavailable, rows.Close())
		}
		records = append(records, record)
	}
	if err := classifyJoin(rows.Err(), rows.Close()); err != nil {
		return err
	}
	for _, record := range records {
		if record.Status == "receiving" {
			if _, err := r.tx.ExecContext(ctx, queryDeleteReceiving, record.ID); err != nil {
				return classify(err)
			}
			continue
		}
		if record.Status == "preview_ready" {
			record.Status = "failed"
			record.ErrorCode = "IMPORT_STATE_CONFLICT"
			record.TerminalAt = now
			record.UpdatedAt = now
			record.Version++
		} else {
			if _, err := r.tx.ExecContext(ctx, queryDeleteChunks, record.ID); err != nil {
				return classify(err)
			}
			// Staging is the durable bulk. Drop it with the raw file, before
			// raw_deleted stops the byte quota from counting this import.
			if _, err := r.tx.ExecContext(ctx, queryResetRows, record.ID); err != nil {
				return classify(err)
			}
			record.RawDeleted = true
		}
		if err := r.Save(ctx, record); err != nil {
			return err
		}
	}
	return nil
}
