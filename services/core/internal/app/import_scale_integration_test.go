//go:build integration

package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"testing"
	"time"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/imports"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/imports"
)

// TestImportScaleCeiling measures upload, processing and commit separately.
// The regular suite skips it. Reproduce with SALDO_LARGE_IMPORT=1 on the
// disposable marked database; SALDO_LARGE_IMPORT_ROWS overrides 300000.
func TestImportScaleCeiling(t *testing.T) {
	if os.Getenv("SALDO_LARGE_IMPORT") != "1" {
		t.Skip("set SALDO_LARGE_IMPORT=1 to measure the import ceiling outside the regular suite")
	}
	rows := 300000
	if raw := os.Getenv("SALDO_LARGE_IMPORT_ROWS"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > domain.MaxRows {
			t.Fatalf("SALDO_LARGE_IMPORT_ROWS=%q", raw)
		}
		rows = parsed
	}
	application, cfg := testApplication(t)
	scope := newScope(t, application)
	t.Cleanup(func() {
		ctx, cancel := contextWithTimeout(t, 3*time.Minute)
		defer cancel()
		for _, query := range []string{
			"DELETE FROM core_transactions WHERE company_id=$1",
			"DELETE FROM core_imports WHERE company_id=$1",
		} {
			if _, err := application.db.ExecContext(ctx, query, scope.CompanyID); err != nil {
				t.Error(err)
			}
		}
	})
	var pgVersion string
	if err := application.db.GetContext(t.Context(), &pgVersion, "SELECT version()"); err != nil {
		t.Fatal(err)
	}
	body := scaleCSV(rows)
	digest := sha256.Sum256(body)
	t.Logf("fixture rows=%d bytes=%d go=%s %s/%s cpus=%d postgres=%s query_timeout=%s commit_budget=%s", rows, len(body), runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), pgVersion, cfg.Postgres.QueryTimeout, importCommitTimeout)

	service, err := newImports(application.db, cfg.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(body)
	uploadStarted := time.Now()
	accepted, err := service.Upload(t.Context(), imports.UploadCommand{
		Scope: scope, ActorID: "fixture-owner", Key: "scale-upload-123", Adapter: "dataset_v1",
		SourceCompanyID: "C001", Currency: "RUB", ByteSize: int64(len(body)), Digest: hex.EncodeToString(digest[:]),
	}, func() ([]byte, error) {
		if reader.Len() == 0 {
			return nil, io.EOF
		}
		chunk := make([]byte, min(domain.MaxChunk, reader.Len()))
		if _, err := reader.Read(chunk); err != nil {
			return nil, err
		}
		return chunk, nil
	})
	uploadElapsed := time.Since(uploadStarted)
	if err != nil {
		t.Fatalf("upload after %s: %v", uploadElapsed, err)
	}
	t.Logf("upload %s", uploadElapsed)

	processStarted := time.Now()
	worked, err := service.ProcessOne(t.Context())
	processElapsed := time.Since(processStarted)
	if err != nil || !worked {
		t.Fatalf("processing after %s worked=%t err=%v", processElapsed, worked, err)
	}
	preview, err := service.Read(t.Context(), scope, accepted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Preview == nil || preview.Preview.Rows != int64(rows) || preview.Preview.Blockers != 0 || preview.Status != "preview_ready" {
		t.Fatalf("preview after %s: status=%s preview=%+v", processElapsed, preview.Status, preview.Preview)
	}
	t.Logf("processing %s new=%d warnings=%d", processElapsed, preview.Preview.NewTransactions, preview.Preview.Warnings)

	cmd := imports.CommitCommand{
		Scope: scope, ActorID: "fixture-owner", Key: "scale-commit-123", ID: accepted.ID,
		ExpectedVersion: preview.Version, ExpectedDataVersion: preview.Preview.BaseVersion,
		AcknowledgeWarnings: true, ViewDate: "2026-10-25",
	}
	commitStarted := time.Now()
	result, err := service.Commit(t.Context(), cmd)
	commitElapsed := time.Since(commitStarted)
	if err != nil {
		t.Fatalf("commit after %s (budget %s): %v", commitElapsed, importCommitTimeout, err)
	}
	if commitElapsed >= importCommitTimeout {
		t.Fatalf("commit %s reached budget %s", commitElapsed, importCommitTimeout)
	}
	if result.Import.Status != "applied" || result.Import.AppliedDataVersion != 2 {
		t.Fatalf("commit result status=%s version=%d elapsed=%s", result.Import.Status, result.Import.AppliedDataVersion, commitElapsed)
	}
	var facts int
	if err := application.db.QueryRowxContext(t.Context(), "SELECT count(*) FROM core_transactions WHERE company_id=$1", scope.CompanyID).Scan(&facts); err != nil || facts != rows {
		t.Fatalf("facts=%d rows=%d err=%v", facts, rows, err)
	}
	t.Logf("commit %s facts=%d budget=%s", commitElapsed, facts, importCommitTimeout)
}

func scaleCSV(rows int) []byte {
	var body bytes.Buffer
	body.Grow(rows * 80)
	body.WriteString("company_id;operation_id;operation_date;amount;direction;counterparty_inn;counterparty_name;payment_purpose;balance_eod\n")
	for n := 1; n <= rows; n++ {
		fmt.Fprintf(&body, "C001;OP%07d;2026-09-30;1.00;Поступление;0012345678;name;purpose-%07d;11.00\n", n, n)
	}
	return body.Bytes()
}

func contextWithTimeout(t *testing.T, timeout time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.WithoutCancel(t.Context()), timeout)
}
