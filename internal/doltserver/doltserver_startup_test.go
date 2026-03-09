package doltserver

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestWaitForServerReady_RefreshesPIDFileUntilReachable(t *testing.T) {
	townRoot := t.TempDir()
	pidFile := filepath.Join(townRoot, "daemon", "dolt.pid")
	if err := os.MkdirAll(filepath.Dir(pidFile), 0755); err != nil {
		t.Fatalf("mkdir daemon: %v", err)
	}

	cmd := exec.Command("sh", "-c", "sleep 2")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start test process: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	// Start with a stale/missing PID state to verify startup refresh logic.
	if err := os.WriteFile(pidFile, []byte("999999"), 0644); err != nil {
		t.Fatalf("write initial pid file: %v", err)
	}

	attempts := 0
	err := waitForServerReady(
		townRoot,
		cmd.Process,
		pidFile,
		cmd.Process.Pid,
		5,
		1*time.Millisecond,
		func(string) error {
			attempts++
			if attempts == 1 {
				_ = os.Remove(pidFile)
				return fmt.Errorf("still warming up")
			}
			if attempts < 3 {
				return fmt.Errorf("still warming up")
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("waitForServerReady returned error: %v", err)
	}

	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read refreshed pid file: %v", err)
	}
	gotPID := strings.TrimSpace(string(data))
	wantPID := strconv.Itoa(cmd.Process.Pid)
	if gotPID != wantPID {
		t.Fatalf("pid file = %q, want %q", gotPID, wantPID)
	}
}

func TestWaitForServerReady_ProcessDies_RemovesPIDAndFails(t *testing.T) {
	townRoot := t.TempDir()
	pidFile := filepath.Join(townRoot, "daemon", "dolt.pid")
	if err := os.MkdirAll(filepath.Dir(pidFile), 0755); err != nil {
		t.Fatalf("mkdir daemon: %v", err)
	}

	cmd := exec.Command("sh", "-c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start test process: %v", err)
	}
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- cmd.Wait()
	}()

	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0644); err != nil {
		t.Fatalf("write initial pid file: %v", err)
	}

	err := waitForServerReady(
		townRoot,
		cmd.Process,
		pidFile,
		cmd.Process.Pid,
		5,
		10*time.Millisecond,
		func(string) error { return fmt.Errorf("not reachable") },
	)
	if err == nil {
		t.Fatal("expected startup failure, got nil")
	}
	if !strings.Contains(err.Error(), "failed to start") {
		t.Fatalf("error = %q, want startup failure", err.Error())
	}

	if _, statErr := os.Stat(pidFile); !os.IsNotExist(statErr) {
		t.Fatalf("expected pid file removed after process death, stat err = %v", statErr)
	}

	select {
	case <-waitDone:
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for short-lived process to exit")
	}
}
