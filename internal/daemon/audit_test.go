package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogDaemonSignalAudit_WritesDaemonAndTownLogs(t *testing.T) {
	t.Parallel()

	townRoot := t.TempDir()

	logDaemonSignalAudit(townRoot, "gt daemon stop", 4242, "daemon")

	daemonLog, err := os.ReadFile(filepath.Join(townRoot, "daemon", "daemon.log"))
	if err != nil {
		t.Fatalf("read daemon log: %v", err)
	}
	if !strings.Contains(string(daemonLog), "Audit: sending SIGTERM to daemon PID 4242 via gt daemon stop") {
		t.Fatalf("daemon log missing audit line: %s", string(daemonLog))
	}

	townLog, err := os.ReadFile(filepath.Join(townRoot, "logs", "town.log"))
	if err != nil {
		t.Fatalf("read town log: %v", err)
	}
	if !strings.Contains(string(townLog), "[signal] daemon Audit: sending SIGTERM to daemon PID 4242 via gt daemon stop") {
		t.Fatalf("town log missing signal audit line: %s", string(townLog))
	}
}

func TestNormalizeAuditSource_DefaultsBlank(t *testing.T) {
	t.Parallel()

	if got := normalizeAuditSource("   "); got != "unspecified" {
		t.Fatalf("normalizeAuditSource(blank) = %q, want unspecified", got)
	}
}
