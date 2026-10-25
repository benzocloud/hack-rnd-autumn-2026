//go:build integration

package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/imports"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/imports"
	ledgerService "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
)

func TestImportCommitAtomicReplayAndGuards(t *testing.T) {
	application, _ := testApplication(t)
	scope := newScope(t, application)
	t.Cleanup(func() {
		_, err := application.db.ExecContext(context.WithoutCancel(t.Context()), "DELETE FROM core_transactions WHERE company_id=$1", scope.CompanyID)
		if err != nil {
			t.Error(err)
		}
		_, err = application.db.ExecContext(context.WithoutCancel(t.Context()), "DELETE FROM core_history_coverage WHERE company_id=$1", scope.CompanyID)
		if err != nil {
			t.Error(err)
		}
		_, err = application.db.ExecContext(context.WithoutCancel(t.Context()), "DELETE FROM core_imports WHERE company_id=$1", scope.CompanyID)
		if err != nil {
			t.Error(err)
		}
	})
	body := "company_id;operation_id;operation_date;amount;direction;counterparty_inn;counterparty_name;payment_purpose;balance_eod\nC001;OP1;2026-09-29;2.00;Поступление;0012345678;name;purpose;12.00\nC001;OP2;2026-09-30;1.00;Списание;;name;;11.00\n"
	digest := sha256.Sum256([]byte(body))
	reader := strings.NewReader(body)
	accepted, err := application.Imports.Upload(t.Context(), imports.UploadCommand{Scope: scope, ActorID: "actor", Key: "upload-key-123", Adapter: "dataset_v1", SourceCompanyID: "C001", Currency: "RUB", ByteSize: int64(len(body)), Digest: hex.EncodeToString(digest[:])}, func() ([]byte, error) { b := make([]byte, 256); n, err := reader.Read(b); return b[:n], err })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.Imports.ProcessOne(t.Context()); err != nil {
		t.Fatal(err)
	}
	preview, err := application.Imports.Read(t.Context(), scope, accepted.ID)
	if err != nil {
		t.Fatal(err)
	}
	cmd := imports.CommitCommand{Scope: scope, ActorID: "actor", Key: "commit-key-123", ID: accepted.ID, ExpectedVersion: preview.Version, ExpectedDataVersion: 1, ViewDate: "2026-10-25"}
	if _, err := application.Imports.Commit(t.Context(), cmd); !errors.Is(err, domain.ErrWarnings) {
		t.Fatal("missing warning acknowledgement", err)
	}
	assertRevision(t, application, scope, 1)
	cmd.AcknowledgeWarnings = true
	result, err := application.Imports.Commit(t.Context(), cmd)
	if err != nil || result.Import.AppliedDataVersion != 2 || result.Import.Status != "applied" {
		t.Fatal(result, err)
	}
	replay, err := application.Imports.Commit(t.Context(), cmd)
	if err != nil || replay.Import.AppliedDataVersion != 2 {
		t.Fatal(replay, err)
	}
	var facts, balances int
	if err := application.db.QueryRowxContext(t.Context(), "SELECT (SELECT count(*) FROM core_transactions WHERE company_id=$1),(SELECT count(*) FROM core_balances WHERE company_id=$1)", scope.CompanyID).Scan(&facts, &balances); err != nil || facts != 2 || balances != 2 {
		t.Fatal(facts, balances, err)
	}
	assertRevision(t, application, scope, 2)
	cmd.Key = "different-key-123"
	if _, err := application.Imports.Commit(t.Context(), cmd); !errors.Is(err, domain.ErrState) {
		t.Fatal(err)
	}
}

