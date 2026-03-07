package daemon

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/townlog"
)

func normalizeAuditSource(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return "unspecified"
	}
	return source
}

func writeDaemonAuditLog(townRoot, message string) {
	logPath := filepath.Join(townRoot, "daemon", "daemon.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		return
	}

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()

	logger := log.New(f, "", log.LstdFlags)
	logger.Print(message)
}

func logDaemonSignalAudit(townRoot, source string, pid int, target string) {
	source = normalizeAuditSource(source)
	message := fmt.Sprintf("Audit: sending SIGTERM to %s PID %d via %s", target, pid, source)

	writeDaemonAuditLog(townRoot, message)
	_ = townlog.NewLogger(townRoot).Log(townlog.EventSignal, "daemon", message)
}
