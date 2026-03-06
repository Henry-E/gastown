package cmd

import (
	"encoding/json"
	"testing"

	"github.com/steveyegge/gastown/internal/refinery"
	"github.com/steveyegge/gastown/internal/session"
)

func TestResolveRigNameOrPrefix(t *testing.T) {
	registry := session.NewPrefixRegistry()
	registry.Register("gt", "gastown")
	registry.Register("bd", "beads")

	rigExists := func(name string) bool {
		return name == "gastown" || name == "beads"
	}

	tests := []struct {
		name       string
		input      string
		wantRig    string
		wantPrefix string
		wantErr    bool
	}{
		{name: "rig name", input: "gastown", wantRig: "gastown", wantPrefix: "gt"},
		{name: "prefix", input: "gt", wantRig: "gastown", wantPrefix: "gt"},
		{name: "second rig prefix", input: "bd", wantRig: "beads", wantPrefix: "bd"},
		{name: "unknown", input: "zzz", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotRig, gotPrefix, err := resolveRigNameOrPrefix(tc.input, registry, rigExists)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveRigNameOrPrefix(%q) expected error, got nil", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveRigNameOrPrefix(%q) error = %v", tc.input, err)
			}
			if gotRig != tc.wantRig {
				t.Fatalf("rig = %q, want %q", gotRig, tc.wantRig)
			}
			if gotPrefix != tc.wantPrefix {
				t.Fatalf("prefix = %q, want %q", gotPrefix, tc.wantPrefix)
			}
		})
	}
}

func TestWitnessCheckOutputJSONSchema(t *testing.T) {
	payload := WitnessCheckOutput{
		CheckedAt: "2026-03-06T12:00:00Z",
		Rig:       "gastown",
		Prefix:    "gt",
		Refinery: WitnessCheckRefinery{
			Session: "gt-refinery",
			Running: true,
		},
		Deacon: WitnessCheckDeacon{
			Session:                         "hq-deacon",
			Running:                         true,
			GraceWindowSeconds:              300,
			RecommendedRecheckSeconds:       30,
			WithinGraceWindow:               false,
			HeartbeatStaleThresholdSeconds:  300,
			HeartbeatVeryStaleThresholdSecs: 900,
			Heartbeat:                       &WitnessCheckHeartbeat{Timestamp: "2026-03-06T11:59:00Z", AgeSeconds: 60, Stale: false, VeryStale: false, Cycle: 42},
		},
		Queue: WitnessCheckQueueHealth{
			Summary: WitnessCheckQueueSummary{
				OpenTotal:           2,
				ReadyCount:          1,
				BlockedCount:        1,
				ClaimedCount:        1,
				UnclaimedCount:      1,
				AnomalyCount:        0,
				OrphanedBranchCount: 0,
			},
			Open: []*refinery.MRInfo{
				{ID: "gt-1", Branch: "polecat/x", Target: "main"},
			},
			Ready: []*refinery.MRInfo{
				{ID: "gt-1", Branch: "polecat/x", Target: "main"},
			},
		},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	for _, key := range []string{"checked_at", "rig", "prefix", "refinery", "deacon", "queue"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("missing top-level key %q in JSON payload", key)
		}
	}

	queueVal, ok := decoded["queue"].(map[string]any)
	if !ok {
		t.Fatalf("queue is %T, want object", decoded["queue"])
	}
	summaryVal, ok := queueVal["summary"].(map[string]any)
	if !ok {
		t.Fatalf("queue.summary is %T, want object", queueVal["summary"])
	}
	if _, ok := summaryVal["open_total"]; !ok {
		t.Fatalf("missing queue.summary.open_total")
	}
}

func TestBuildWitnessQueueSummary(t *testing.T) {
	open := []*refinery.MRInfo{
		{ID: "gt-1", Assignee: "worker-1", BranchExistsLocal: true, BranchExistsRemote: true},
		{ID: "gt-2", Assignee: "", BranchExistsLocal: false, BranchExistsRemote: false},
	}
	ready := []*refinery.MRInfo{{ID: "gt-2"}}
	blocked := []*refinery.MRInfo{{ID: "gt-1"}}
	anomalies := []*refinery.MRAnomaly{{ID: "gt-1"}}

	summary := buildWitnessQueueSummary(open, ready, blocked, anomalies)
	if summary.OpenTotal != 2 {
		t.Fatalf("OpenTotal = %d, want 2", summary.OpenTotal)
	}
	if summary.ClaimedCount != 1 || summary.UnclaimedCount != 1 {
		t.Fatalf("claimed/unclaimed = %d/%d, want 1/1", summary.ClaimedCount, summary.UnclaimedCount)
	}
	if summary.OrphanedBranchCount != 1 {
		t.Fatalf("OrphanedBranchCount = %d, want 1", summary.OrphanedBranchCount)
	}
}
