package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/wisp"
)

var slingCmd = &cobra.Command{
	Use:   "sling <bead-id>",
	Short: "Attach work to hook and restart agent",
	Long: `Sling work onto the agent's hook and restart with that context.

This is the "restart-and-resume" mechanism - attach a bead (issue) to your hook,
then restart with a fresh context. The new session wakes up, finds the slung work
on its hook, and begins working on it immediately.

The wisp is ephemeral (stored in .beads-wisp/, not git-tracked). It's burned
after the agent picks it up.

Examples:
  gt sling gt-abc                       # Attach issue and restart
  gt sling gt-abc -s "Fix the bug"      # With handoff subject
  gt sling gt-abc -m "Check tests too"  # With handoff message

The propulsion principle: if you find something on your hook, YOU RUN IT.`,
	Args: cobra.ExactArgs(1),
	RunE: runSling,
}

var (
	slingSubject string
	slingMessage string
	slingDryRun  bool
)

func init() {
	slingCmd.Flags().StringVarP(&slingSubject, "subject", "s", "", "Subject for handoff mail")
	slingCmd.Flags().StringVarP(&slingMessage, "message", "m", "", "Message for handoff mail")
	slingCmd.Flags().BoolVarP(&slingDryRun, "dry-run", "n", false, "Show what would be done")
	rootCmd.AddCommand(slingCmd)
}

func runSling(cmd *cobra.Command, args []string) error {
	beadID := args[0]

	// Polecats cannot sling - check early before writing anything
	if polecatName := os.Getenv("GT_POLECAT"); polecatName != "" {
		return fmt.Errorf("polecats cannot sling (use gt done for handoff)")
	}

	// Verify the bead exists
	if err := verifyBeadExists(beadID); err != nil {
		return err
	}

	// Determine agent identity
	agentID, err := detectAgentIdentity()
	if err != nil {
		return fmt.Errorf("detecting agent identity: %w", err)
	}

	// Get cwd for wisp storage (use clone root, not town root)
	cloneRoot, err := detectCloneRoot()
	if err != nil {
		return fmt.Errorf("detecting clone root: %w", err)
	}

	// Create the slung work wisp
	sw := wisp.NewSlungWork(beadID, agentID)
	sw.Subject = slingSubject
	sw.Context = slingMessage

	fmt.Printf("%s Slinging %s onto hook...\n", style.Bold.Render("🎯"), beadID)

	if slingDryRun {
		fmt.Printf("Would create wisp: %s\n", wisp.HookPath(cloneRoot, agentID))
		fmt.Printf("  bead_id: %s\n", beadID)
		fmt.Printf("  agent: %s\n", agentID)
		if slingSubject != "" {
			fmt.Printf("  subject: %s\n", slingSubject)
		}
		if slingMessage != "" {
			fmt.Printf("  context: %s\n", slingMessage)
		}
		fmt.Println("Would trigger handoff...")
		return nil
	}

	// Write the wisp to the hook
	if err := wisp.WriteSlungWork(cloneRoot, agentID, sw); err != nil {
		return fmt.Errorf("writing wisp: %w", err)
	}

	fmt.Printf("%s Work attached to hook\n", style.Bold.Render("✓"))

	// Now trigger handoff (reuse existing handoff logic)
	return triggerHandoff(agentID, beadID)
}

// verifyBeadExists checks that the bead exists using bd show.
func verifyBeadExists(beadID string) error {
	cmd := exec.Command("bd", "show", beadID, "--json")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bead '%s' not found (bd show failed)", beadID)
	}
	return nil
}

// detectAgentIdentity figures out who we are (crew/joe, witness, etc).
func detectAgentIdentity() (string, error) {
	// Check environment first
	if crew := os.Getenv("GT_CREW"); crew != "" {
		if rig := os.Getenv("GT_RIG"); rig != "" {
			return fmt.Sprintf("%s/crew/%s", rig, crew), nil
		}
	}

	// Check if we're a polecat
	if polecat := os.Getenv("GT_POLECAT"); polecat != "" {
		if rig := os.Getenv("GT_RIG"); rig != "" {
			return fmt.Sprintf("%s/polecats/%s", rig, polecat), nil
		}
	}

	// Try to detect from cwd
	detected, err := detectCrewFromCwd()
	if err == nil {
		return fmt.Sprintf("%s/crew/%s", detected.rigName, detected.crewName), nil
	}

	// Check for other role markers in session name
	if session := os.Getenv("TMUX"); session != "" {
		sessionName, err := getCurrentTmuxSession()
		if err == nil {
			if sessionName == "gt-mayor" {
				return "mayor", nil
			}
			if sessionName == "gt-deacon" {
				return "deacon", nil
			}
			if strings.HasSuffix(sessionName, "-witness") {
				rig := strings.TrimSuffix(strings.TrimPrefix(sessionName, "gt-"), "-witness")
				return fmt.Sprintf("%s/witness", rig), nil
			}
			if strings.HasSuffix(sessionName, "-refinery") {
				rig := strings.TrimSuffix(strings.TrimPrefix(sessionName, "gt-"), "-refinery")
				return fmt.Sprintf("%s/refinery", rig), nil
			}
		}
	}

	return "", fmt.Errorf("cannot determine agent identity - set GT_RIG/GT_CREW or run from clone directory")
}

// detectCloneRoot finds the root of the current git clone.
func detectCloneRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not in a git repository")
	}
	return strings.TrimSpace(string(out)), nil
}

// triggerHandoff restarts the agent session.
func triggerHandoff(agentID, beadID string) error {
	// Must be in tmux
	if !tmux.IsInsideTmux() {
		return fmt.Errorf("not running in tmux - cannot restart")
	}

	pane := os.Getenv("TMUX_PANE")
	if pane == "" {
		return fmt.Errorf("TMUX_PANE not set")
	}

	// Get current session
	currentSession, err := getCurrentTmuxSession()
	if err != nil {
		return fmt.Errorf("getting session: %w", err)
	}

	// Build restart command
	restartCmd, err := buildRestartCommand(currentSession)
	if err != nil {
		return err
	}

	// Send handoff mail with the bead reference
	subject := slingSubject
	if subject == "" {
		subject = fmt.Sprintf("🎯 SLUNG: %s", beadID)
	} else {
		subject = fmt.Sprintf("🎯 SLUNG: %s", subject)
	}

	message := slingMessage
	if message == "" {
		message = fmt.Sprintf("Work slung onto hook. Run bd show %s for details.", beadID)
	}

	if err := sendHandoffMail(subject, message); err != nil {
		fmt.Printf("%s Warning: could not send handoff mail: %v\n", style.Dim.Render("⚠"), err)
	} else {
		fmt.Printf("%s Sent handoff mail\n", style.Bold.Render("📬"))
	}

	fmt.Printf("%s Restarting with slung work...\n", style.Bold.Render("🔄"))

	// Respawn the pane
	t := tmux.NewTmux()
	return t.RespawnPane(pane, restartCmd)
}
