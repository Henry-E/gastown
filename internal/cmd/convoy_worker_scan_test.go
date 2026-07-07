package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/wisp"
)

// TestGetWorkersForIssues_SkipsParkedAndDockedRigs verifies that the
// parallel worker scan does not fire the unbounded `bd list --type=agent`
// query into rigs that are parked (wisp state) or docked (status:docked
// label on the rig identity bead). The daemon's stranded-convoy scan runs
// this fan-out for every open convoy every cycle, so scanning offline rigs
// is the dominant wasted load on the Dolt server. (See getWorkersForIssues.)
func TestGetWorkersForIssues_SkipsParkedAndDockedRigs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script mock bd not supported on Windows")
	}

	binDir := t.TempDir()
	townRoot := t.TempDir()

	// Town marker so workspace.FindFromCwd resolves townRoot.
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0o755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "town.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}

	// Three rigs, each with the polecats/ and mayor/rig/.beads/ layout the
	// scan discovers.
	for _, rigName := range []string{"liverig", "parkedrig", "dockedrig"} {
		if err := os.MkdirAll(filepath.Join(townRoot, rigName, "polecats"), 0o755); err != nil {
			t.Fatalf("mkdir polecats for %s: %v", rigName, err)
		}
		if err := os.MkdirAll(filepath.Join(townRoot, rigName, "mayor", "rig", ".beads"), 0o755); err != nil {
			t.Fatalf("mkdir .beads for %s: %v", rigName, err)
		}
	}

	// Register rigs with beads prefixes so the docked-label lookup can
	// derive rig bead IDs (lv-rig-liverig, dk-rig-dockedrig, ...).
	rigsCfg := config.RigsConfig{
		Version: config.CurrentRigsVersion,
		Rigs: map[string]config.RigEntry{
			"liverig":   {BeadsConfig: &config.BeadsConfig{Prefix: "lv"}},
			"parkedrig": {BeadsConfig: &config.BeadsConfig{Prefix: "pk"}},
			"dockedrig": {BeadsConfig: &config.BeadsConfig{Prefix: "dk"}},
		},
	}
	rigsData, err := json.Marshal(rigsCfg)
	if err != nil {
		t.Fatalf("marshal rigs.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), rigsData, 0o644); err != nil {
		t.Fatalf("write rigs.json: %v", err)
	}

	// Park "parkedrig" via wisp config (ephemeral parked state — checked
	// locally, before any bd call).
	wispDir := filepath.Join(townRoot, wisp.WispConfigDir, wisp.ConfigSubdir)
	if err := os.MkdirAll(wispDir, 0o755); err != nil {
		t.Fatalf("mkdir wisp config dir: %v", err)
	}
	wispData, _ := json.Marshal(wisp.ConfigFile{
		Rig:    "parkedrig",
		Values: map[string]interface{}{RigStatusKey: RigStatusParked},
	})
	if err := os.WriteFile(filepath.Join(wispDir, "parkedrig.json"), wispData, 0o644); err != nil {
		t.Fatalf("write wisp config: %v", err)
	}

	// Mock bd: logs every invocation (cwd + args), answers rig-bead shows
	// (dockedrig carries status:docked), and returns one agent from the
	// live rig for agent list scans.
	callLog := filepath.Join(binDir, "bd-calls.log")
	script := `#!/bin/sh
echo "$PWD $@" >> "` + callLog + `"

# Find the actual subcommand (skip global flags like --allow-stale)
cmd=""
for arg in "$@"; do
  case "$arg" in
    --*) ;; # skip flags
    *) cmd="$arg"; break ;;
  esac
done

case "$cmd" in
  show)
    case "$*" in
      *dk-rig-dockedrig*)
        echo '[{"id":"dk-rig-dockedrig","labels":["status:docked"]}]'
        ;;
      *lv-rig-liverig*)
        echo '[{"id":"lv-rig-liverig","labels":[]}]'
        ;;
      *)
        echo '[]'
        ;;
    esac
    ;;
  list)
    case "$PWD" in
      */liverig/*)
        echo '[{"id":"gt-liverig-polecat-nux","hook_bead":"hq-cv-test.1","last_activity":""}]'
        ;;
      *)
        echo '[]'
        ;;
    esac
    ;;
  *)
    echo '[]'
    ;;
esac
exit 0
`
	if err := os.WriteFile(filepath.Join(binDir, "bd"), []byte(script), 0o755); err != nil {
		t.Fatalf("write mock bd: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// getWorkersForIssues finds the town root from cwd.
	t.Chdir(townRoot)

	workers := getWorkersForIssues([]string{"hq-cv-test.1"})

	// The live rig's agent must be found.
	worker, ok := workers["hq-cv-test.1"]
	if !ok {
		t.Fatalf("expected a worker for hq-cv-test.1, got %v", workers)
	}
	if worker.Worker != "liverig/polecat/nux" {
		t.Errorf("worker = %q, want %q", worker.Worker, "liverig/polecat/nux")
	}

	// The unbounded agent scan must only have run in the live rig — never
	// in the parked or docked rig.
	logData, err := os.ReadFile(callLog)
	if err != nil {
		t.Fatalf("read bd call log: %v", err)
	}
	var sawLiveList bool
	for _, line := range strings.Split(strings.TrimSpace(string(logData)), "\n") {
		if !strings.Contains(line, " list ") {
			continue
		}
		switch {
		case strings.Contains(line, "/liverig/"):
			sawLiveList = true
		case strings.Contains(line, "/parkedrig/"):
			t.Errorf("agent scan ran in parked rig: %s", line)
		case strings.Contains(line, "/dockedrig/"):
			t.Errorf("agent scan ran in docked rig: %s", line)
		}
	}
	if !sawLiveList {
		t.Errorf("expected an agent scan in the live rig; bd calls:\n%s", logData)
	}
}
