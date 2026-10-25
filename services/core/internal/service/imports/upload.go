package imports

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/imports"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

type UploadCommand struct {
	Scope                                                    ledger.Scope
	ActorID, Key, Adapter, SourceCompanyID, Currency, Digest string
	ByteSize                                                 int64
}
type service struct {
	withinTx    func(context.Context, Work) error
	commitTx    func(context.Context, CommitWork) error
	manifest    []CoverageManifestEntry
	manifestErr error
}

type Option func(*service)

func New(withinTx func(context.Context, Work) error, options ...Option) *service {
	s := &service{withinTx: withinTx}
	for _, option := range options {
		if option != nil {
			option(s)
		}
	}
	return s
}

func validateUpload(cmd UploadCommand) error {
	if cmd.ByteSize > domain.MaxBytes {
		return domain.ErrTooLarge
	}
	decoded, err := hex.DecodeString(cmd.Digest)
	if cmd.Scope.Validate() != nil || strings.TrimSpace(cmd.ActorID) == "" || cmd.Adapter != "dataset_v1" || cmd.Currency != "RUB" || cmd.ByteSize < 1 || len(cmd.Key) < 8 || len(cmd.Key) > 128 || len(cmd.SourceCompanyID) < 1 || len(cmd.SourceCompanyID) > 128 || !utf8.ValidString(cmd.SourceCompanyID) || err != nil || len(decoded) != 32 || strings.ToLower(cmd.Digest) != cmd.Digest {
		return domain.ErrInvalid
	}
	for _, c := range cmd.Key {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && !strings.ContainsRune("._:-", c) {
			return domain.ErrInvalid
		}
	}
	return nil
}

// Upload drains and verifies even an accepted replay, without storing it twice.
// Receive returns the next payload chunk or EOF; metadata belongs to the adapter.
func (s *service) Upload(ctx context.Context, cmd UploadCommand, receive func() ([]byte, error)) (domain.Accepted, error) {
	if err := validateUpload(cmd); err != nil {
		return domain.Accepted{}, err
	}
	if s.withinTx == nil || receive == nil {
		return domain.Accepted{}, domain.ErrUnavailable
	}
	id, err := uuid.NewV7()
	if err != nil {
		return domain.Accepted{}, domain.ErrUnavailable
	}
	token, err := uuid.NewV7()
	if err != nil {
		return domain.Accepted{}, domain.ErrUnavailable
	}
	now := time.Now().UTC()
	metadata := domain.Metadata{Scope: cmd.Scope, ActorID: cmd.ActorID, Key: cmd.Key, Adapter: cmd.Adapter, SourceCompanyID: cmd.SourceCompanyID, Currency: cmd.Currency, Digest: cmd.Digest, ByteSize: cmd.ByteSize}
	record := domain.Record{ID: id, Metadata: metadata, Token: token, Status: "receiving", Version: 1, CreatedAt: now, UpdatedAt: now, LeaseUntil: now.Add(domain.Lease)}
	replay := false
	err = s.withinTx(ctx, func(ctx context.Context, st store) error {
		if err := st.LockQuota(ctx); err != nil {
			return err
		}
		old, found, err := st.FindKey(ctx, metadata)
		if err != nil {
			return err
		}
		if found {
			if old.Metadata != metadata {
				return domain.ErrConflict
			}
			if old.Status == "receiving" {
				return domain.ErrState
			}
			record, replay = old, true
			return nil
		}
		if _, _, err := st.Account(ctx, cmd.Scope); err != nil {
			return err
		}
		accountBytes, globalBytes, active, err := st.Usage(ctx, cmd.Scope)
		if err != nil {
			return err
		}
		if accountBytes+cmd.ByteSize > 200<<20 || globalBytes+cmd.ByteSize > 1<<30 || active >= 2 {
			return domain.ErrQuota
		}
		return st.Insert(ctx, record)
	})
	if err != nil {
		return domain.Accepted{}, err
	}
	hash := sha256.New()
	var received int64
	count := 0
	for {
		chunk, err := receive()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return domain.Accepted{}, err
		}
		if len(chunk) == 0 || len(chunk) > domain.MaxChunk {
			return domain.Accepted{}, domain.ErrInvalid
		}
		received += int64(len(chunk))
		if received > cmd.ByteSize {
			return domain.Accepted{}, domain.ErrInvalid
		}
		if _, err := hash.Write(chunk); err != nil {
			return domain.Accepted{}, domain.ErrUnavailable
		}
		if !replay {
			err = s.withinTx(ctx, func(ctx context.Context, st store) error {
				current, err := st.Read(ctx, cmd.Scope, record.ID)
				if err != nil {
					return err
				}
				if current.Status != "receiving" || current.Token != token || current.ChunkCount != count || time.Now().After(current.LeaseUntil) {
					return domain.ErrState
				}
				if err := st.AppendChunk(ctx, record.ID, count, chunk); err != nil {
					return err
				}
				current.Received = received
				current.ChunkCount = count + 1
				current.UpdatedAt = time.Now().UTC()
				current.LeaseUntil = current.UpdatedAt.Add(domain.Lease)
				return st.Save(ctx, current)
			})
			if err != nil {
				return domain.Accepted{}, err
			}
		}
		count++
	}
	if received != cmd.ByteSize || hex.EncodeToString(hash.Sum(nil)) != cmd.Digest {
		return domain.Accepted{}, domain.ErrInvalid
	}
	if replay {
		return domain.Accepted{ID: record.ID}, nil
	}
	err = s.withinTx(ctx, func(ctx context.Context, st store) error {
		current, err := st.Read(ctx, cmd.Scope, record.ID)
		if err != nil {
			return err
		}
		if current.Status != "receiving" || current.Token != token || current.Received != received || current.ChunkCount != count || time.Now().After(current.LeaseUntil) {
			return domain.ErrState
		}
		current.Status = "queued"
		current.AcceptedAt = time.Now().UTC()
		current.UpdatedAt = current.AcceptedAt
		current.ExpiresAt = current.AcceptedAt.Add(7 * 24 * time.Hour)
		current.LeaseUntil = time.Time{}
		current.Token = uuid.Nil
		return st.Save(ctx, current)
	})
	if err != nil {
		return domain.Accepted{}, err
	}
	return domain.Accepted{ID: record.ID}, nil
}

func WithCommitTransaction(tx func(context.Context, CommitWork) error) Option {
	return func(s *service) { s.commitTx = tx }
}
func WithCoverageManifest(entries []CoverageManifestEntry) Option {
	return func(s *service) {
		s.manifest = make([]CoverageManifestEntry, len(entries))
		for i, entry := range entries {
			s.manifest[i] = entry
			s.manifest[i].CutoffRowNumbers = append([]int64(nil), entry.CutoffRowNumbers...)
		}
		s.manifestErr = ValidateCoverageManifest(s.manifest)
	}
}
