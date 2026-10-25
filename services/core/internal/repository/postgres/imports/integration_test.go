//go:build integration

package imports_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	appPostgres "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/app/postgres"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/imports"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	repository "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres/imports"
	service "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/imports"
)

const csvHeader = "company_id;operation_id;operation_date;amount;direction;counterparty_inn;counterparty_name;payment_purpose;balance_eod\n"
const csvRows = "C001;OP1;2026-09-29;2.00;Поступление;0012345678;name;purpose;12.00\nC001;OP2;2026-09-30;1.00;Списание;;name;;11.00\n"

type importer interface {
	Upload(context.Context, service.UploadCommand, func() ([]byte, error)) (domain.Accepted, error)
	Read(context.Context, ledger.Scope, uuid.UUID) (domain.Record, error)
	List(context.Context, service.ListCommand) (service.ListResult, error)
	Issues(context.Context, service.IssuesCommand) (service.IssuesResult, error)
	ProcessOne(context.Context) (bool, error)
}

func fixture(t *testing.T) (*sqlx.DB, ledger.Scope, func() importer) {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Postgres.Host != "127.0.0.1" || cfg.Postgres.Database != "saldo_core_test" {
		t.Fatal("dedicated loopback saldo_core_test required")
	}
	db, err := appPostgres.Open(t.Context(), cfg.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	var marker string
	if err := db.GetContext(t.Context(), &marker, "SELECT coalesce(current_setting('saldo.test_database',true),'')"); err != nil || marker != "saldo-core-integration" {
		t.Fatal("refusing writes without marked test database", err)
	}
	scope := ledger.Scope{CompanyID: uuid.New(), AccountID: uuid.New()}
	if _, err := db.ExecContext(t.Context(), "INSERT INTO core_companies(id) VALUES($1)", scope.CompanyID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "INSERT INTO core_accounts(company_id,id,currency,data_version) VALUES($1,$2,'RUB',1)", scope.CompanyID, scope.AccountID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
		defer cancel()
		for _, query := range []string{"DELETE FROM core_imports WHERE company_id=$1", "DELETE FROM core_accounts WHERE company_id=$1", "DELETE FROM core_companies WHERE id=$1"} {
			if _, err := db.ExecContext(ctx, query, scope.CompanyID); err != nil {
				t.Error(err)
			}
		}
	})
	newService := func() importer {
		return service.New(func(ctx context.Context, work service.Work) error {
			return appPostgres.WithinTx(ctx, db, 5*time.Second, func(ctx context.Context, tx *sqlx.Tx) error { return work(ctx, repository.New(tx)) })
		})
	}
	return db, scope, newService
}
func command(scope ledger.Scope, body string) service.UploadCommand {
	digest := sha256.Sum256([]byte(body))
	return service.UploadCommand{Scope: scope, ActorID: "test-importer", Key: uuid.NewString(), Adapter: "dataset_v1", SourceCompanyID: "C001", Currency: "RUB", ByteSize: int64(len(body)), Digest: hex.EncodeToString(digest[:])}
}
func receive(body string) func() ([]byte, error) {
	reader := strings.NewReader(body)
	return func() ([]byte, error) { b := make([]byte, 37); n, err := reader.Read(b); return b[:n], err }
}

