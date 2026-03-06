package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseUsageSnapshotPane(t *testing.T) {
	pane := `
Current session usage: 12.5%
Current week usage: 47%
Sonnet-only usage: 3.25%
`
	sessionPct, weekPct, sonnetPct, err := parseUsageSnapshotPane(pane)
	if err != nil {
		t.Fatalf("parseUsageSnapshotPane error: %v", err)
	}
	if sessionPct != 12.5 {
		t.Fatalf("session pct = %v, want 12.5", sessionPct)
	}
	if weekPct != 47 {
		t.Fatalf("week pct = %v, want 47", weekPct)
	}
	if sonnetPct != 3.25 {
		t.Fatalf("sonnet pct = %v, want 3.25", sonnetPct)
	}
}

func TestAggregateUsageFromProjects_ByAgentAndSince(t *testing.T) {
	tmp := t.TempDir()
	projectsRoot := filepath.Join(tmp, ".claude", "projects")
	if err := os.MkdirAll(projectsRoot, 0755); err != nil {
		t.Fatalf("mkdir projects root: %v", err)
	}

	townRoot := filepath.Join(tmp, "town")
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "town.json"), []byte(`{"name":"test-town"}`), 0644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}

	witnessCWD := filepath.Join(townRoot, "barnaby", "witness", "rig")
	crewCWD := filepath.Join(townRoot, "barnaby", "crew", "alice")
	if err := os.MkdirAll(witnessCWD, 0755); err != nil {
		t.Fatalf("mkdir witness cwd: %v", err)
	}
	if err := os.MkdirAll(crewCWD, 0755); err != nil {
		t.Fatalf("mkdir crew cwd: %v", err)
	}

	now := time.Now().UTC()
	recent1 := now.Add(-30 * time.Minute).Format(time.RFC3339Nano)
	recent2 := now.Add(-10 * time.Minute).Format(time.RFC3339Nano)
	old := now.Add(-3 * time.Hour).Format(time.RFC3339Nano)

	transcript1 := filepath.Join(projectsRoot, "proj-a", "a.jsonl")
	transcript2 := filepath.Join(projectsRoot, "proj-b", "b.jsonl")
	if err := os.MkdirAll(filepath.Dir(transcript1), 0755); err != nil {
		t.Fatalf("mkdir proj-a: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(transcript2), 0755); err != nil {
		t.Fatalf("mkdir proj-b: %v", err)
	}

	lines1 := []string{
		`{"type":"assistant","cwd":"` + witnessCWD + `","timestamp":"` + recent1 + `","message":{"role":"assistant","usage":{"input_tokens":100,"output_tokens":20,"cache_read_input_tokens":5,"cache_creation_input_tokens":10}}}`,
		`{"type":"assistant","cwd":"` + witnessCWD + `","timestamp":"` + old + `","message":{"role":"assistant","usage":{"input_tokens":999,"output_tokens":999,"cache_read_input_tokens":999,"cache_creation_input_tokens":999}}}`,
	}
	lines2 := []string{
		`{"type":"assistant","cwd":"` + witnessCWD + `","timestamp":"` + recent2 + `","message":{"role":"assistant","usage":{"input_tokens":200,"output_tokens":40,"cache_read_input_tokens":10,"cache_creation_input_tokens":20}}}`,
		`{"type":"assistant","cwd":"` + crewCWD + `","timestamp":"` + recent2 + `","message":{"role":"assistant","usage":{"input_tokens":50,"output_tokens":5,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
	}
	if err := os.WriteFile(transcript1, []byte(lines1[0]+"\n"+lines1[1]+"\n"), 0644); err != nil {
		t.Fatalf("write transcript1: %v", err)
	}
	if err := os.WriteFile(transcript2, []byte(lines2[0]+"\n"+lines2[1]+"\n"), 0644); err != nil {
		t.Fatalf("write transcript2: %v", err)
	}

	agg, err := aggregateUsageFromProjects(projectsRoot, now.Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("aggregateUsageFromProjects error: %v", err)
	}

	if agg.AssistantMessages != 3 {
		t.Fatalf("assistant messages = %d, want 3", agg.AssistantMessages)
	}

	witness := agg.Agents["barnaby/witness"]
	if witness == nil {
		keys := make([]string, 0, len(agg.Agents))
		for k := range agg.Agents {
			keys = append(keys, k)
		}
		t.Fatalf("missing witness aggregate, keys=%v", keys)
	}
	if got, want := witness.Totals.InputTokens, int64(300); got != want {
		t.Fatalf("witness input = %d, want %d", got, want)
	}
	if got, want := witness.Totals.OutputTokens, int64(60); got != want {
		t.Fatalf("witness output = %d, want %d", got, want)
	}
	if got, want := witness.Totals.CacheReadInputTokens, int64(15); got != want {
		t.Fatalf("witness cache read = %d, want %d", got, want)
	}
	if got, want := witness.Totals.CacheCreationInputTokens, int64(30); got != want {
		t.Fatalf("witness cache create = %d, want %d", got, want)
	}

	crew := agg.Agents["barnaby/crew/alice"]
	if crew == nil {
		t.Fatalf("missing crew aggregate")
	}
	if got, want := crew.Totals.InputTokens, int64(50); got != want {
		t.Fatalf("crew input = %d, want %d", got, want)
	}
}
