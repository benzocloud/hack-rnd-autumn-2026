package app

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	appPostgres "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/app/postgres"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/imports"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
	commandsSQL "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres/commands"
	importsSQL "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres/imports"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/imports"
)

type importOperations interface {
	Commit(context.Context, imports.CommitCommand) (imports.CommitResult, error)
	Upload(context.Context, imports.UploadCommand, func() ([]byte, error)) (domain.Accepted, error)
	Read(context.Context, ledger.Scope, uuid.UUID) (domain.Record, error)
	List(context.Context, imports.ListCommand) (imports.ListResult, error)
	Issues(context.Context, imports.IssuesCommand) (imports.IssuesResult, error)
	Run(context.Context) error
	ProcessOne(context.Context) (bool, error)
}

const importCommitTimeout = 90 * time.Second

func newImports(db *sqlx.DB, cfg config.Postgres, manifest ...config.FixtureCoverage) (importOperations, error) {
	entries := make([]imports.CoverageManifestEntry, 0, len(manifest))
	for _, entry := range manifest {
		companyID, companyErr := uuid.Parse(entry.CompanyID)
		accountID, accountErr := uuid.Parse(entry.AccountID)
		if companyErr != nil || accountErr != nil || companyID == uuid.Nil || accountID == uuid.Nil || companyID.String() != entry.CompanyID || accountID.String() != entry.AccountID {
			return nil, errors.New("invalid fixture coverage scope")
		}
		entries = append(entries, imports.CoverageManifestEntry{Scope: ledger.Scope{CompanyID: companyID, AccountID: accountID}, FileSHA256: entry.FileSHA256, SourceCompanyID: entry.SourceCompanyID, FromDate: ledger.Date(entry.FromDate), ThroughDate: ledger.Date(entry.ThroughDate), CutoffDate: ledger.Date(entry.CutoffDate), CutoffBalanceMinor: entry.CutoffBalanceMinor, CutoffRowNumbers: append([]int64(nil), entry.CutoffRowNumbers...)})
	}
	if err := imports.ValidateCoverageManifest(entries); err != nil {
		return nil, errors.New("invalid fixture coverage manifest")
	}
	return imports.New(func(ctx context.Context, work imports.Work) error {
		err := appPostgres.WithinTx(ctx, db, cfg.QueryTimeout, func(ctx context.Context, tx *sqlx.Tx) error {
			return work(ctx, importsSQL.New(tx))
		})
		if errors.Is(err, storage.ErrUnavailable) {
			return storage.WithCause(domain.ErrUnavailable, err)
		}
		return err
	}, imports.WithCommitTransaction(func(ctx context.Context, work imports.CommitWork) error {
		err := appPostgres.WithinTx(ctx, db, importCommitTimeout, func(ctx context.Context, tx *sqlx.Tx) error {
			return work(ctx, importsSQL.New(tx), commandsSQL.New(tx))
		})
		if errors.Is(err, storage.ErrIdempotencyConflict) {
			return storage.WithCause(domain.ErrConflict, err)
		}
		return err
	}), imports.WithCoverageManifest(entries)), nil
}