func TestFixtureCoverageRejectsExtraHistoryAndNoOpDoesNotMintProof(t *testing.T) {
	for _, extra := range []bool{false, true} {
		name := "exact no-op"
		if extra {
			name = "unmanifested transaction"
		}
		t.Run(name, func(t *testing.T) {
			application, cfg := testApplication(t)
			scope := newScope(t, application)
			t.Cleanup(func() {
				for _, table := range []string{"core_history_coverage", "core_imports"} {
					if _, err := application.db.ExecContext(context.WithoutCancel(t.Context()), "DELETE FROM "+table+" WHERE company_id=$1", scope.CompanyID); err != nil {
						t.Error(err)
					}
				}
			})

			if _, err := application.db.ExecContext(t.Context(), "INSERT INTO core_source_bindings(company_id,account_id,source_system,source_company_id) VALUES($1,$2,'dataset_v1','C001')", scope.CompanyID, scope.AccountID); err != nil {
				t.Fatal(err)
			}
			if _, err := application.db.ExecContext(t.Context(), "UPDATE core_accounts SET data_version=2,factual_kind='imported',source_system='dataset_v1',source_company_id='C001' WHERE company_id=$1 AND id=$2", scope.CompanyID, scope.AccountID); err != nil {
				t.Fatal(err)
			}
			for _, externalID := range []string{"OP1"} {
				insertImportedFixtureFact(t, application, scope, externalID)
			}
			if extra {
				insertImportedFixtureFact(t, application, scope, "EXTRA")
			}
			if _, err := application.db.ExecContext(t.Context(), "INSERT INTO core_balances(company_id,account_id,balance_date,amount_minor,currency,source_kind,source_system,source_company_id,recorded_at,note,created_data_version) VALUES($1,$2,'2026-09-30',1100,'RUB','statement_eod','dataset_v1','C001',now(),'fixture',2)", scope.CompanyID, scope.AccountID); err != nil {
				t.Fatal(err)
			}

			body := "company_id;operation_id;operation_date;amount;direction;counterparty_inn;counterparty_name;payment_purpose;balance_eod\nC001;OP1;2026-09-30;1.00;Поступление;0012345678;name;purpose;11.00\n"
			digest := sha256.Sum256([]byte(body))
			imports, err := newImports(application.db, cfg.Postgres, config.FixtureCoverage{CompanyID: scope.CompanyID.String(), AccountID: scope.AccountID.String(), FileSHA256: hex.EncodeToString(digest[:]), SourceCompanyID: "C001", FromDate: "2026-09-30", ThroughDate: "2026-09-30", CutoffDate: "2026-09-30", CutoffBalanceMinor: 1100, CutoffRowNumbers: []int64{2}})
			if err != nil {
				t.Fatal(err)
			}
			reader := strings.NewReader(body)
			accepted, err := imports.Upload(t.Context(), importsUploadCommand(scope, digest, body), func() ([]byte, error) {
				buf := make([]byte, 256)
				n, err := reader.Read(buf)
				return buf[:n], err
			})
			if err != nil {
				t.Fatal(err)
			}
			if worked, err := imports.ProcessOne(t.Context()); err != nil || !worked {
				t.Fatal(worked, err)
			}
			preview, err := imports.Read(t.Context(), scope, accepted.ID)
			if err != nil {
				t.Fatal(err)
			}
			_, err = imports.Commit(t.Context(), importsCommitCommand(scope, accepted.ID, preview.Version))
			if extra && !errors.Is(err, domain.ErrState) {
				t.Fatalf("unmanifested account history must reject coverage, got %v", err)
			}
			if !extra && err != nil {
				t.Fatal(err)
			}
			var proofs, facts int
			var revision int64
			if err := application.db.QueryRowxContext(t.Context(), "SELECT (SELECT count(*) FROM core_history_coverage WHERE company_id=$1),(SELECT count(*) FROM core_transactions WHERE company_id=$1),data_version FROM core_accounts WHERE company_id=$1", scope.CompanyID).Scan(&proofs, &facts, &revision); err != nil {
				t.Fatal(err)
			}
			if proofs != 0 || revision != 2 || facts != 1+btoi(extra) {
				t.Fatalf("coverage/commit changed history: proofs=%d revision=%d facts=%d", proofs, revision, facts)
			}
		})
	}
}

