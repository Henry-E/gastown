package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/wisp"
)

// TestWorkerScanCacheTTL_StrictlyUnderDaemonCycle pins the safety property:
// stranded-work handling must never act on data older than one daemon cycle
// (defaultStrandedScanInterval in internal/daemon, 30s). If you change either
// interval, keep the TTL strictly below the cycle.
func TestWorkerScanCacheTTL_StrictlyUnderDaemonCycle(t *testing.T) {
	const daemonStrandedScanInterval = 30 * time.Second
	if workerScanCacheTTL >= daemonStrandedScanInterval {
		t.Fatalf("workerScanCacheTTL (%v) must be strictly less than the daemon stranded-scan interval (%v)",
			workerScanCacheTTL, daemonStrandedScanInterval)
	}
}

func TestWorkerScanCache_RoundTrip(t *testing.T) {
	townRoot := t.TempDir()
	now := time.Now()

	agents := []workerScanAgent{
		{ID: "gt-liverig-polecat-nux", HookBead: "hq-cv-1", LastActivity: "2026-07-06T10:00:00Z"},
		{ID: "gt-liverig-crew-amber", HookBead: "hq-cv-2"},
	}
	writeWorkerScanCache(townRoot, "liverig", agents, now)

	got, ok := readWorkerScanCache(townRoot, "liverig", now)
	if !ok {
		t.Fatal("expected fresh cache hit immediately after write")
	}
	if len(got) != 2 || got[0].ID != "gt-liverig-polecat-nux" || got[0].HookBead != "hq-cv-1" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestWorkerScanCache_EmptyScanIsAValidHit(t *testing.T) {
	townRoot := t.TempDir()
	now := time.Now()

	writeWorkerScanCache(townRoot, "quietrig", []workerScanAgent{}, now)

	got, ok := readWorkerScanCache(townRoot, "quietrig", now)
	if !ok {
		t.Fatal("a cached empty scan (rig with no open agents) must be a hit")
	}
	if len(got) != 0 {
		t.Errorf("got %d agents, want 0", len(got))
	}
}

func TestWorkerScanCache_Expiry(t *testing.T) {
	townRoot := t.TempDir()
	scannedAt := time.Now()

	writeWorkerScanCache(townRoot, "liverig", []workerScanAgent{{ID: "a"}}, scannedAt)

	// Just under the TTL: hit.
	if _, ok := readWorkerScanCache(townRoot, "liverig", scannedAt.Add(workerScanCacheTTL-time.Millisecond)); !ok {
		t.Error("cache just under TTL should be a hit")
	}
	// At the TTL boundary: miss (strictly less than TTL is fresh).
	if _, ok := readWorkerScanCache(townRoot, "liverig", scannedAt.Add(workerScanCacheTTL)); ok {
		t.Error("cache at exactly the TTL must be stale")
	}
	// Well past: miss.
	if _, ok := readWorkerScanCache(townRoot, "liverig", scannedAt.Add(time.Minute)); ok {
		t.Error("old cache must be stale")
	}
}

func TestWorkerScanCache_FailsOpenOnBadFiles(t *testing.T) {
	townRoot := t.TempDir()
	now := time.Now()
	dir := filepath.Join(townRoot, ".runtime", "convoy-scan")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	cases := map[string]string{
		"corrupt":   "{definitely not json",
		"empty":     "",
		"no-time":   `{"agents": [{"id": "a"}]}`,
		"future":    `{"scanned_at": "` + now.Add(time.Hour).Format(time.RFC3339) + `", "agents": []}`,
		"zero-time": `{"scanned_at": "0001-01-01T00:00:00Z", "agents": []}`,
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(dir, name+".json"), []byte(data), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			if _, ok := readWorkerScanCache(townRoot, name, now); ok {
				t.Errorf("readWorkerScanCache accepted %s cache as fresh — must fail open to a live scan", name)
			}
		})
	}

	t.Run("missing", func(t *testing.T) {
		if _, ok := readWorkerScanCache(townRoot, "no-such-rig", now); ok {
			t.Error("missing cache file must be a miss")
		}
	})
}

