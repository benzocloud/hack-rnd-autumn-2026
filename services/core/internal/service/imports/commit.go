package imports

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/imports"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

type CoverageManifestEntry struct {
	Scope                             ledger.Scope
	FileSHA256, SourceCompanyID       string
	FromDate, ThroughDate, CutoffDate ledger.Date
	CutoffBalanceMinor                int64
	CutoffRowNumbers                  []int64
}
type CommitCommand struct {
	Scope                                ledger.Scope
	ActorID, Key                         string
	ID                                   uuid.UUID
	ExpectedVersion, ExpectedDataVersion int64
	AcknowledgeWarnings                  bool
	ViewDate                             ledger.Date
}
type CommitResult struct{ Import domain.Record }

func (s *service) Commit(ctx context.Context, cmd CommitCommand) (CommitResult, error) {
	if err := validateCommitCommand(cmd); err != nil {
		return CommitResult{}, domain.ErrInvalid
	}
	if s.manifestErr != nil {
		return CommitResult{}, domain.ErrInvalid
	}
	if s.commitTx == nil {
		return CommitResult{}, domain.ErrUnavailable
	}
	fingerprint := cmd
	fingerprint.ViewDate = ""
	payload, err := json.Marshal(fingerprint)
	if err != nil {
		return CommitResult{}, domain.ErrInvalid
	}
	digest := sha256.Sum256(payload)
	var result CommitResult
	err = s.commitTx(ctx, func(ctx context.Context, st commitStore, commands commandStore) error {
		receipt, replay, err := commands.Reserve(ctx, cmd.Scope, cmd.ActorID, "commit_import_v1", cmd.ID.String(), cmd.Key, digest[:])
		if err != nil {
			return err
		}
		if replay {
			if json.Unmarshal(receipt, &result) != nil || result.Import.ID != cmd.ID || result.Import.Metadata.Scope != cmd.Scope || result.Import.Status != "applied" || result.Import.AppliedDataVersion < 1 {
				return domain.ErrUnavailable
			}
			return nil
		}
		revision, binding, err := st.Account(ctx, cmd.Scope)
		if err != nil {
			return err
		}
		record, err := st.Read(ctx, cmd.Scope, cmd.ID)
		if err != nil {
			return err
		}
		if record.Status != "preview_ready" || record.Preview == nil || !time.Now().Before(record.ExpiresAt) {
			return domain.ErrState
		}
		if record.Version != cmd.ExpectedVersion || revision != cmd.ExpectedDataVersion || record.Preview.BaseVersion != revision {
			return domain.ErrPreviewStale
		}
		if record.Preview.Blockers > 0 {
			return domain.ErrState
		}
		if record.Preview.Warnings > 0 && !cmd.AcknowledgeWarnings {
			return domain.ErrWarnings
		}
		if record.Preview.PeriodEnd > string(cmd.ViewDate) {
			return domain.ErrInvalid
		}
		if binding.Mode != "" && (binding.Mode != "imported" || binding.System != "dataset_v1") {
			return domain.ErrSource
		}
		if binding.Mode == "imported" && binding.CompanyID != record.Metadata.SourceCompanyID {
			return domain.ErrSourceBinding
		}
		categories, err := ledger.DefaultCategories(cmd.Scope.CompanyID)
		if err != nil {
			return domain.ErrUnavailable
		}
		if err := st.SeedCategories(ctx, categories); err != nil {
			return err
		}
		conflict, err := st.CommitConflicts(ctx, record)
		if err != nil {
			return err
		}
		if conflict {
			return domain.ErrState
		}
		newFacts, newBalances, err := st.CountNew(ctx, record)
		if err != nil {
			return err
		}
		next := revision
		if newFacts+newBalances > 0 {
			if revision == math.MaxInt64 {
				return domain.ErrState
			}
			next++
			if err := st.BindSource(ctx, cmd.Scope, record.Metadata.SourceCompanyID); err != nil {
				return err
			}
			if err := s.applyStaged(ctx, st, record, next); err != nil {
				return err
			}
			if err := st.SetRevision(ctx, cmd.Scope, next); err != nil {
				return err
			}
		}
		for _, entry := range s.manifest {
			if entry.Scope != cmd.Scope || entry.FileSHA256 != record.Metadata.Digest || entry.SourceCompanyID != record.Metadata.SourceCompanyID {
				continue
			}
			valid, err := st.VerifyCoverage(ctx, record, entry.FromDate, entry.ThroughDate, entry.CutoffDate, entry.CutoffBalanceMinor, entry.CutoffRowNumbers)
			if err != nil {
				return err
			}
			if !valid {
				return domain.ErrState
			}
			if newFacts+newBalances == 0 {
				break
			}
			exists, err := st.CoverageExists(ctx, cmd.Scope, next)
			if err != nil {
				return err
			}
			if exists {
				break
			}
			proof := domain.HistoryCoverage{Kind: "dataset_fixture_assumption", FromDate: string(entry.FromDate), ThroughDate: string(entry.ThroughDate), Reference: "dataset_fixture_assumption:" + entry.FileSHA256, DataVersion: next, Limitations: []string{"dataset_coverage_assumed_not_bank_certified"}}
			if err := st.InsertCoverage(ctx, cmd.Scope, proof, record.ID, entry.FileSHA256); err != nil {
				return err
			}
			record.Coverage = &proof
			break
		}
		record.Status = "applied"
		record.AppliedDataVersion = next
		record.Version++
		record.UpdatedAt = time.Now().UTC()
		record.TerminalAt = record.UpdatedAt
		if err := st.Save(ctx, record); err != nil {
			return err
		}
		result = CommitResult{Import: record}
		receipt, err = json.Marshal(result)
		if err != nil {
			return domain.ErrUnavailable
		}
		return commands.Complete(ctx, cmd.Scope, cmd.ActorID, "commit_import_v1", cmd.ID.String(), cmd.Key, receipt)
	})
	if err != nil {
		return CommitResult{}, err
	}
	return result, nil
}