func TestImportCommitConcurrentReplayAppliesOnce(t *testing.T) {
	application, cfg := testApplication(t)
	scope := newScope(t, application)
	service, accepted, preview := stageImport(t, application, cfg, scope, validImportBody)
	cmd := imports.CommitCommand{Scope: scope, ActorID: "fixture-owner", Key: "concurrent-commit-123", ID: accepted.ID, ExpectedVersion: preview.Version, ExpectedDataVersion: preview.Preview.BaseVersion, AcknowledgeWarnings: true, ViewDate: "2026-10-25"}
	start := make(chan struct{})
	results := make(chan struct {
		result imports.CommitResult
		err    error
	}, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			result, err := service.Commit(t.Context(), cmd)
			results <- struct {
				result imports.CommitResult
				err    error
			}{result, err}
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	for got := range results {
		if got.err != nil || got.result.Import.Status != "applied" || got.result.Import.AppliedDataVersion != 2 {
			t.Fatalf("concurrent commit result=%+v err=%v", got.result.Import, got.err)
		}
	}
	assertCount(t, application, "SELECT count(*) FROM core_transactions WHERE company_id=$1", scope, 1)
	assertCount(t, application, "SELECT count(*) FROM core_balances WHERE company_id=$1", scope, 1)
	assertCount(t, application, "SELECT count(*) FROM core_command_results WHERE company_id=$1 AND operation='commit_import_v1'", scope, 1)
	assertRevision(t, application, scope, 2)
}

func TestImportCommitCannotCrossCompanyScope(t *testing.T) {
	application, cfg := testApplication(t)
	scope := newScope(t, application)
	foreignScope := newScope(t, application)
	service, accepted, preview := stageImport(t, application, cfg, scope, validImportBody)
	cmd := imports.CommitCommand{Scope: foreignScope, ActorID: "fixture-owner", Key: "foreign-commit-123", ID: accepted.ID, ExpectedVersion: preview.Version, ExpectedDataVersion: 1, AcknowledgeWarnings: true, ViewDate: "2026-10-25"}
	if _, err := service.Commit(t.Context(), cmd); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign-scope commit error=%v", err)
	}
	assertCount(t, application, "SELECT count(*) FROM core_transactions WHERE company_id=$1", scope, 0)
	assertCount(t, application, "SELECT count(*) FROM core_balances WHERE company_id=$1", scope, 0)
	assertCount(t, application, "SELECT count(*) FROM core_history_coverage WHERE company_id=$1", scope, 0)
	assertCount(t, application, "SELECT count(*) FROM core_command_results WHERE company_id=$1 AND operation='commit_import_v1'", foreignScope, 0)
	assertCount(t, application, "SELECT count(*) FROM core_imports WHERE company_id=$1 AND status='preview_ready'", scope, 1)
	assertRevision(t, application, scope, 1)
	assertRevision(t, application, foreignScope, 1)
}

func TestImportCommitDeduplicatesEquivalentSourceRowsAcrossFiles(t *testing.T) {
	for _, test := range []struct {
		name       string
		amount     int64
		wantCommit bool
	}{
		{name: "equivalent aggregate and company files", amount: 100, wantCommit: true},
		{name: "same external ID with changed content", amount: 101, wantCommit: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			application, cfg := testApplication(t)
			scope := newScope(t, application)
			seedImportedSourceFact(t, application, scope, test.amount)
			service, accepted, preview := stageImport(t, application, cfg, scope, validImportBody)
			if test.wantCommit {
				if preview.Preview.NewTransactions != 0 || preview.Preview.AlreadyPresent != 1 || preview.Preview.Blockers != 0 {
					t.Fatalf("equivalent record was not recognized: %+v", preview.Preview)
				}
				cmd := imports.CommitCommand{Scope: scope, ActorID: "fixture-owner", Key: "dedupe-commit-123", ID: accepted.ID, ExpectedVersion: preview.Version, ExpectedDataVersion: preview.Preview.BaseVersion, AcknowledgeWarnings: true, ViewDate: "2026-10-25"}
				result, err := service.Commit(t.Context(), cmd)
				if err != nil || result.Import.Status != "applied" {
					t.Fatalf("equivalent file commit result=%+v err=%v", result.Import, err)
				}
				assertCount(t, application, "SELECT count(*) FROM core_transactions WHERE company_id=$1 AND external_id='OP1'", scope, 1)
				assertCount(t, application, "SELECT count(*) FROM core_balances WHERE company_id=$1", scope, 1)
				assertRevision(t, application, scope, 3)
				return
			}
			if preview.Preview.Blockers == 0 {
				t.Fatal("changed content under an existing external ID was not blocked")
			}
			cmd := imports.CommitCommand{Scope: scope, ActorID: "fixture-owner", Key: "conflict-commit-123", ID: accepted.ID, ExpectedVersion: preview.Version, ExpectedDataVersion: preview.Preview.BaseVersion, AcknowledgeWarnings: true, ViewDate: "2026-10-25"}
			if _, err := service.Commit(t.Context(), cmd); !errors.Is(err, domain.ErrState) {
				t.Fatalf("changed external record commit error=%v", err)
			}
			assertCount(t, application, "SELECT count(*) FROM core_transactions WHERE company_id=$1 AND external_id='OP1'", scope, 1)
			assertCount(t, application, "SELECT count(*) FROM core_balances WHERE company_id=$1", scope, 0)
			assertRevision(t, application, scope, 2)
		})
	}
}

