package imports

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/imports"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

func TestCommitReceiptCodec(t *testing.T) {
	t.Parallel()
	encoded, err := os.ReadFile("testdata/commit_receipt_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct{ Body, Digest, Receipt string }
	if err := json.Unmarshal(encoded, &fixture); err != nil {
		t.Fatal(err)
	}
	cmd := commitReceiptCommand()
	fingerprint := cmd
	fingerprint.ViewDate = ""
	body, err := json.Marshal(fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := json.Marshal(commitReceiptResult())
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	if string(body) != fixture.Body || hex.EncodeToString(sum[:]) != fixture.Digest || string(receipt) != fixture.Receipt {
		t.Fatalf("persisted commit codec changed: body=%s receipt=%s", body, receipt)
	}
	var replay CommitResult
	if json.Unmarshal([]byte(fixture.Receipt), &replay) != nil || replay.Import.ID != cmd.ID || replay.Import.Metadata.Scope != cmd.Scope || replay.Import.Status != "applied" || replay.Import.AppliedDataVersion < 1 {
		t.Fatal("frozen receipt does not satisfy replay")
	}
	var withExtra map[string]any
	if err := json.Unmarshal([]byte(fixture.Receipt), &withExtra); err != nil {
		t.Fatal(err)
	}
	record := withExtra["Import"].(map[string]any)
	record["FutureField"] = "ignored"
	extra, err := json.Marshal(withExtra)
	if err != nil {
		t.Fatal(err)
	}
	replay = CommitResult{}
	if json.Unmarshal(extra, &replay) != nil || replay.Import.Status != "applied" || replay.Import.AppliedDataVersion != 2 {
		t.Fatal("unknown receipt field broke replay", replay)
	}
}

func commitReceiptCommand() CommitCommand {
	return CommitCommand{
		Scope: ledger.Scope{
			CompanyID: uuid.MustParse("019a0c65-9800-7000-8000-000000000001"),
			AccountID: uuid.MustParse("019a0c65-9800-7000-8000-000000000002"),
		},
		ActorID: "fixture-owner", Key: "commit-key-123",
		ID:                  uuid.MustParse("019a0c65-9800-7000-8000-000000000003"),
		ExpectedVersion:     2,
		ExpectedDataVersion: 1,
		AcknowledgeWarnings: true,
		ViewDate:            "2026-10-25",
	}
}

func commitReceiptResult() CommitResult {
	at := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	cmd := commitReceiptCommand()
	return CommitResult{Import: domain.Record{
		ID: cmd.ID,
		Metadata: domain.Metadata{
			Scope: cmd.Scope, ActorID: cmd.ActorID, Key: "upload-key-123", Adapter: "dataset_v1",
			SourceCompanyID: "C001", Currency: "RUB", Digest: "ab", ByteSize: 42,
		},
		Status: "applied", Version: 3, CreatedAt: at, UpdatedAt: at, AcceptedAt: at,
		ExpiresAt: at.Add(7 * 24 * time.Hour), TerminalAt: at, AppliedDataVersion: 2,
		Preview: &domain.Preview{PeriodStart: "2026-09-30", PeriodEnd: "2026-09-30", Rows: 1, BaseVersion: 1},
	}}
}
