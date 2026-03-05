package deacon

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	// PatrolHeartbeatCreditsPerCycle is the number of heartbeats a Deacon patrol
	// cycle is allowed to emit before proving progress via patrol lifecycle
	// commands again.
	PatrolHeartbeatCreditsPerCycle = 2

	// bootstrapPatrolHeartbeatCredits allows first-run startup to emit one
	// heartbeat before the first patrol wisp is created.
	bootstrapPatrolHeartbeatCredits = 1
)

// ErrPatrolHeartbeatBudgetExhausted is returned when a heartbeat is attempted
// without remaining patrol heartbeat credits.
var ErrPatrolHeartbeatBudgetExhausted = errors.New("patrol heartbeat budget exhausted")

// PatrolHeartbeatBudget tracks how many deacon heartbeats may be emitted before
// patrol lifecycle progress must be recorded again.
type PatrolHeartbeatBudget struct {
	Remaining      int       `json:"remaining"`
	LastGrantAt    time.Time `json:"last_grant_at,omitempty"`
	LastGrantBy    string    `json:"last_grant_by,omitempty"`
	LastConsumedAt time.Time `json:"last_consumed_at,omitempty"`
	LastUpdated    time.Time `json:"last_updated,omitempty"`
}

// PatrolHeartbeatBudgetFile returns the path to the deacon heartbeat budget file.
func PatrolHeartbeatBudgetFile(townRoot string) string {
	return filepath.Join(townRoot, "deacon", "patrol-heartbeat-budget.json")
}

// LoadPatrolHeartbeatBudget loads heartbeat budget state.
// Returns zero-value state when no file exists.
func LoadPatrolHeartbeatBudget(townRoot string) (*PatrolHeartbeatBudget, error) {
	path := PatrolHeartbeatBudgetFile(townRoot)
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is derived from trusted townRoot
	if err != nil {
		if os.IsNotExist(err) {
			return &PatrolHeartbeatBudget{}, nil
		}
		return nil, fmt.Errorf("reading patrol heartbeat budget: %w", err)
	}

	var state PatrolHeartbeatBudget
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing patrol heartbeat budget: %w", err)
	}
	if state.Remaining < 0 {
		state.Remaining = 0
	}
	return &state, nil
}

// SavePatrolHeartbeatBudget persists heartbeat budget state.
func SavePatrolHeartbeatBudget(townRoot string, state *PatrolHeartbeatBudget) error {
	path := PatrolHeartbeatBudgetFile(townRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating deacon directory: %w", err)
	}
	state.LastUpdated = time.Now().UTC()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling patrol heartbeat budget: %w", err)
	}

	return os.WriteFile(path, data, 0600)
}

// GrantPatrolHeartbeatCredits resets the remaining heartbeat budget to a fixed
// number of credits when a new/next patrol cycle is created.
func GrantPatrolHeartbeatCredits(townRoot string, credits int, grantedBy string) error {
	if credits < 0 {
		credits = 0
	}

	state, err := LoadPatrolHeartbeatBudget(townRoot)
	if err != nil {
		return err
	}
	state.Remaining = credits
	state.LastGrantBy = grantedBy
	state.LastGrantAt = time.Now().UTC()

	return SavePatrolHeartbeatBudget(townRoot, state)
}

// ConsumePatrolHeartbeatCredit decrements heartbeat budget by one.
// On first run (no existing state), this seeds a one-time bootstrap credit so
// deacon startup can emit its initial heartbeat before the first patrol exists.
func ConsumePatrolHeartbeatCredit(townRoot string) (int, error) {
	state, err := LoadPatrolHeartbeatBudget(townRoot)
	if err != nil {
		return 0, err
	}

	if state.Remaining == 0 && state.LastUpdated.IsZero() && state.LastGrantAt.IsZero() {
		state.Remaining = bootstrapPatrolHeartbeatCredits
		state.LastGrantBy = "bootstrap"
		state.LastGrantAt = time.Now().UTC()
	}

	if state.Remaining <= 0 {
		return 0, ErrPatrolHeartbeatBudgetExhausted
	}

	state.Remaining--
	state.LastConsumedAt = time.Now().UTC()
	if err := SavePatrolHeartbeatBudget(townRoot, state); err != nil {
		return 0, err
	}

	return state.Remaining, nil
}
