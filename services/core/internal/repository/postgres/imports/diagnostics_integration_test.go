//go:build integration

package imports_test

import (
	"fmt"
	"strings"
	"testing"

	service "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/imports"
)

func TestDuplicateDiagnosticsRemainPagedAndBounded(t *testing.T) {
	_, scope, newService := fixture(t)
	var body strings.Builder
	body.WriteString(csvHeader)
	for index := range 150 {
		_, _ = fmt.Fprintf(&body, "C001;OP%03d;2026-09-30;1.00;Списание;0012345678;name;purpose;1.00\n", index)
	}
	svc := newService()
	cmd := command(scope, body.String())
	accepted, err := svc.Upload(t.Context(), cmd, receive(body.String()))
	if err != nil {
		t.Fatal(err)
	}
	if worked, err := svc.ProcessOne(t.Context()); err != nil || !worked {
		t.Fatal(worked, err)
	}
	record, err := svc.Read(t.Context(), scope, accepted.ID)
	if err != nil || record.Preview == nil || record.Preview.Warnings != 150 || record.Preview.Blockers != 0 || len(record.Preview.Issues) != 50 {
		t.Fatal("duplicate diagnostics did not preserve complete counts", record, err)
	}
	count := 0
	cursor := ""
	for {
		page, err := svc.Issues(t.Context(), service.IssuesCommand{Scope: scope, ActorID: cmd.ActorID, ID: accepted.ID, Limit: 37, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		for _, issue := range page.Items {
			if issue.Code != "SUSPECTED_CONTENT_DUPLICATE" || issue.Severity != "warning" || len(issue.ExternalIDs) != 1 || issue.Row < 2 {
				t.Fatal("diagnostic contains unbounded/noncanonical nested evidence", issue)
			}
			count++
		}
		if page.Cursor == "" {
			break
		}
		cursor = page.Cursor
	}
	if count != 150 {
		t.Fatal("diagnostic paging silently dropped evidence", count)
	}
}

func TestExpiredPreviewKeepsReceiptAndCleanupReleasesBytesOnce(t *testing.T) {
	db, scope, newService := fixture(t)
	svc := newService()
	body := csvHeader + csvRows
	cmd := command(scope, body)
	accepted, err := svc.Upload(t.Context(), cmd, receive(body))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProcessOne(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "UPDATE core_imports SET expires_at=now()-interval '1 minute' WHERE id=$1", accepted.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProcessOne(t.Context()); err != nil {
		t.Fatal(err)
	}
	record, err := svc.Read(t.Context(), scope, accepted.ID)
	if err != nil || record.Status != "failed" || record.ErrorCode != "IMPORT_STATE_CONFLICT" || record.Preview == nil || record.RawDeleted {
		t.Fatal("expiry lost durable preview/receipt or removed bytes prematurely", record, err)
	}
	if _, err := db.ExecContext(t.Context(), "UPDATE core_imports SET terminal_at=now()-interval '25 hours' WHERE id=$1", accepted.ID); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := svc.ProcessOne(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	var chunks int
	if err := db.GetContext(t.Context(), &chunks, "SELECT count(*) FROM core_import_chunks WHERE import_id=$1", accepted.ID); err != nil || chunks != 0 {
		t.Fatal("staging cleanup did not delete raw bytes", chunks, err)
	}
	record, err = svc.Read(t.Context(), scope, accepted.ID)
	if err != nil || !record.RawDeleted || record.Status != "failed" || record.Preview == nil {
		t.Fatal("cleanup changed retained resource", record, err)
	}
	replayed, err := svc.Upload(t.Context(), cmd, receive(body))
	if err != nil || replayed.ID != accepted.ID {
		t.Fatal("cleanup removed original acceptance receipt", replayed, err)
	}
}
