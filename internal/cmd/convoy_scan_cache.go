package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/steveyegge/gastown/internal/util"
)

// workerScanCacheTTL bounds reuse of a rig's agent-scan result across convoy
// check processes. The daemon's stranded scan runs `gt convoy check` as a
// separate process per open convoy every cycle (defaultStrandedScanInterval,
// 30s), and each check independently fired the identical unbounded
// `bd list --type=agent` query into every live rig. Caching the raw scan in
// a per-rig file dedupes those queries within a cycle.
//
// SAFETY: the TTL is deliberately strictly less than the 30s daemon cycle so
// stranded-work handling never acts on data older than one cycle. Do not
// raise it to >= the cycle interval.
const workerScanCacheTTL = 25 * time.Second

// workerScanAgent is one agent row from `bd list --type=agent --status=open`.
// It is both the bd JSON shape and the cache file shape.
type workerScanAgent struct {
	ID           string `json:"id"`
	HookBead     string `json:"hook_bead"`
	LastActivity string `json:"last_activity"`
}

// workerScanCacheEntry is the on-disk format for one rig's cached agent scan.
type workerScanCacheEntry struct {
	ScannedAt time.Time         `json:"scanned_at"`
	Agents    []workerScanAgent `json:"agents"`
}

// workerScanCachePath returns the cache file for a rig's agent scan, under
// the town's .runtime state directory (same convention as
// scheduler-state.json, keepalive.json, pids/). Rig names are directory
// basenames, so they are safe as filename components.
func workerScanCachePath(townRoot, rigName string) string {
	return filepath.Join(townRoot, ".runtime", "convoy-scan", rigName+".json")
}

// readWorkerScanCache loads a rig's cached agent scan and reports whether it
// is fresh. Fail open: any read/parse problem, a zero timestamp, a future
// timestamp, or an entry older than workerScanCacheTTL is a miss, and the
// caller falls through to a live scan. A fresh entry with zero agents is a
// valid hit (a rig with no open agent beads).
func readWorkerScanCache(townRoot, rigName string, now time.Time) ([]workerScanAgent, bool) {
	data, err := os.ReadFile(workerScanCachePath(townRoot, rigName))
	if err != nil {
		return nil, false
	}
	var entry workerScanCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}
	if entry.ScannedAt.IsZero() {
		return nil, false
	}
	age := now.Sub(entry.ScannedAt)
	if age < 0 || age >= workerScanCacheTTL {
		return nil, false
	}
	return entry.Agents, true
}

// writeWorkerScanCache persists a rig's agent scan atomically (temp file +
// rename), so a concurrent convoy check never observes a partial write.
// Two checks that both see a stale cache will both scan and both write —
// that race is accepted (both results are live). Failures are swallowed:
// the cache is purely an optimization and the caller already holds the
// live scan result.
func writeWorkerScanCache(townRoot, rigName string, agents []workerScanAgent, scannedAt time.Time) {
	path := workerScanCachePath(townRoot, rigName)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	data, err := json.Marshal(workerScanCacheEntry{
		ScannedAt: scannedAt,
		Agents:    agents,
	})
	if err != nil {
		return
	}
	_ = util.AtomicWriteFile(path, data, 0644)
}