func TestUploadPreviewRestartReplayTenantAndNoLedgerWrites(t *testing.T) {
	db, scope, newService := fixture(t)
	svc := newService()
	body := csvHeader + csvRows
	cmd := command(scope, body)
	accepted, err := svc.Upload(t.Context(), cmd, receive(body))
	if err != nil {
		t.Fatal(err)
	}
	svc = newService()
	queued, err := svc.Read(t.Context(), scope, accepted.ID)
	if err != nil || queued.Status != "queued" {
		t.Fatal(queued, err)
	}
	if worked, err := svc.ProcessOne(t.Context()); err != nil || !worked {
		t.Fatal(worked, err)
	}
	preview, err := newService().Read(t.Context(), scope, accepted.ID)
	if err != nil || preview.Status != "preview_ready" || preview.Preview == nil || preview.Preview.NewTransactions != 2 || preview.Preview.Blockers != 0 || preview.Preview.BaseVersion != 1 || preview.Preview.WouldBind == nil || len(preview.Preview.Balances) != 2 || preview.Preview.Warnings != 2 {
		t.Fatal(preview, err)
	}
	repeated, err := svc.Upload(t.Context(), cmd, receive(body))
	if err != nil || repeated.ID != accepted.ID {
		t.Fatal(repeated, err)
	}
	cmd.SourceCompanyID = "C002"
	if _, err := svc.Upload(t.Context(), cmd, receive(body)); !errors.Is(err, domain.ErrConflict) {
		t.Fatal(err)
	}
	foreign := scope
	foreign.CompanyID = uuid.New()
	if _, err := svc.Read(t.Context(), foreign, accepted.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal(err)
	}
	var facts, balances, revision int64
	var source *string
	if err := db.QueryRowxContext(t.Context(), "SELECT (SELECT count(*) FROM core_transactions WHERE company_id=$1),(SELECT count(*) FROM core_balances WHERE company_id=$1),data_version,factual_kind FROM core_accounts WHERE company_id=$1", scope.CompanyID).Scan(&facts, &balances, &revision, &source); err != nil || facts != 0 || balances != 0 || revision != 1 || source != nil {
		t.Fatal(facts, balances, revision, source, err)
	}
	issues, err := svc.Issues(t.Context(), service.IssuesCommand{Scope: scope, ActorID: cmd.ActorID, ID: accepted.ID, Limit: 1})
	if err != nil || len(issues.Items) != 1 || issues.Cursor == "" {
		t.Fatal(issues, err)
	}
	if _, err := svc.Issues(t.Context(), service.IssuesCommand{Scope: scope, ActorID: "other", ID: accepted.ID, Cursor: issues.Cursor}); !errors.Is(err, domain.ErrCursor) {
		t.Fatal(err)
	}
}
func TestUploadMalformedStreamsAreNeverAcceptedAndQuotaRecovers(t *testing.T) {
	db, scope, newService := fixture(t)
	svc := newService()
	body := csvHeader + csvRows
	for _, test := range []struct{ name, body, digest string }{{"short", body[:20], ""}, {"hash", body, strings.Repeat("0", 64)}} {
		t.Run(test.name, func(t *testing.T) {
			cmd := command(scope, body)
			if test.digest != "" {
				cmd.Digest = test.digest
			}
			if _, err := svc.Upload(t.Context(), cmd, receive(test.body)); !errors.Is(err, domain.ErrInvalid) {
				t.Fatal(err)
			}
		})
	}
	if _, err := svc.Upload(t.Context(), command(scope, body), receive(body)); !errors.Is(err, domain.ErrQuota) {
		t.Fatal("receiving uploads did not reserve quota", err)
	}
	listed, err := svc.List(t.Context(), service.ListCommand{Scope: scope, ActorID: "test-importer"})
	if err != nil || len(listed.Items) != 0 {
		t.Fatal(listed, err)
	}
	if _, err := db.ExecContext(t.Context(), "UPDATE core_imports SET lease_until=now()-interval '1 minute' WHERE company_id=$1", scope.CompanyID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProcessOne(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Upload(t.Context(), command(scope, body), receive(body)); err != nil {
		t.Fatal("orphan cleanup did not release quota", err)
	}
}
func TestPreviewSourceConflictsAndDurableWorkerRecovery(t *testing.T) {
	db, scope, newService := fixture(t)
	svc := newService()
	body := csvHeader + csvRows
	if _, err := db.ExecContext(t.Context(), "UPDATE core_accounts SET factual_kind='manual' WHERE company_id=$1", scope.CompanyID); err != nil {
		t.Fatal(err)
	}
	accepted, err := svc.Upload(t.Context(), command(scope, body), receive(body))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProcessOne(t.Context()); err != nil {
		t.Fatal(err)
	}
	r, err := svc.Read(t.Context(), scope, accepted.ID)
	if err != nil || r.Preview == nil || r.Preview.Blockers != 1 {
		t.Fatal(r, err)
	}
	// Recovery is tested through the persisted lease representation, not sleeps.
	if _, err := db.ExecContext(t.Context(), "UPDATE core_imports SET status='processing',lease_until=now()-interval '1 minute',payload=jsonb_set(payload,'{Status}','\"processing\"') WHERE id=$1", accepted.ID); err != nil {
		t.Fatal(err)
	}
	if worked, err := newService().ProcessOne(t.Context()); err != nil || !worked {
		t.Fatal(worked, err)
	}
	r, err = svc.Read(t.Context(), scope, accepted.ID)
	if err != nil || r.Status != "preview_ready" || r.Preview.Blockers != 1 {
		t.Fatal(r, err)
	}
}
func TestConcurrentIdenticalUploadsHaveOneDurableAcceptance(t *testing.T) {
	_, scope, newService := fixture(t)
	body := csvHeader + csvRows
	cmd := command(scope, body)
	var wg sync.WaitGroup
	results := make([]domain.Accepted, 8)
	errs := make([]error, 8)
	for i := range results {
		wg.Go(func() { results[i], errs[i] = newService().Upload(t.Context(), cmd, receive(body)) })
	}
	wg.Wait()
	id := uuid.Nil
	for i, err := range errs {
		if err != nil {
			if !errors.Is(err, domain.ErrState) {
				t.Fatal(err)
			}
			continue
		}
		if id == uuid.Nil {
			id = results[i].ID
		}
		if results[i].ID != id {
			t.Fatal("more than one upload accepted")
		}
	}
	if id == uuid.Nil {
		t.Fatal("no acceptance")
	}
	list, err := newService().List(t.Context(), service.ListCommand{Scope: scope, ActorID: cmd.ActorID})
	if err != nil || len(list.Items) != 1 {
		t.Fatal(list, err)
	}
}
