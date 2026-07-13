package deliveryreceipt

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func testReceipt() Receipt {
	return Receipt{
		Rig:                "barnaby",
		Repository:         "https://token-user:secret@example.test/org/repo.git",
		SourceBead:         "ba-source",
		BarnabyJobID:       "67c0efff-d338-4658-9b2b-f7b8a76de56c",
		MRID:               "ba-mr-1",
		AttemptID:          "attempt-1",
		SubmittedBranch:    "polecat/Nux/ba-source",
		SubmittedCommit:    strings.Repeat("a", 40),
		TargetRef:          "main",
		FinalTargetSHA:     strings.Repeat("b", 40),
		MergeStrategy:      "rebase",
		Producer:           "gt.mq.record-delivery",
		VerifiedAt:         time.Date(2026, 7, 13, 12, 34, 56, 123000000, time.UTC),
		VerificationMethod: "git_ls_remote_matches_local_and_supplied_merge_result",
	}
}

func TestAppendIsDurableIdempotentAndFindable(t *testing.T) {
	townRoot := t.TempDir()
	receipt := testReceipt()

	stored, inserted, err := Append(townRoot, receipt)
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if !inserted {
		t.Fatal("first Append() inserted = false, want true")
	}
	if len(stored.ReceiptID) != 64 {
		t.Fatalf("ReceiptID length = %d, want 64", len(stored.ReceiptID))
	}
	if strings.Contains(stored.Repository, "secret") || stored.Repository != "https://example.test/org/repo.git" {
		t.Fatalf("Repository = %q, credentials were not removed", stored.Repository)
	}

	replayed, inserted, err := Append(townRoot, receipt)
	if err != nil {
		t.Fatalf("replay Append() error = %v", err)
	}
	if inserted {
		t.Fatal("replay Append() inserted = true, want false")
	}
	if replayed.ReceiptID != stored.ReceiptID {
		t.Fatalf("replay receipt = %s, want %s", replayed.ReceiptID, stored.ReceiptID)
	}

	found, err := Find(townRoot, stored.Identity())
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if found == nil || found.ReceiptID != stored.ReceiptID {
		t.Fatalf("Find() = %#v, want receipt %s", found, stored.ReceiptID)
	}

	data, err := os.ReadFile(StorePath(townRoot))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if lines := strings.Count(string(data), "\n"); lines != 1 {
		t.Fatalf("receipt lines = %d, want 1", lines)
	}
	var decoded Receipt
	if err := json.Unmarshal(data[:len(data)-1], &decoded); err != nil {
		t.Fatalf("stored line is not JSON: %v", err)
	}
	if decoded.FinalTargetSHA != receipt.FinalTargetSHA {
		t.Fatalf("FinalTargetSHA = %q, want %q", decoded.FinalTargetSHA, receipt.FinalTargetSHA)
	}
	if outputPath := os.Getenv("GT_DELIVERY_RECEIPT_TEST_OUTPUT"); outputPath != "" {
		if err := os.WriteFile(outputPath, data, 0o644); err != nil {
			t.Fatalf("write cross-repo receipt fixture: %v", err)
		}
	}
}

func TestAppendRejectsConflictingAttempt(t *testing.T) {
	townRoot := t.TempDir()
	receipt := testReceipt()
	if _, _, err := Append(townRoot, receipt); err != nil {
		t.Fatalf("first Append() error = %v", err)
	}
	receipt.FinalTargetSHA = strings.Repeat("c", 40)
	if _, _, err := Append(townRoot, receipt); err == nil || !strings.Contains(err.Error(), "conflicting delivery receipt") {
		t.Fatalf("conflicting Append() error = %v", err)
	}
}

func TestFindRejectsPartialReplayLog(t *testing.T) {
	townRoot := t.TempDir()
	if _, _, err := ensureStore(townRoot); err != nil {
		t.Fatalf("ensureStore() error = %v", err)
	}
	if err := os.WriteFile(StorePath(townRoot), []byte(`{"receipt_id":"partial"}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := Find(townRoot, testReceipt().Identity()); err == nil || !strings.Contains(err.Error(), "partial line") {
		t.Fatalf("Find() error = %v, want partial-line error", err)
	}
}
