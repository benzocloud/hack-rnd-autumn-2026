package imports

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/imports"
)

type chunkReader struct {
	ctx          context.Context
	service      *service
	id           uuid.UUID
	count, index int
	pending      []byte
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if len(r.pending) == 0 {
		if r.index >= r.count {
			return 0, io.EOF
		}
		err := r.service.withinTx(r.ctx, func(ctx context.Context, st store) error {
			var err error
			r.pending, err = st.Chunk(ctx, r.id, r.index)
			return err
		})
		if err != nil {
			return 0, err
		}
		if len(r.pending) == 0 {
			return 0, domain.ErrUnavailable
		}
		r.index++
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

func (s *service) Run(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return nil
		}
		_, err := s.ProcessOne(ctx)
		if err != nil && ctx.Err() != nil {
			return nil
		}
		// Transient storage errors leave a durable lease for recovery, not a
		// fabricated failed parse. A later attempt uses a fresh transaction.
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
func (s *service) ProcessOne(ctx context.Context) (bool, error) {
	var record domain.Record
	var found bool
	token, err := uuid.NewV7()
	if err != nil {
		return false, domain.ErrUnavailable
	}
	err = s.withinTx(ctx, func(ctx context.Context, st store) error {
		if err := st.LockQuota(ctx); err != nil {
			return err
		}
		if err := st.Cleanup(ctx, time.Now().UTC()); err != nil {
			return err
		}
		var err error
		record, found, err = st.Claim(ctx, token, time.Now().UTC())
		return err
	})
	if err != nil || !found {
		return false, err
	}
	reader := &chunkReader{ctx: ctx, service: s, id: record.ID, count: record.ChunkCount}
	stats, parseErr := parseDataset(reader, record.Metadata.SourceCompanyID, func(rows []domain.NormalizedRow, issues []domain.Issue) error {
		return s.withinTx(ctx, func(ctx context.Context, st store) error {
			current, err := st.Read(ctx, record.Metadata.Scope, record.ID)
			if err != nil {
				return err
			}
			if current.Status != "processing" || current.Token != token || time.Now().After(current.LeaseUntil) {
				return domain.ErrState
			}
			if err := st.AddRows(ctx, current, rows); err != nil {
				return err
			}
			if err := st.AddIssues(ctx, record.ID, issues); err != nil {
				return err
			}
			current.UpdatedAt = time.Now().UTC()
			current.LeaseUntil = current.UpdatedAt.Add(domain.Lease)
			return st.Save(ctx, current)
		})
	})
	if parseErr != nil && !errors.Is(parseErr, domain.ErrInvalid) && !errors.Is(parseErr, domain.ErrTooLarge) {
		return true, parseErr
	}
	err = s.withinTx(ctx, func(ctx context.Context, st store) error {
		if _, _, err := st.Account(ctx, record.Metadata.Scope); err != nil {
			return err
		}
		current, err := st.Read(ctx, record.Metadata.Scope, record.ID)
		if err != nil {
			return err
		}
		if current.Status != "processing" || current.Token != token || nowAfter(current.LeaseUntil) {
			return domain.ErrState
		}
		now := time.Now().UTC()
		current.UpdatedAt = now
		current.Token = uuid.Nil
		current.LeaseUntil = time.Time{}
		current.Version++
		if parseErr != nil {
			current.Status = "failed"
			current.ErrorCode = "VALIDATION_ERROR"
			if errors.Is(parseErr, domain.ErrTooLarge) {
				current.ErrorCode = "PAYLOAD_TOO_LARGE"
			}
			current.TerminalAt = now
			return st.Save(ctx, current)
		}
		preview, err := st.Summary(ctx, current)
		if err != nil {
			return err
		}
		if preview.PeriodStart == "" {
			current.Status = "failed"
			current.ErrorCode = "VALIDATION_ERROR"
			current.TerminalAt = now
			return st.Save(ctx, current)
		}
		preview.Rows = stats.Rows
		preview.Excluded = stats.Excluded
		for _, entry := range s.manifest {
			if entry.Scope == current.Metadata.Scope && entry.FileSHA256 == current.Metadata.Digest && entry.SourceCompanyID == current.Metadata.SourceCompanyID {
				manual, err := st.ManualHistoryExists(ctx, current.Metadata.Scope, entry.FromDate, entry.ThroughDate)
				if err != nil {
					return err
				}
				if manual {
					break
				}
				preview.Coverage = &domain.HistoryCoverage{Kind: "dataset_fixture_assumption", FromDate: string(entry.FromDate), ThroughDate: string(entry.ThroughDate), Reference: "dataset_fixture_assumption:" + entry.FileSHA256, DataVersion: preview.BaseVersion, Limitations: []string{"dataset_coverage_assumed_not_bank_certified"}}
				break
			}
		}
		var finalIssues []domain.Issue
		binding := preview.Source
		if binding.Mode != "" && (binding.Mode != "imported" || binding.System != "dataset_v1") {
			finalIssues = append(finalIssues, domain.Issue{Code: "FACTUAL_SOURCE_CONFLICT", Severity: "blocking"})
		} else if binding.Mode == "imported" && binding.CompanyID != current.Metadata.SourceCompanyID {
			finalIssues = append(finalIssues, domain.Issue{Code: "SOURCE_BINDING_CONFLICT", Severity: "blocking"})
		}
		if len(preview.Balances) > 1000 {
			preview.Balances = preview.Balances[:1000]
			finalIssues = append(finalIssues, domain.Issue{Code: "INVALID_ROW", Severity: "blocking", Field: "balance_eod"})
		}
		if err := st.AddIssues(ctx, current.ID, finalIssues); err != nil {
			return err
		}
		preview.Blockers += int64(len(finalIssues))
		preview.Issues, _, err = st.Issues(ctx, current.ID, 0, 50)
		if err != nil {
			return err
		}
		current.Preview = &preview
		current.Status = "preview_ready"
		return st.Save(ctx, current)
	})
	return true, err
}

func nowAfter(deadline time.Time) bool { return time.Now().After(deadline) }