func TestImportCommitRejectsStalePreviewAndManualSourceConflict(t *testing.T) {
	t.Run("stale preview", func(t *testing.T) {
		application, cfg := testApplication(t)
		scope := newScope(t, application)
		service, accepted, preview := stageImport(t, application, cfg, scope, validImportBody)
		if _, err := application.Ledger.RecordManualBalance(t.Context(), ledgerService.ManualBalanceCommand{Scope: scope, Date: "2026-10-24", ViewDate: "2026-10-25", Amount: rub(t, 5000), Note: "counted", ExpectedDataVersion: 1, ActorID: "owner", IdempotencyKey: "stale-balance-123"}); err != nil {
			t.Fatal(err)
		}
		cmd := imports.CommitCommand{Scope: scope, ActorID: "fixture-owner", Key: "stale-commit-123", ID: accepted.ID, ExpectedVersion: preview.Version, ExpectedDataVersion: preview.Preview.BaseVersion, AcknowledgeWarnings: true, ViewDate: "2026-10-25"}
		if _, err := service.Commit(t.Context(), cmd); !errors.Is(err, domain.ErrPreviewStale) {
			t.Fatalf("stale preview commit error=%v", err)
		}
		assertCount(t, application, "SELECT count(*) FROM core_transactions WHERE company_id=$1", scope, 0)
		assertCount(t, application, "SELECT count(*) FROM core_history_coverage WHERE company_id=$1", scope, 0)
		assertCount(t, application, "SELECT count(*) FROM core_imports WHERE company_id=$1 AND status='preview_ready'", scope, 1)
		assertRevision(t, application, scope, 2)
	})

	t.Run("manual source", func(t *testing.T) {
		application, cfg := testApplication(t)
		scope := newScope(t, application)
		if _, err := application.Ledger.CreateTransaction(t.Context(), transactionCommand(t, scope)); err != nil {
			t.Fatal(err)
		}
		service, accepted, preview := stageImport(t, application, cfg, scope, validImportBody)
		if preview.Preview == nil || preview.Preview.Blockers == 0 {
			t.Fatal("manual source conflict was not visible in preview")
		}
		cmd := imports.CommitCommand{Scope: scope, ActorID: "fixture-owner", Key: "manual-commit-123", ID: accepted.ID, ExpectedVersion: preview.Version, ExpectedDataVersion: preview.Preview.BaseVersion, AcknowledgeWarnings: true, ViewDate: "2026-10-25"}
		if _, err := service.Commit(t.Context(), cmd); !errors.Is(err, domain.ErrState) {
			t.Fatalf("manual source conflict commit error=%v", err)
		}
		assertCount(t, application, "SELECT count(*) FROM core_transactions WHERE company_id=$1", scope, 1)
		assertCount(t, application, "SELECT count(*) FROM core_balances WHERE company_id=$1", scope, 0)
		assertCount(t, application, "SELECT count(*) FROM core_history_coverage WHERE company_id=$1", scope, 0)
		assertCount(t, application, "SELECT count(*) FROM core_imports WHERE company_id=$1 AND status='preview_ready'", scope, 1)
		assertRevision(t, application, scope, 2)
	})
}

func TestImportCommitRollsBackFactsWhenEODWriteFails(t *testing.T) {
	application, cfg := testApplication(t)
	scope := newScope(t, application)
	service, accepted, preview := stageImport(t, application, cfg, scope, validImportBody)
	const functionName = "core_test_reject_import_balance"
	const triggerName = "core_test_reject_import_balance"
	if _, err := application.db.ExecContext(t.Context(), "CREATE FUNCTION "+functionName+"() RETURNS trigger LANGUAGE plpgsql AS 'BEGIN RAISE EXCEPTION ''forced integration failure''; END'"); err != nil {
		t.Fatal(err)
	}
	if _, err := application.db.ExecContext(t.Context(), "CREATE TRIGGER "+triggerName+" BEFORE INSERT ON core_balances FOR EACH ROW WHEN (NEW.account_id = '"+scope.AccountID.String()+"'::uuid) EXECUTE FUNCTION "+functionName+"()"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx := context.WithoutCancel(t.Context())
		if _, err := application.db.ExecContext(ctx, "DROP TRIGGER IF EXISTS "+triggerName+" ON core_balances"); err != nil {
			t.Error(err)
		}
		if _, err := application.db.ExecContext(ctx, "DROP FUNCTION IF EXISTS "+functionName+"()"); err != nil {
			t.Error(err)
		}
	})
	cmd := imports.CommitCommand{Scope: scope, ActorID: "fixture-owner", Key: "rollback-commit-123", ID: accepted.ID, ExpectedVersion: preview.Version, ExpectedDataVersion: preview.Preview.BaseVersion, AcknowledgeWarnings: true, ViewDate: "2026-10-25"}
	if _, err := service.Commit(t.Context(), cmd); err == nil {
		t.Fatal("EOD storage failure did not fail commit")
	}
	assertCount(t, application, "SELECT count(*) FROM core_transactions WHERE company_id=$1", scope, 0)
	assertCount(t, application, "SELECT count(*) FROM core_balances WHERE company_id=$1", scope, 0)
	assertCount(t, application, "SELECT count(*) FROM core_source_bindings WHERE company_id=$1", scope, 0)
	assertCount(t, application, "SELECT count(*) FROM core_history_coverage WHERE company_id=$1", scope, 0)
	assertCount(t, application, "SELECT count(*) FROM core_command_results WHERE company_id=$1 AND operation='commit_import_v1'", scope, 0)
	assertCount(t, application, "SELECT count(*) FROM core_imports WHERE company_id=$1 AND status='preview_ready'", scope, 1)
	assertRevision(t, application, scope, 1)
}

