package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

func TestUpdateAgentHookBead_SetsHookSlotWithAutoCommit(t *testing.T) {
	tmpDir := t.TempDir()
	townRoot := filepath.Join(tmpDir, "town")
	townBeadsDir := filepath.Join(townRoot, ".beads")
	rigDir := filepath.Join(townRoot, "barnaby", "mayor", "rig")
	rigBeadsDir := filepath.Join(rigDir, ".beads")
	workerDir := filepath.Join(townRoot, "barnaby", "crew", "tom")

	for _, dir := range []string{
		filepath.Join(townRoot, "mayor"),
		townBeadsDir,
		rigBeadsDir,
		workerDir,
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "town.json"), []byte(`{"name":"test-town"}`), 0644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}
	if err := beads.WriteRoutes(townBeadsDir, []beads.Route{
		{Prefix: "hq-", Path: "."},
		{Prefix: "ba-", Path: "barnaby/mayor/rig"},
	}); err != nil {
		t.Fatalf("write routes: %v", err)
	}

	logPath := filepath.Join(tmpDir, "bd.log")
	binDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}

	bdStub := filepath.Join(binDir, "bd")
	stub := `#!/usr/bin/env bash
set -euo pipefail
logfile="$GT_TEST_BD_LOG"
echo "ARGS:$*" >> "$logfile"
echo "AUTO:${BD_DOLT_AUTO_COMMIT:-}" >> "$logfile"
echo "BEADS:${BEADS_DIR:-}" >> "$logfile"
echo "PWD:$(pwd)" >> "$logfile"
exit 0
`
	if err := os.WriteFile(bdStub, []byte(stub), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GT_TEST_BD_LOG", logPath)
	t.Setenv("BD_DOLT_AUTO_COMMIT", "off")
	t.Setenv("BEADS_DIR", "")

	if err := updateAgentHookBead("barnaby/crew/tom", "ba-task-123", workerDir, townBeadsDir); err != nil {
		t.Fatalf("updateAgentHookBead error: %v", err)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	log := string(raw)

	if !strings.Contains(log, "ARGS:slot set ba-barnaby-crew-tom hook ba-task-123") {
		t.Fatalf("slot set not called as expected:\n%s", log)
	}
	if !strings.Contains(log, "AUTO:on") {
		t.Fatalf("expected BD_DOLT_AUTO_COMMIT=on, got:\n%s", log)
	}
	if !strings.Contains(log, "BEADS:"+rigBeadsDir) {
		t.Fatalf("expected BEADS_DIR=%s, got:\n%s", rigBeadsDir, log)
	}
}
