package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

func TestRunHookShow_CrossRigFallbackForRigAgent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses bash bd stub")
	}

	townRoot := t.TempDir()
	targetAgent := "ore_v3_smoke_8168/polecats/tom"

	// Minimal workspace marker so findTownRoot()/workspace.FindFromCwd succeeds.
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor"), 0o755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "town.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}

	// Create town + rig beads directories.
	townBeadsDir := filepath.Join(townRoot, ".beads")
	oreRigDir := filepath.Join(townRoot, "ore_v3_smoke_8168", "mayor", "rig")
	barnabyRigDir := filepath.Join(townRoot, "barnaby", "mayor", "rig")
	for _, dir := range []string{
		townBeadsDir,
		filepath.Join(oreRigDir, ".beads"),
		filepath.Join(barnabyRigDir, ".beads"),
		filepath.Join(townRoot, "ore_v3_smoke_8168", "polecats", "tom"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	// Route ba-* to barnaby rig so cross-rig lookup can find the bead.
	routes := []beads.Route{
		{Prefix: "hq-", Path: "."},
		{Prefix: "ore-", Path: "ore_v3_smoke_8168/mayor/rig"},
		{Prefix: "ba-", Path: "barnaby/mayor/rig"},
	}
	if err := beads.WriteRoutes(townBeadsDir, routes); err != nil {
		t.Fatalf("write routes: %v", err)
	}

	// Stub bd: only barnaby rig has a hooked bead for this assignee.
	binDir := t.TempDir()
	barnabyBeadsDir := filepath.Join(barnabyRigDir, ".beads")
	bdScript := `#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "--allow-stale" ]]; then shift; fi
cmd="${1:-}"
shift || true

if [[ "$cmd" == "list" ]]; then
  status=""
  assignee=""
  for arg in "$@"; do
    case "$arg" in
      --status=*) status="${arg#--status=}" ;;
      --assignee=*) assignee="${arg#--assignee=}" ;;
    esac
  done

  if [[ "${BEADS_DIR:-}" == "` + barnabyBeadsDir + `" && "$status" == "hooked" && "$assignee" == "` + targetAgent + `" ]]; then
    echo '[{"id":"ba-715.3","title":"Cross-rig bead","status":"hooked","assignee":"` + targetAgent + `"}]'
    exit 0
  fi

  echo '[]'
  exit 0
fi

echo '[]'
`
	writeBDStub(t, binDir, bdScript, "")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// Ensure gt hook starts from ore rig workspace.
	t.Setenv("BEADS_DIR", filepath.Join(oreRigDir, ".beads"))
	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	polecatDir := filepath.Join(townRoot, "ore_v3_smoke_8168", "polecats", "tom")
	if err := os.Chdir(polecatDir); err != nil {
		t.Fatalf("chdir polecat dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldCwd)
	})

	oldJSON := moleculeJSON
	moleculeJSON = false
	t.Cleanup(func() { moleculeJSON = oldJSON })

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = oldStdout })

	err = runHookShow(nil, []string{targetAgent})
	_ = w.Close()
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()

	if err != nil {
		t.Fatalf("runHookShow returned error: %v", err)
	}

	got := strings.TrimSpace(buf.String())
	if !strings.Contains(got, "ba-715.3") {
		t.Fatalf("expected cross-rig hooked bead in output, got: %q", got)
	}
}
