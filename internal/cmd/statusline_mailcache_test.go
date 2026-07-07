package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestGetMailPreviewCached_MissThenHit(t *testing.T) {
	townRoot := t.TempDir()
	identity := "gastown/crew/tom"

	fetchCalls := 0
	fetch := func() (int, string) {
		fetchCalls++
		return 3, "hello from the mayor"
	}

	// First call: cache missing → fetch runs and the cache is written.
	unread, subject := getMailPreviewCached(identity, townRoot, fetch)
	if unread != 3 || subject != "hello from the mayor" {
		t.Errorf("first call = (%d, %q), want (3, %q)", unread, subject, "hello from the mayor")
	}
	if fetchCalls != 1 {
		t.Fatalf("fetchCalls = %d after first call, want 1", fetchCalls)
	}

	// Second call within the TTL: served from cache, no fetch.
	unread, subject = getMailPreviewCached(identity, townRoot, fetch)
	if unread != 3 || subject != "hello from the mayor" {
		t.Errorf("cached call = (%d, %q), want (3, %q)", unread, subject, "hello from the mayor")
	}
	if fetchCalls != 1 {
		t.Errorf("fetchCalls = %d after cached call, want 1 (cache should have served)", fetchCalls)
	}
}

func TestGetMailPreviewCached_ZeroCountIsCached(t *testing.T) {
	townRoot := t.TempDir()
	identity := "gastown/crew/quiet"

	fetchCalls := 0
	fetch := func() (int, string) {
		fetchCalls++
		return 0, ""
	}

	getMailPreviewCached(identity, townRoot, fetch)
	getMailPreviewCached(identity, townRoot, fetch)
	if fetchCalls != 1 {
		t.Errorf("fetchCalls = %d, want 1 — an empty inbox must be cached too", fetchCalls)
	}
}

func TestGetMailPreviewCached_StaleCacheRefetches(t *testing.T) {
	townRoot := t.TempDir()
	identity := "gastown/crew/tom"
	path := mailPreviewCachePath(townRoot, identity)

	// Seed a cache entry older than the TTL.
	writeMailPreviewCache(path, mailPreviewCacheEntry{
		CachedAt: time.Now().Add(-mailPreviewCacheTTL - time.Second),
		Unread:   9,
		Subject:  "stale",
	})

	fetchCalls := 0
	unread, subject := getMailPreviewCached(identity, townRoot, func() (int, string) {
		fetchCalls++
		return 1, "fresh"
	})
	if fetchCalls != 1 {
		t.Errorf("fetchCalls = %d, want 1 — stale cache must trigger a live query", fetchCalls)
	}
	if unread != 1 || subject != "fresh" {
		t.Errorf("got (%d, %q), want (1, %q)", unread, subject, "fresh")
	}

	// The refetch must have rewritten the cache.
	entry, ok := readMailPreviewCache(path, time.Now())
	if !ok {
		t.Fatal("cache not fresh after refetch")
	}
	if entry.Unread != 1 || entry.Subject != "fresh" {
		t.Errorf("rewritten cache = %+v, want unread=1 subject=fresh", entry)
	}
}

func TestReadMailPreviewCache_CorruptAndInvalid(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	cases := []struct {
		name string
		data string
	}{
		{"corrupt-json", "{not json"},
		{"empty-file", ""},
		{"missing-timestamp", `{"unread": 5, "subject": "x"}`},
		{"future-timestamp", `{"cached_at": "` + now.Add(time.Hour).Format(time.RFC3339) + `", "unread": 5}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name+".json")
			if err := os.WriteFile(path, []byte(tc.data), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			if _, ok := readMailPreviewCache(path, now); ok {
				t.Errorf("readMailPreviewCache accepted %s as fresh", tc.name)
			}
		})
	}

	t.Run("missing-file", func(t *testing.T) {
		if _, ok := readMailPreviewCache(filepath.Join(dir, "nope.json"), now); ok {
			t.Error("readMailPreviewCache reported a missing file as fresh")
		}
	})
}

// TestMailPreviewCache_ConcurrentWritersNeverCorrupt exercises the
// temp+rename atomicity: many concurrent writers and readers must never
// observe a torn file. Readers either miss or parse a complete entry.
func TestMailPreviewCache_ConcurrentWritersNeverCorrupt(t *testing.T) {
	townRoot := t.TempDir()
	path := mailPreviewCachePath(townRoot, "gastown/crew/racer")

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			writeMailPreviewCache(path, mailPreviewCacheEntry{
				CachedAt: time.Now(),
				Unread:   n,
				Subject:  "concurrent",
			})
		}(i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			// A read during racing writes may miss (file not yet created)
			// but must never return a fresh entry with garbage in it.
			if entry, ok := readMailPreviewCache(path, time.Now()); ok {
				if entry.Subject != "concurrent" {
					t.Errorf("read a torn/garbage entry: %+v", entry)
				}
			}
		}()
	}
	wg.Wait()

	// Final state must be a complete, parseable entry.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read final cache: %v", err)
	}
	var entry mailPreviewCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("final cache file corrupt: %v\n%s", err, data)
	}
	if entry.Subject != "concurrent" {
		t.Errorf("final entry = %+v", entry)
	}
}

func TestSanitizeIdentityForFilename(t *testing.T) {
	cases := map[string]string{
		"gastown/crew/tom": "gastown-crew-tom",
		"mayor/":           "mayor-",
		"deacon":           "deacon",
		"rig/polecat/nux":  "rig-polecat-nux",
		"weird id!":        "weird-id-",
	}
	for in, want := range cases {
		if got := sanitizeIdentityForFilename(in); got != want {
			t.Errorf("sanitizeIdentityForFilename(%q) = %q, want %q", in, got, want)
		}
	}
}
