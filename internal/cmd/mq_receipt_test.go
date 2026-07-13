package cmd

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/deliveryreceipt"
	"github.com/steveyegge/gastown/internal/refinery"
	"github.com/steveyegge/gastown/internal/rig"
)

func TestPersistPostMergeReceiptFreshlyReadsRemoteAndReplays(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "receipt-rig")
	remote := filepath.Join(townRoot, "remote.git")
	seed := filepath.Join(townRoot, "seed")
	receiptTestGit(t, townRoot, "init", "--bare", "--initial-branch=main", remote)
	receiptTestGit(t, townRoot, "clone", remote, seed)
	receiptTestGit(t, seed, "config", "user.email", "receipt@example.test")
	receiptTestGit(t, seed, "config", "user.name", "Receipt Test")
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	receiptTestGit(t, seed, "add", "README.md")
	receiptTestGit(t, seed, "commit", "-m", "initial")
	receiptTestGit(t, seed, "push", "origin", "main")
	commit := receiptTestGit(t, seed, "rev-parse", "HEAD")
	if err := os.MkdirAll(rigPath, 0o755); err != nil {
		t.Fatal(err)
	}
	receiptTestGit(t, rigPath, "clone", "--bare", remote, filepath.Join(rigPath, ".repo.git"))

	r := &rig.Rig{Name: "receipt-rig", Path: rigPath, GitURL: remote}
	mr := &refinery.MergeRequest{
		ID:           "ba-mr-receipt",
		Branch:       "polecat/Nux/ba-source",
		IssueID:      "ba-source",
		CommitSHA:    commit,
		AttemptID:    "attempt-receipt-1",
		TargetBranch: "main",
	}
	if receipt, err := validatePostMergeReceipt(townRoot, r.Name, r, mr); err == nil || receipt != nil {
		t.Fatalf("post-merge validation minted proof: receipt=%#v err=%v", receipt, err)
	}
	if _, statErr := os.Stat(deliveryreceipt.StorePath(townRoot)); !os.IsNotExist(statErr) {
		t.Fatalf("post-merge validation created receipt store: %v", statErr)
	}
	if receipt, err := persistMQDeliveryReceipt(townRoot, r.Name, r, mr, strings.Repeat("a", 40), "direct"); err == nil || receipt != nil {
		t.Fatalf("producer accepted wrong merge result: receipt=%#v err=%v", receipt, err)
	}
	if _, statErr := os.Stat(deliveryreceipt.StorePath(townRoot)); !os.IsNotExist(statErr) {
		t.Fatalf("failed producer created receipt store: %v", statErr)
	}

	first, err := persistMQDeliveryReceipt(townRoot, r.Name, r, mr, commit, "direct")
	if err != nil {
		t.Fatalf("persistMQDeliveryReceipt() error = %v", err)
	}
	if first.FinalTargetSHA != commit {
		t.Fatalf("FinalTargetSHA = %s, want remote main %s", first.FinalTargetSHA, commit)
	}
	if _, statErr := os.Stat(deliveryreceipt.StorePath(townRoot)); statErr != nil {
		t.Fatalf("receipt replay log missing: %v", statErr)
	}
	validated, err := validatePostMergeReceipt(townRoot, r.Name, r, mr)
	if err != nil || validated.ReceiptID != first.ReceiptID {
		t.Fatalf("post-merge validation = %#v, %v; want %s", validated, err, first.ReceiptID)
	}
	if outputPath := os.Getenv("GT_DELIVERY_RECEIPT_TEST_OUTPUT"); outputPath != "" {
		data, readErr := os.ReadFile(deliveryreceipt.StorePath(townRoot))
		if readErr != nil {
			t.Fatalf("read cross-repo receipt spool: %v", readErr)
		}
		if writeErr := os.WriteFile(outputPath, data, 0o644); writeErr != nil {
			t.Fatalf("write cross-repo receipt spool: %v", writeErr)
		}
	}

	// Cleanup can be retried after the target advances: the already-durable
	// receipt remains the proof for this attempt instead of conflicting with a
	// later remote SHA.
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("second\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	receiptTestGit(t, seed, "add", "README.md")
	receiptTestGit(t, seed, "commit", "-m", "advance")
	receiptTestGit(t, seed, "push", "origin", "main")
	advancedCommit := receiptTestGit(t, seed, "rev-parse", "HEAD")
	secondAttempt := *mr
	secondAttempt.AttemptID = "attempt-receipt-2"
	if receipt, err := persistMQDeliveryReceipt(townRoot, r.Name, r, &secondAttempt, advancedCommit, "direct"); err == nil || receipt != nil {
		t.Fatalf("producer accepted remote target without matching local merge result: receipt=%#v err=%v", receipt, err)
	}
	prAttempt := *mr
	prAttempt.AttemptID = "attempt-receipt-pr"
	prReceipt, err := persistMQDeliveryReceipt(townRoot, r.Name, r, &prAttempt, advancedCommit, "pr")
	if err != nil {
		t.Fatalf("hosted-PR receipt with fresh remote merge result: %v", err)
	}
	if prReceipt.MergeStrategy != "rebase" || prReceipt.FinalTargetSHA != advancedCommit {
		t.Fatalf("hosted-PR receipt = strategy %s target %s", prReceipt.MergeStrategy, prReceipt.FinalTargetSHA)
	}
	delayedAttempt := *mr
	delayedAttempt.AttemptID = "attempt-receipt-delayed"
	delayed, err := persistMQDeliveryReceipt(townRoot, r.Name, r, &delayedAttempt, commit, "direct")
	if err != nil {
		t.Fatalf("delayed receipt retry after target advance: %v", err)
	}
	if delayed.FinalTargetSHA != commit || !strings.Contains(delayed.VerificationMethod, "after_advance") {
		t.Fatalf("delayed receipt = target %s method %s, want %s with after_advance proof", delayed.FinalTargetSHA, delayed.VerificationMethod, commit)
	}
	validated, err = validatePostMergeReceipt(townRoot, r.Name, r, mr)
	if err != nil || validated.ReceiptID != first.ReceiptID {
		t.Fatalf("advanced-target validation = %#v, %v; want %s", validated, err, first.ReceiptID)
	}
	replayed, err := persistMQDeliveryReceipt(townRoot, r.Name, r, mr, commit, "direct")
	if err != nil {
		t.Fatalf("receipt retry error = %v", err)
	}
	if replayed.ReceiptID != first.ReceiptID || replayed.FinalTargetSHA != first.FinalTargetSHA {
		t.Fatalf("receipt retry = %#v, want original %#v", replayed, first)
	}
}

