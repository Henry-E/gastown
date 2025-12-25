// Package wisp provides ephemeral molecule support for Gas Town agents.
//
// Wisps are short-lived workflow state that lives in .beads-wisp/ and is
// never git-tracked. They are used for:
//   - Slung work: attaching a bead to an agent's hook for restart-and-resume
//   - Patrol cycles: ephemeral state for continuous loops (Deacon, Witness, etc)
//
// Unlike regular molecules in .beads/, wisps are burned after use.
package wisp

import (
	"strings"
	"time"
)

// WispType identifies the kind of wisp.
type WispType string

const (
	// TypeSlungWork is a wisp that attaches a bead to an agent's hook.
	// Created by `gt sling <bead-id>` and burned after pickup.
	TypeSlungWork WispType = "slung-work"

	// TypePatrolCycle is a wisp tracking patrol execution state.
	// Used by Deacon, Witness, Refinery for their continuous loops.
	TypePatrolCycle WispType = "patrol-cycle"
)

// WispDir is the directory name for ephemeral wisps (not git-tracked).
const WispDir = ".beads-wisp"

// HookPrefix is the filename prefix for hook files.
const HookPrefix = "hook-"

// HookSuffix is the filename suffix for hook files.
const HookSuffix = ".json"

// Wisp is the common header for all wisp types.
type Wisp struct {
	// Type identifies what kind of wisp this is.
	Type WispType `json:"type"`

	// CreatedAt is when the wisp was created.
	CreatedAt time.Time `json:"created_at"`

	// CreatedBy identifies who created the wisp (e.g., "crew/joe", "deacon").
	CreatedBy string `json:"created_by"`
}

// SlungWork represents work attached to an agent's hook.
// Created by `gt sling` and burned after the agent picks it up.
type SlungWork struct {
	Wisp

	// BeadID is the issue/bead to work on (e.g., "gt-xxx").
	BeadID string `json:"bead_id"`

	// Context is optional additional context from the slinger.
	Context string `json:"context,omitempty"`

	// Subject is optional subject line (used in handoff mail).
	Subject string `json:"subject,omitempty"`
}

// PatrolCycle represents the execution state of a patrol loop.
// Used by roles that run continuous patrols (Deacon, Witness, Refinery).
type PatrolCycle struct {
	Wisp

	// Formula is the patrol formula being executed (e.g., "mol-deacon-patrol").
	Formula string `json:"formula"`

	// CurrentStep is the ID of the step currently being executed.
	CurrentStep string `json:"current_step"`

	// StepStates tracks completion state of each step.
	StepStates map[string]StepState `json:"step_states,omitempty"`

	// CycleCount tracks how many complete cycles have been run.
	CycleCount int `json:"cycle_count"`

	// LastCycleAt is when the last complete cycle finished.
	LastCycleAt *time.Time `json:"last_cycle_at,omitempty"`
}

// StepState represents the execution state of a single patrol step.
type StepState struct {
	// Status is the current status: pending, in_progress, completed, skipped.
	Status string `json:"status"`

	// StartedAt is when this step began execution.
	StartedAt *time.Time `json:"started_at,omitempty"`

	// CompletedAt is when this step finished.
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Output is optional output from step execution.
	Output string `json:"output,omitempty"`

	// Error is set if the step failed.
	Error string `json:"error,omitempty"`
}

// NewSlungWork creates a new slung work wisp.
func NewSlungWork(beadID, createdBy string) *SlungWork {
	return &SlungWork{
		Wisp: Wisp{
			Type:      TypeSlungWork,
			CreatedAt: time.Now(),
			CreatedBy: createdBy,
		},
		BeadID: beadID,
	}
}

// NewPatrolCycle creates a new patrol cycle wisp.
func NewPatrolCycle(formula, createdBy string) *PatrolCycle {
	return &PatrolCycle{
		Wisp: Wisp{
			Type:      TypePatrolCycle,
			CreatedAt: time.Now(),
			CreatedBy: createdBy,
		},
		Formula:    formula,
		StepStates: make(map[string]StepState),
	}
}

// HookFilename returns the filename for an agent's hook file.
// Agent IDs containing slashes (e.g., "gastown/crew/joe") are sanitized
// by replacing "/" with "--" to create valid filenames.
func HookFilename(agent string) string {
	// Sanitize agent ID: replace path separators with double-dash
	// This is reversible and avoids creating subdirectories
	sanitized := sanitizeAgentID(agent)
	return HookPrefix + sanitized + HookSuffix
}

// sanitizeAgentID converts an agent ID to a safe filename component.
// "gastown/crew/joe" -> "gastown--crew--joe"
func sanitizeAgentID(agent string) string {
	return strings.ReplaceAll(agent, "/", "--")
}

// unsanitizeAgentID converts a sanitized filename back to an agent ID.
// "gastown--crew--joe" -> "gastown/crew/joe"
func unsanitizeAgentID(sanitized string) string {
	return strings.ReplaceAll(sanitized, "--", "/")
}
