package wisp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Common errors.
var (
	ErrNoWispDir   = errors.New("wisp directory does not exist")
	ErrNoHook      = errors.New("no hook file found")
	ErrInvalidWisp = errors.New("invalid wisp format")
)

// EnsureDir ensures the .beads-wisp directory exists in the given root.
func EnsureDir(root string) (string, error) {
	dir := filepath.Join(root, WispDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create wisp dir: %w", err)
	}
	return dir, nil
}

// WispPath returns the full path to a wisp file.
func WispPath(root, filename string) string {
	return filepath.Join(root, WispDir, filename)
}

// HookPath returns the full path to an agent's hook file.
func HookPath(root, agent string) string {
	return WispPath(root, HookFilename(agent))
}

// WriteSlungWork writes a slung work wisp to the agent's hook.
func WriteSlungWork(root, agent string, sw *SlungWork) error {
	dir, err := EnsureDir(root)
	if err != nil {
		return err
	}

	path := filepath.Join(dir, HookFilename(agent))
	return writeJSON(path, sw)
}

// WritePatrolCycle writes a patrol cycle wisp.
func WritePatrolCycle(root, id string, pc *PatrolCycle) error {
	dir, err := EnsureDir(root)
	if err != nil {
		return err
	}

	filename := "patrol-" + id + ".json"
	path := filepath.Join(dir, filename)
	return writeJSON(path, pc)
}

// ReadHook reads the slung work from an agent's hook file.
// Returns ErrNoHook if no hook file exists.
func ReadHook(root, agent string) (*SlungWork, error) {
	path := HookPath(root, agent)

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, ErrNoHook
	}
	if err != nil {
		return nil, fmt.Errorf("read hook: %w", err)
	}

	var sw SlungWork
	if err := json.Unmarshal(data, &sw); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidWisp, err)
	}

	if sw.Type != TypeSlungWork {
		return nil, fmt.Errorf("%w: expected slung-work, got %s", ErrInvalidWisp, sw.Type)
	}

	return &sw, nil
}

// ReadPatrolCycle reads a patrol cycle wisp.
func ReadPatrolCycle(root, id string) (*PatrolCycle, error) {
	filename := "patrol-" + id + ".json"
	path := WispPath(root, filename)

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, ErrNoHook // reuse error for "not found"
	}
	if err != nil {
		return nil, fmt.Errorf("read patrol cycle: %w", err)
	}

	var pc PatrolCycle
	if err := json.Unmarshal(data, &pc); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidWisp, err)
	}

	if pc.Type != TypePatrolCycle {
		return nil, fmt.Errorf("%w: expected patrol-cycle, got %s", ErrInvalidWisp, pc.Type)
	}

	return &pc, nil
}

// BurnHook removes an agent's hook file after it has been picked up.
func BurnHook(root, agent string) error {
	path := HookPath(root, agent)
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil // already burned
	}
	return err
}

// BurnPatrolCycle removes a patrol cycle wisp.
func BurnPatrolCycle(root, id string) error {
	filename := "patrol-" + id + ".json"
	path := WispPath(root, filename)
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil // already burned
	}
	return err
}

// HasHook checks if an agent has a hook file.
func HasHook(root, agent string) bool {
	path := HookPath(root, agent)
	_, err := os.Stat(path)
	return err == nil
}

// ListHooks returns a list of agents with active hooks.
// Agent IDs are returned in their original form (with slashes).
func ListHooks(root string) ([]string, error) {
	dir := filepath.Join(root, WispDir)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var agents []string
	for _, e := range entries {
		name := e.Name()
		if len(name) > len(HookPrefix)+len(HookSuffix) &&
			name[:len(HookPrefix)] == HookPrefix &&
			name[len(name)-len(HookSuffix):] == HookSuffix {
			sanitized := name[len(HookPrefix) : len(name)-len(HookSuffix)]
			agent := unsanitizeAgentID(sanitized)
			agents = append(agents, agent)
		}
	}
	return agents, nil
}

// writeJSON is a helper to write JSON files atomically.
func writeJSON(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}

	// Write to temp file then rename for atomicity
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write temp: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp) // cleanup on failure
		return fmt.Errorf("rename: %w", err)
	}

	return nil
}
