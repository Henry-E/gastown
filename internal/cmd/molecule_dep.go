package cmd

import (
	"sort"
	"strconv"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
)

// isNonBlockingDepType returns true for dependency types that should NOT block
// molecule step progress. Unknown types default to BLOCKING (safe default).
func isNonBlockingDepType(depType string) bool {
	switch depType {
	case "parent-child",
		"tracks",
		"related",
		"discovered-from",
		"caused-by",
		"validates",
		"relates-to",
		"supersedes":
		return true
	default:
		// "blocks", "", "needs", unknown, etc. are treated as blocking.
		return false
	}
}

// extractStepSequence extracts the numeric step suffix from IDs like "gt-mol.12".
// If the suffix is missing or unparsable, returns a large number so unknowns sort last.
func extractStepSequence(id string) int {
	idx := strings.LastIndex(id, ".")
	if idx < 0 || idx == len(id)-1 {
		return int(^uint(0) >> 1) // MaxInt
	}
	n, err := strconv.Atoi(id[idx+1:])
	if err != nil {
		return int(^uint(0) >> 1) // MaxInt
	}
	return n
}

func sortStepIDsBySequence(ids []string) {
	sort.SliceStable(ids, func(i, j int) bool {
		return extractStepSequence(ids[i]) < extractStepSequence(ids[j])
	})
}

func sortStepsBySequence(steps []*beads.Issue) {
	sort.SliceStable(steps, func(i, j int) bool {
		return extractStepSequence(steps[i].ID) < extractStepSequence(steps[j].ID)
	})
}

