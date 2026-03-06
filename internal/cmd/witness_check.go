package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/deacon"
	"github.com/steveyegge/gastown/internal/refinery"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/workspace"
)

// WitnessCheckOutput is the machine-readable payload for `gt witness check --json`.
type WitnessCheckOutput struct {
	CheckedAt string                  `json:"checked_at"`
	Rig       string                  `json:"rig"`
	Prefix    string                  `json:"prefix"`
	Refinery  WitnessCheckRefinery    `json:"refinery"`
	Deacon    WitnessCheckDeacon      `json:"deacon"`
	Queue     WitnessCheckQueueHealth `json:"queue"`
}

type WitnessCheckRefinery struct {
	Session string `json:"session"`
	Running bool   `json:"running"`
}

type WitnessCheckDeacon struct {
	Session                         string                 `json:"session"`
	Running                         bool                   `json:"running"`
	GraceWindowSeconds              int64                  `json:"grace_window_seconds"`
	RecommendedRecheckSeconds       int64                  `json:"recommended_recheck_seconds"`
	WithinGraceWindow               bool                   `json:"within_grace_window"`
	HeartbeatStaleThresholdSeconds  int64                  `json:"heartbeat_stale_threshold_seconds"`
	HeartbeatVeryStaleThresholdSecs int64                  `json:"heartbeat_very_stale_threshold_seconds"`
	Heartbeat                       *WitnessCheckHeartbeat `json:"heartbeat,omitempty"`
}

type WitnessCheckHeartbeat struct {
	Timestamp  string `json:"timestamp"`
	AgeSeconds int64  `json:"age_seconds"`
	Stale      bool   `json:"stale"`
	VeryStale  bool   `json:"very_stale"`
	Cycle      int64  `json:"cycle"`
	LastAction string `json:"last_action,omitempty"`
}

type WitnessCheckQueueHealth struct {
	Summary   WitnessCheckQueueSummary `json:"summary"`
	Open      []*refinery.MRInfo       `json:"open"`
	Ready     []*refinery.MRInfo       `json:"ready"`
	Blocked   []*refinery.MRInfo       `json:"blocked"`
	Anomalies []*refinery.MRAnomaly    `json:"anomalies,omitempty"`
}

type WitnessCheckQueueSummary struct {
	OpenTotal           int `json:"open_total"`
	ReadyCount          int `json:"ready_count"`
	BlockedCount        int `json:"blocked_count"`
	ClaimedCount        int `json:"claimed_count"`
	UnclaimedCount      int `json:"unclaimed_count"`
	AnomalyCount        int `json:"anomaly_count"`
	OrphanedBranchCount int `json:"orphaned_branch_count"`
}

