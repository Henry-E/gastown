package cmd

import (
	"path/filepath"

	"github.com/steveyegge/gastown/internal/beads"
)

// listHookedOrInProgressByAssignee returns hooked work for an assignee, falling
// back to in_progress to handle interrupted sessions.
func listHookedOrInProgressByAssignee(b *beads.Beads, assignee string) ([]*beads.Issue, error) {
	if b == nil || assignee == "" {
		return nil, nil
	}

	hookedBeads, err := b.List(beads.ListOptions{
		Status:   beads.StatusHooked,
		Assignee: assignee,
		Priority: -1,
	})
	if err != nil {
		return nil, err
	}

	if len(hookedBeads) == 0 {
		inProgressBeads, err := b.List(beads.ListOptions{
			Status:   "in_progress",
			Assignee: assignee,
			Priority: -1,
		})
		if err != nil {
			return nil, err
		}
		hookedBeads = inProgressBeads
	}

	return hookedBeads, nil
}

// findAssignedHookedBeads resolves assignee-hooked work across local, town, and
// all routed rig databases.
func findAssignedHookedBeads(local *beads.Beads, townRoot, assignee string) ([]*beads.Issue, error) {
	hookedBeads, err := listHookedOrInProgressByAssignee(local, assignee)
	if err != nil {
		return nil, err
	}
	if len(hookedBeads) > 0 {
		return hookedBeads, nil
	}

	if townRoot == "" {
		return nil, nil
	}

	// Rig-level agents may be hooked to town-level work (hq-* beads).
	if !isTownLevelRole(assignee) {
		townB := beads.New(filepath.Join(townRoot, ".beads"))
		if townHooked, err := listHookedOrInProgressByAssignee(townB, assignee); err == nil && len(townHooked) > 0 {
			return townHooked, nil
		}
	}

	// Cross-rig fallback: assignee may be hooked to work in another rig DB.
	if routedHooked := scanAllRigsForHookedBeads(townRoot, assignee); len(routedHooked) > 0 {
		return routedHooked, nil
	}

	return nil, nil
}

// setAgentHookReference updates agent hook metadata best-effort.
// This updates both the description-backed field and the hook_bead slot.
func setAgentHookReference(agentB *beads.Beads, agentBeadID, hookBeadID string) {
	if agentB == nil || agentBeadID == "" {
		return
	}

	_ = agentB.UpdateAgentDescriptionFields(agentBeadID, beads.AgentFieldUpdates{
		HookBead: &hookBeadID,
	})

	if hookBeadID == "" {
		_, _ = agentB.Run("slot", "clear", agentBeadID, "hook_bead")
		return
	}

	_, _ = agentB.Run("slot", "set", agentBeadID, "hook_bead", hookBeadID)
}

// clearAgentHookReference clears stale hook metadata from an agent bead.
func clearAgentHookReference(agentB *beads.Beads, agentBeadID string) {
	setAgentHookReference(agentB, agentBeadID, "")
}