func validateCommitCommand(cmd CommitCommand) error {
	if cmd.Scope.Validate() != nil || cmd.ID == uuid.Nil || strings.TrimSpace(cmd.ActorID) == "" || cmd.ExpectedVersion < 1 || cmd.ExpectedDataVersion < 1 || cmd.ViewDate.Validate() != nil || len(cmd.Key) < 8 || len(cmd.Key) > 128 {
		return domain.ErrInvalid
	}
	for _, c := range []byte(cmd.Key) {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && !strings.ContainsRune("._:-", rune(c)) {
			return domain.ErrInvalid
		}
	}
	return nil
}

func ValidateCoverageManifest(entries []CoverageManifestEntry) error {
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		digest, err := hex.DecodeString(entry.FileSHA256)
		if entry.Scope.Validate() != nil || err != nil || len(digest) != sha256.Size || strings.ToLower(entry.FileSHA256) != entry.FileSHA256 || strings.TrimSpace(entry.SourceCompanyID) == "" || len(entry.SourceCompanyID) > 128 || !utf8.ValidString(entry.SourceCompanyID) || entry.FromDate.Validate() != nil || entry.ThroughDate.Validate() != nil || entry.CutoffDate.Validate() != nil || entry.FromDate > entry.CutoffDate || entry.CutoffDate > entry.ThroughDate || len(entry.CutoffRowNumbers) == 0 || len(entry.CutoffRowNumbers) > domain.MaxRows {
			return domain.ErrInvalid
		}
		key := entry.Scope.CompanyID.String() + "/" + entry.Scope.AccountID.String() + "/" + entry.FileSHA256 + "/" + entry.SourceCompanyID
		if seen[key] {
			return domain.ErrInvalid
		}
		seen[key] = true
		rows := make(map[int64]bool, len(entry.CutoffRowNumbers))
		for _, row := range entry.CutoffRowNumbers {
			if row < 2 || row > domain.MaxRows+1 || rows[row] {
				return domain.ErrInvalid
			}
			rows[row] = true
		}
	}
	return nil
}
func (s *service) applyStaged(ctx context.Context, st commitStore, record domain.Record, revision int64) error {
	var after int64
	namespace := ledger.Namespace{System: "dataset_v1", CompanyID: record.Metadata.SourceCompanyID}
	now := time.Now().UTC()
	for {
		rows, err := st.StagedRows(ctx, record.ID, after, 1000)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}
		facts := make([]ledger.Transaction, 0, len(rows))
		for _, row := range rows {
			id, err := uuid.NewV7()
			if err != nil {
				return domain.ErrUnavailable
			}
			amount, err := money.New(row.Amount, money.RUB)
			if err != nil {
				return domain.ErrInvalid
			}
			fact, err := ledger.NewTransaction(ledger.TransactionData{ID: id, Scope: record.Metadata.Scope, Date: ledger.Date(row.Date), Direction: ledger.Direction(row.Direction), Amount: amount, Source: ledger.Source{Kind: ledger.Imported, Namespace: namespace, ExternalID: row.ExternalID, ImportedAt: now}, CounterpartyINN: row.INN, CounterpartyName: row.Name, Purpose: row.Purpose})
			if err != nil {
				return domain.ErrInvalid
			}
			facts = append(facts, fact)
			after = row.Row
		}
		if err := st.InsertFacts(ctx, facts, revision, record.ID); err != nil {
			return err
		}
	}
	for _, balance := range record.Preview.Balances {
		amount, err := money.New(balance.Minor, money.RUB)
		if err != nil {
			return domain.ErrInvalid
		}
		observation, err := ledger.NewBalanceObservation(record.Metadata.Scope, ledger.Date(balance.Date), amount, ledger.BalanceSource{Kind: ledger.StatementEOD, Namespace: namespace, RecordedAt: now})
		if err != nil {
			return domain.ErrInvalid
		}
		if err := st.InsertBalance(ctx, observation, revision); err != nil {
			return err
		}
	}
	return nil
}