func runWitnessCheck(cmd *cobra.Command, args []string) error {
	rigArg := ""
	if len(args) > 0 {
		rigArg = args[0]
	}

	rigName, err := resolveWitnessCheckRigName(rigArg)
	if err != nil {
		return err
	}

	_, r, err := getRig(rigName)
	if err != nil {
		return err
	}

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	prefix := session.PrefixFor(rigName)

	refMgr := refinery.NewManager(r)
	refineryRunning, _ := refMgr.IsRunning()

	operational := config.LoadOperationalConfig(townRoot)
	daemonCfg := operational.GetDaemonConfig()
	deaconCfg := operational.GetDeaconConfig()
	deaconGraceWindow := daemonCfg.DeaconGracePeriodD()
	deaconStaleThreshold := deaconCfg.HeartbeatStaleThresholdD()
	deaconVeryStaleThreshold := deaconCfg.HeartbeatVeryStaleThresholdD()

	deaconMgr := deacon.NewManager(townRoot)
	deaconRunning, _ := deaconMgr.IsRunning()
	deaconHeartbeat := deacon.ReadHeartbeat(townRoot)

	deaconOut := WitnessCheckDeacon{
		Session:                         session.DeaconSessionName(),
		Running:                         deaconRunning,
		GraceWindowSeconds:              int64(deaconGraceWindow.Seconds()),
		RecommendedRecheckSeconds:       30,
		HeartbeatStaleThresholdSeconds:  int64(deaconStaleThreshold.Seconds()),
		HeartbeatVeryStaleThresholdSecs: int64(deaconVeryStaleThreshold.Seconds()),
	}
	if deaconHeartbeat != nil {
		age := deaconHeartbeat.Age()
		deaconOut.Heartbeat = &WitnessCheckHeartbeat{
			Timestamp:  deaconHeartbeat.Timestamp.UTC().Format(time.RFC3339),
			AgeSeconds: int64(age.Seconds()),
			Stale:      deaconHeartbeat.IsStale(),
			VeryStale:  deaconHeartbeat.IsVeryStale(),
			Cycle:      deaconHeartbeat.Cycle,
			LastAction: deaconHeartbeat.LastAction,
		}
		// Best-effort grace signal for patrol logic when deacon appears temporarily down.
		if !deaconRunning {
			deaconOut.WithinGraceWindow = age < deaconGraceWindow
		}
	}

	eng := refinery.NewEngineer(r)
	openMRs, err := eng.ListAllOpenMRs()
	if err != nil {
		return fmt.Errorf("listing all open refinery MRs: %w", err)
	}
	readyMRs, err := eng.ListReadyMRs()
	if err != nil {
		return fmt.Errorf("listing ready refinery MRs: %w", err)
	}
	blockedMRs, err := eng.ListBlockedMRs()
	if err != nil {
		return fmt.Errorf("listing blocked refinery MRs: %w", err)
	}
	anomalies, err := eng.ListQueueAnomalies(time.Now())
	if err != nil {
		return fmt.Errorf("listing refinery queue anomalies: %w", err)
	}

	out := WitnessCheckOutput{
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
		Rig:       rigName,
		Prefix:    prefix,
		Refinery: WitnessCheckRefinery{
			Session: session.RefinerySessionName(prefix),
			Running: refineryRunning,
		},
		Deacon: deaconOut,
		Queue: WitnessCheckQueueHealth{
			Summary:   buildWitnessQueueSummary(openMRs, readyMRs, blockedMRs, anomalies),
			Open:      openMRs,
			Ready:     readyMRs,
			Blocked:   blockedMRs,
			Anomalies: anomalies,
		},
	}

	if witnessCheckJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	fmt.Printf("Witness dependency check for %s (%s)\n", out.Rig, out.Prefix)
	fmt.Printf("  Refinery: %v (%s)\n", out.Refinery.Running, out.Refinery.Session)
	fmt.Printf("  Deacon:   %v (%s)\n", out.Deacon.Running, out.Deacon.Session)
	fmt.Printf("  Queue:    open=%d ready=%d blocked=%d anomalies=%d\n",
		out.Queue.Summary.OpenTotal,
		out.Queue.Summary.ReadyCount,
		out.Queue.Summary.BlockedCount,
		out.Queue.Summary.AnomalyCount,
	)
	return nil
}

func resolveWitnessCheckRigName(rigArg string) (string, error) {
	rigArg = strings.TrimSpace(rigArg)
	if rigArg == "" {
		// Reuse refinery inference logic for consistency with other agent commands.
		_, _, rigName, err := getRefineryManager("")
		if err != nil {
			return "", err
		}
		return rigName, nil
	}

	rigName, _, err := resolveRigNameOrPrefix(rigArg, session.DefaultRegistry(), func(candidate string) bool {
		_, _, getRigErr := getRig(candidate)
		return getRigErr == nil
	})
	if err != nil {
		return "", err
	}
	return rigName, nil
}

func resolveRigNameOrPrefix(input string, registry *session.PrefixRegistry, rigExists func(string) bool) (rigName, prefix string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", fmt.Errorf("empty rig or prefix")
	}
	if rigExists(input) {
		return input, prefixForRig(input, registry), nil
	}
	if registry != nil {
		mapped := registry.RigForPrefix(input)
		if mapped != input && rigExists(mapped) {
			return mapped, input, nil
		}
	}
	return "", "", fmt.Errorf("unknown rig or prefix %q", input)
}

func prefixForRig(rigName string, registry *session.PrefixRegistry) string {
	if registry != nil {
		return registry.PrefixForRig(rigName)
	}
	return session.PrefixFor(rigName)
}

func buildWitnessQueueSummary(open, ready, blocked []*refinery.MRInfo, anomalies []*refinery.MRAnomaly) WitnessCheckQueueSummary {
	var claimed, unclaimed, orphaned int
	for _, mr := range open {
		if mr.Assignee == "" {
			unclaimed++
		} else {
			claimed++
		}
		if !mr.BranchExistsLocal && !mr.BranchExistsRemote {
			orphaned++
		}
	}
	return WitnessCheckQueueSummary{
		OpenTotal:           len(open),
		ReadyCount:          len(ready),
		BlockedCount:        len(blocked),
		ClaimedCount:        claimed,
		UnclaimedCount:      unclaimed,
		AnomalyCount:        len(anomalies),
		OrphanedBranchCount: orphaned,
	}
}