// setupWorkerScanCacheTown builds the 3-rig town used by the worker-scan
// tests (same pattern as TestGetWorkersForIssues_SkipsParkedAndDockedRigs):
// a live rig, a parked rig (wisp state), and a docked rig (status:docked
// label), plus a mock bd that logs every invocation. Returns the town root
// and the bd call-log path.
func setupWorkerScanCacheTown(t *testing.T) (townRoot, callLog string) {
	t.Helper()

	binDir := t.TempDir()
	townRoot = t.TempDir()

	// Town marker so workspace.FindFromCwd resolves townRoot.
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0o755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "town.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}

	// Three rigs with the polecats/ and mayor/rig/.beads/ layout the scan discovers.
	for _, rigName := range []string{"liverig", "parkedrig", "dockedrig"} {
		if err := os.MkdirAll(filepath.Join(townRoot, rigName, "polecats"), 0o755); err != nil {
			t.Fatalf("mkdir polecats for %s: %v", rigName, err)
		}
		if err := os.MkdirAll(filepath.Join(townRoot, rigName, "mayor", "rig", ".beads"), 0o755); err != nil {
			t.Fatalf("mkdir .beads for %s: %v", rigName, err)
		}
	}

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

	// Park "parkedrig" via wisp config.
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

	// Mock bd: logs every invocation, answers rig-bead shows (dockedrig
	// carries status:docked), and returns one agent from the live rig.
	callLog = filepath.Join(binDir, "bd-calls.log")
	script := `#!/bin/sh
echo "$PWD $@" >> "` + callLog + `"

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

	return townRoot, callLog
}

// countLiveRigListCalls counts `bd list` invocations that ran in the live rig.
func countLiveRigListCalls(t *testing.T, callLog string) int {
	t.Helper()
	data, err := os.ReadFile(callLog)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatalf("read bd call log: %v", err)
	}
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.Contains(line, " list ") && strings.Contains(line, "/liverig/") {
			count++
		}
	}
	return count
}

// TestGetWorkersForIssues_SharedScanCache verifies the cycle-scoped cache:
// the first call scans the live rig via bd and writes the per-rig cache
// file; a second call within the TTL is served entirely from the cache (no
// new bd list); a corrupted cache falls open to a live scan; an expired
// cache triggers a rescan. Parked/docked rigs are skipped before the cache
// is ever consulted, so they get neither scans nor cache files.
func TestGetWorkersForIssues_SharedScanCache(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script mock bd not supported on Windows")
	}

	townRoot, callLog := setupWorkerScanCacheTown(t)
	t.Chdir(townRoot)

	assertWorker := func(step string) {
		t.Helper()
		workers := getWorkersForIssues([]string{"hq-cv-test.1"})
		worker, ok := workers["hq-cv-test.1"]
		if !ok {
			t.Fatalf("%s: expected a worker for hq-cv-test.1, got %v", step, workers)
		}
		if worker.Worker != "liverig/polecat/nux" {
			t.Errorf("%s: worker = %q, want %q", step, worker.Worker, "liverig/polecat/nux")
		}
	}

	// 1. Cold: live scan runs, cache file appears.
	assertWorker("cold call")
	if got := countLiveRigListCalls(t, callLog); got != 1 {
		t.Fatalf("after cold call: %d live-rig list calls, want 1", got)
	}
	cachePath := workerScanCachePath(townRoot, "liverig")
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("expected cache file after cold call: %v", err)
	}

	// Parked/docked rigs must have neither scans nor cache files.
	for _, rig := range []string{"parkedrig", "dockedrig"} {
		if _, err := os.Stat(workerScanCachePath(townRoot, rig)); !os.IsNotExist(err) {
			t.Errorf("unexpected cache file for %s (stat err: %v) — skip must run before the cache", rig, err)
		}
	}

	// 2. Warm: served from cache, no new bd list.
	assertWorker("warm call")
	if got := countLiveRigListCalls(t, callLog); got != 1 {
		t.Errorf("after warm call: %d live-rig list calls, want 1 (cache should have served)", got)
	}

	// 3. Corrupt cache: falls open to a live scan and rewrites the cache.
	if err := os.WriteFile(cachePath, []byte("{corrupt"), 0o644); err != nil {
		t.Fatalf("corrupt cache: %v", err)
	}
	assertWorker("corrupt-cache call")
	if got := countLiveRigListCalls(t, callLog); got != 2 {
		t.Errorf("after corrupt-cache call: %d live-rig list calls, want 2 (must fail open)", got)
	}
	if data, err := os.ReadFile(cachePath); err != nil || !json.Valid(data) {
		t.Errorf("cache not rewritten after corrupt-cache scan (err=%v)", err)
	}

	// 4. Expired cache: rescans.
	writeWorkerScanCache(townRoot, "liverig",
		[]workerScanAgent{{ID: "gt-liverig-polecat-stale", HookBead: "hq-cv-test.1"}},
		time.Now().Add(-workerScanCacheTTL-time.Second))
	assertWorker("expired-cache call")
	if got := countLiveRigListCalls(t, callLog); got != 3 {
		t.Errorf("after expired-cache call: %d live-rig list calls, want 3 (stale cache must rescan)", got)
	}

	// 5. Fresh cache contents are actually used: seed a fake fresh entry and
	// confirm its (different) worker comes back with no new scan.
	writeWorkerScanCache(townRoot, "liverig",
		[]workerScanAgent{{ID: "gt-liverig-polecat-cached", HookBead: "hq-cv-test.1"}},
		time.Now())
	workers := getWorkersForIssues([]string{"hq-cv-test.1"})
	if worker, ok := workers["hq-cv-test.1"]; !ok || worker.Worker != "liverig/polecat/cached" {
		t.Errorf("cached-entry call: got %v, want worker liverig/polecat/cached", workers)
	}
	if got := countLiveRigListCalls(t, callLog); got != 3 {
		t.Errorf("after cached-entry call: %d live-rig list calls, want 3", got)
	}

	// Throughout: the scan must never have run in parked/docked rigs.
	data, err := os.ReadFile(callLog)
	if err != nil {
		t.Fatalf("read bd call log: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if !strings.Contains(line, " list ") {
			continue
		}
		if strings.Contains(line, "/parkedrig/") || strings.Contains(line, "/dockedrig/") {
			t.Errorf("agent scan ran in parked/docked rig: %s", line)
		}
	}
}