func TestPostMergeReceiptPersistenceFailureIsClassifiedFailClosed(t *testing.T) {
	townRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(townRoot, "delivery-receipts"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	mr := &refinery.MergeRequest{
		ID:        "ba-mr-receipt",
		Branch:    "polecat/Nux/ba-source",
		IssueID:   "ba-source",
		CommitSHA: strings.Repeat("a", 40),
		AttemptID: "attempt-receipt-1",
	}
	_, err := persistMQDeliveryReceipt(townRoot, "receipt-rig", &rig.Rig{Path: filepath.Join(townRoot, "receipt-rig")}, mr, strings.Repeat("a", 40), "direct")
	if !errors.Is(err, errDeliveryReceiptPersistence) {
		t.Fatalf("error = %v, want errDeliveryReceiptPersistence", err)
	}
	_, err = validatePostMergeReceipt(townRoot, "receipt-rig", &rig.Rig{Path: filepath.Join(townRoot, "receipt-rig")}, mr)
	if !errors.Is(err, errDeliveryReceiptPersistence) {
		t.Fatalf("cleanup validation error = %v, want errDeliveryReceiptPersistence", err)
	}
}

func TestPostMergeReceiptGateIsWarnOnlyUntilOptIn(t *testing.T) {
	t.Setenv("GT_DELIVERY_RECEIPT_ENFORCE", "")
	if postMergeReceiptStrict(&refinery.MergeRequest{}) {
		t.Fatal("legacy MR unexpectedly strict")
	}
	if postMergeReceiptStrict(&refinery.MergeRequest{AttemptID: "attempt-1"}) {
		t.Fatal("receipt-capable MR must remain warn-only before enforcement rollout")
	}
	t.Setenv("GT_DELIVERY_RECEIPT_ENFORCE", "true")
	if !postMergeReceiptStrict(&refinery.MergeRequest{}) {
		t.Fatal("enforcement setting did not make legacy MR strict")
	}
}

func receiptTestGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}
