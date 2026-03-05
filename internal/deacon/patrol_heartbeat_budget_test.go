package deacon

import (
	"errors"
	"os"
	"testing"
)

func TestConsumePatrolHeartbeatCredit_BootstrapOnce(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "deacon-budget-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	remaining, err := ConsumePatrolHeartbeatCredit(tmpDir)
	if err != nil {
		t.Fatalf("first consume returned error: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("first consume remaining = %d, want 0", remaining)
	}

	_, err = ConsumePatrolHeartbeatCredit(tmpDir)
	if !errors.Is(err, ErrPatrolHeartbeatBudgetExhausted) {
		t.Fatalf("second consume error = %v, want ErrPatrolHeartbeatBudgetExhausted", err)
	}
}

func TestGrantAndConsumePatrolHeartbeatCredits(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "deacon-budget-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	if err := GrantPatrolHeartbeatCredits(tmpDir, PatrolHeartbeatCreditsPerCycle, "patrol-report"); err != nil {
		t.Fatalf("grant credits: %v", err)
	}

	state, err := LoadPatrolHeartbeatBudget(tmpDir)
	if err != nil {
		t.Fatalf("load budget: %v", err)
	}
	if state.Remaining != PatrolHeartbeatCreditsPerCycle {
		t.Fatalf("remaining = %d, want %d", state.Remaining, PatrolHeartbeatCreditsPerCycle)
	}
	if state.LastGrantBy != "patrol-report" {
		t.Fatalf("last grant by = %q, want %q", state.LastGrantBy, "patrol-report")
	}

	remaining, err := ConsumePatrolHeartbeatCredit(tmpDir)
	if err != nil {
		t.Fatalf("consume after grant: %v", err)
	}
	if remaining != PatrolHeartbeatCreditsPerCycle-1 {
		t.Fatalf("remaining after consume = %d, want %d", remaining, PatrolHeartbeatCreditsPerCycle-1)
	}
}

func TestGrantPatrolHeartbeatCredits_OverwriteNotAccumulate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "deacon-budget-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	if err := GrantPatrolHeartbeatCredits(tmpDir, 5, "manual"); err != nil {
		t.Fatalf("initial grant: %v", err)
	}
	if err := GrantPatrolHeartbeatCredits(tmpDir, 2, "patrol-new"); err != nil {
		t.Fatalf("overwrite grant: %v", err)
	}

	state, err := LoadPatrolHeartbeatBudget(tmpDir)
	if err != nil {
		t.Fatalf("load budget: %v", err)
	}
	if state.Remaining != 2 {
		t.Fatalf("remaining = %d, want 2", state.Remaining)
	}
	if state.LastGrantBy != "patrol-new" {
		t.Fatalf("last grant by = %q, want %q", state.LastGrantBy, "patrol-new")
	}
}
