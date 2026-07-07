package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/util"
)

// mailPreviewCacheTTL bounds how often a status-line refresh may query the
// beads database for a mail preview. Every crew/agent tmux status bar refresh
// used to run the full unbounded mail scan (bd list --label gt:message per
// identity variant, plus CC and wisp queries); with ~7 sessions refreshing
// every few seconds this was roughly half of the sustained Dolt load. A mail
// count in a status bar may be up to ~30s stale — that's explicitly
// acceptable.
const mailPreviewCacheTTL = 30 * time.Second

// mailPreviewCacheEntry is the on-disk format for a cached mail preview.
type mailPreviewCacheEntry struct {
	CachedAt time.Time `json:"cached_at"`
	Unread   int       `json:"unread"`
	Subject  string    `json:"subject"`
}

// mailPreviewCachePath returns the cache file for an identity's mail preview,
// under the town's .runtime state directory (same convention as
// scheduler-state.json, keepalive.json, pids/).
func mailPreviewCachePath(townRoot, identity string) string {
	return filepath.Join(townRoot, ".runtime", "statusline-mail",
		sanitizeIdentityForFilename(identity)+".json")
}

// sanitizeIdentityForFilename maps an agent identity (e.g. "gastown/crew/tom",
// "mayor/") to a safe flat filename component.
func sanitizeIdentityForFilename(identity string) string {
	var b strings.Builder
	for _, r := range identity {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return b.String()
}

// readMailPreviewCache loads a cache entry and reports whether it is fresh.
// Any read/parse problem, a zero timestamp, a future timestamp, or an entry
// older than the TTL is a miss — callers fall through to a live query.
func readMailPreviewCache(path string, now time.Time) (mailPreviewCacheEntry, bool) {
	var entry mailPreviewCacheEntry
	data, err := os.ReadFile(path)
	if err != nil {
		return entry, false
	}
	if err := json.Unmarshal(data, &entry); err != nil {
		return mailPreviewCacheEntry{}, false
	}
	if entry.CachedAt.IsZero() {
		return entry, false
	}
	age := now.Sub(entry.CachedAt)
	if age < 0 || age >= mailPreviewCacheTTL {
		return entry, false
	}
	return entry, true
}

// writeMailPreviewCache persists a cache entry atomically (temp file +
// rename), so concurrent status-line refreshes never observe a partial
// write — a reader sees either the old entry or the new one. Failures are
// swallowed: the cache is purely an optimization and the caller already
// holds the live result.
func writeMailPreviewCache(path string, entry mailPreviewCacheEntry) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_ = util.AtomicWriteFile(path, data, 0644)
}

// getMailPreviewCached returns the unread count and first-unread subject for
// an identity, serving from the per-identity cache file when it is younger
// than mailPreviewCacheTTL and calling fetch (then rewriting the cache)
// otherwise. Two concurrent refreshes that both see a stale cache will both
// fetch and both write — that race is accepted; the atomic write keeps the
// file consistent either way.
func getMailPreviewCached(identity, townRoot string, fetch func() (int, string)) (int, string) {
	now := time.Now()
	path := mailPreviewCachePath(townRoot, identity)
	if entry, ok := readMailPreviewCache(path, now); ok {
		return entry.Unread, entry.Subject
	}
	unread, subject := fetch()
	writeMailPreviewCache(path, mailPreviewCacheEntry{
		CachedAt: now,
		Unread:   unread,
		Subject:  subject,
	})
	return unread, subject
}
