package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// querySessionCostEntriesSince reads session cost entries from the local log file
// and returns entries with ended_at in [since, until].
func querySessionCostEntriesSince(since, until time.Time) ([]CostEntry, error) {
	if !until.IsZero() && until.Before(since) {
		return nil, fmt.Errorf("invalid time range: until before since")
	}

	logPath := getCostsLogPath()
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No log file yet
		}
		return nil, fmt.Errorf("reading costs log: %w", err)
	}

	var entries []CostEntry
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var logEntry CostLogEntry
		if err := json.Unmarshal([]byte(line), &logEntry); err != nil {
			if costsVerbose {
				fmt.Fprintf(os.Stderr, "[costs] failed to parse log entry: %v\n", err)
			}
			continue
		}

		if logEntry.EndedAt.Before(since) {
			continue
		}
		if !until.IsZero() && logEntry.EndedAt.After(until) {
			continue
		}

		entries = append(entries, CostEntry{
			SessionID: logEntry.SessionID,
			Role:      logEntry.Role,
			Rig:       logEntry.Rig,
			Worker:    logEntry.Worker,
			CostUSD:   logEntry.CostUSD,
			EndedAt:   logEntry.EndedAt,
			WorkItem:  logEntry.WorkItem,
		})
	}

	return entries, nil
}