const validImportBody = "company_id;operation_id;operation_date;amount;direction;counterparty_inn;counterparty_name;payment_purpose;balance_eod\nC001;OP1;2026-10-24;1.00;Поступление;0012345678;name;purpose;11.00\n"

func stageImport(t *testing.T, application *application, cfg config.Config, scope ledger.Scope, body string, keys ...string) (importOperations, domain.Accepted, domain.Record) {
	t.Helper()
	service, err := newImports(application.db, cfg.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(body))
	reader := strings.NewReader(body)
	command := importsUploadCommand(scope, digest, body)
	if len(keys) != 0 {
		command.Key = keys[0]
	}
	accepted, err := service.Upload(t.Context(), command, func() ([]byte, error) {
		buf := make([]byte, 256)
		n, err := reader.Read(buf)
		return buf[:n], err
	})
	if err != nil {
		t.Fatal(err)
	}
	if worked, err := service.ProcessOne(t.Context()); err != nil || !worked {
		t.Fatalf("import worker: worked=%t err=%v", worked, err)
	}
	preview, err := service.Read(t.Context(), scope, accepted.ID)
	if err != nil {
		t.Fatal(err)
	}
	return service, accepted, preview
}

func seedImportedSourceFact(t *testing.T, application *application, scope ledger.Scope, amount int64) {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.db.ExecContext(t.Context(), "INSERT INTO core_source_bindings(company_id,account_id,source_system,source_company_id) VALUES($1,$2,'dataset_v1','C001')", scope.CompanyID, scope.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := application.db.ExecContext(t.Context(), "UPDATE core_accounts SET data_version=2,factual_kind='imported',source_system='dataset_v1',source_company_id='C001' WHERE company_id=$1 AND id=$2", scope.CompanyID, scope.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := application.db.ExecContext(t.Context(), "INSERT INTO core_transactions(company_id,account_id,id,operation_date,direction,amount_minor,currency,source_kind,source_system,source_company_id,external_id,imported_at,counterparty_inn,counterparty_name,purpose,version,created_data_version) VALUES($1,$2,$3,'2026-10-24','inflow',$4,'RUB','imported','dataset_v1','C001','OP1',now(),'0012345678','name','purpose',1,2)", scope.CompanyID, scope.AccountID, id, amount); err != nil {
		t.Fatal(err)
	}
}

func insertImportedFixtureFact(t *testing.T, application *application, scope ledger.Scope, externalID string) {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.db.ExecContext(t.Context(), "INSERT INTO core_transactions(company_id,account_id,id,operation_date,direction,amount_minor,currency,source_kind,source_system,source_company_id,external_id,imported_at,counterparty_inn,counterparty_name,purpose,version,created_data_version) VALUES($1,$2,$3,'2026-09-30','inflow',100,'RUB','imported','dataset_v1','C001',$4,now(),'0012345678','name','purpose',1,2)", scope.CompanyID, scope.AccountID, id, externalID); err != nil {
		t.Fatal(err)
	}
}

func importsUploadCommand(scope ledger.Scope, digest [32]byte, body string) imports.UploadCommand {
	return imports.UploadCommand{Scope: scope, ActorID: "fixture-owner", Key: "coverage-upload-123", Adapter: "dataset_v1", SourceCompanyID: "C001", Currency: "RUB", Digest: hex.EncodeToString(digest[:]), ByteSize: int64(len(body))}
}

func importsCommitCommand(scope ledger.Scope, id uuid.UUID, version int64) imports.CommitCommand {
	return imports.CommitCommand{Scope: scope, ActorID: "fixture-owner", Key: "coverage-commit-123", ID: id, ExpectedVersion: version, ExpectedDataVersion: 2, AcknowledgeWarnings: true, ViewDate: "2026-10-25"}
}

func btoi(value bool) int {
	if value {
		return 1
	}
	return 0
}
