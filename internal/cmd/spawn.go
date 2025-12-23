package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

// Spawn command flags
var (
	spawnIssue    string
	spawnMessage  string
	spawnCreate   bool
	spawnNoStart  bool
	spawnPolecat  string
	spawnRig      string
	spawnMolecule string
	spawnForce    bool
)

var spawnCmd = &cobra.Command{
	Use:     "spawn [rig/polecat | rig]",
	Aliases: []string{"sp"},
	Short:   "Spawn a polecat with work assignment",
	Long: `Spawn a polecat with a work assignment.

Assigns an issue or task to a polecat and starts a session. If no polecat
is specified, auto-selects an idle polecat in the rig.

When --molecule is specified, the molecule is first instantiated on the parent
issue (creating child steps), then the polecat is spawned on the first ready step.

Examples:
  gt spawn gastown/Toast --issue gt-abc
  gt spawn gastown --issue gt-def          # auto-select polecat
  gt spawn gastown/Nux -m "Fix the tests"  # free-form task
  gt spawn gastown/Capable --issue gt-xyz --create  # create if missing

  # Flag-based selection (rig inferred from current directory):
  gt spawn --issue gt-xyz --polecat Angharad
  gt spawn --issue gt-abc --rig gastown --polecat Toast

  # With molecule workflow:
  gt spawn --issue gt-abc --molecule mol-engineer-box`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSpawn,
}

func init() {
	spawnCmd.Flags().StringVar(&spawnIssue, "issue", "", "Beads issue ID to assign")
	spawnCmd.Flags().StringVarP(&spawnMessage, "message", "m", "", "Free-form task description")
	spawnCmd.Flags().BoolVar(&spawnCreate, "create", false, "Create polecat if it doesn't exist")
	spawnCmd.Flags().BoolVar(&spawnNoStart, "no-start", false, "Assign work but don't start session")
	spawnCmd.Flags().StringVar(&spawnPolecat, "polecat", "", "Polecat name (alternative to positional arg)")
	spawnCmd.Flags().StringVar(&spawnRig, "rig", "", "Rig name (defaults to current directory's rig)")
	spawnCmd.Flags().StringVar(&spawnMolecule, "molecule", "", "Molecule ID to instantiate on the issue")
	spawnCmd.Flags().BoolVar(&spawnForce, "force", false, "Force spawn even if polecat has unread mail")

	rootCmd.AddCommand(spawnCmd)
}

// BeadsIssue represents a beads issue from JSON output.
type BeadsIssue struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    int    `json:"priority"`
	Type        string `json:"issue_type"`
	Status      string `json:"status"`
}

func runSpawn(cmd *cobra.Command, args []string) error {
	if spawnIssue == "" && spawnMessage == "" {
		return fmt.Errorf("must specify --issue or -m/--message")
	}

	// --molecule requires --issue
	if spawnMolecule != "" && spawnIssue == "" {
		return fmt.Errorf("--molecule requires --issue to be specified")
	}

	// Find workspace first (needed for rig inference)
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	var rigName, polecatName string

	// Determine rig and polecat from positional arg or flags
	if len(args) > 0 {
		// Parse address: rig/polecat or just rig
		rigName, polecatName, err = parseSpawnAddress(args[0])
		if err != nil {
			return err
		}
	} else {
		// No positional arg - use flags
		polecatName = spawnPolecat
		rigName = spawnRig

		// If no --rig flag, infer from current directory
		if rigName == "" {
			rigName, err = inferRigFromCwd(townRoot)
			if err != nil {
				return fmt.Errorf("cannot determine rig: %w\nUse --rig to specify explicitly or provide rig/polecat as positional arg", err)
			}
		}
	}

	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	r, err := rigMgr.GetRig(rigName)
	if err != nil {
		return fmt.Errorf("rig '%s' not found", rigName)
	}

	// Get polecat manager
	polecatGit := git.NewGit(r.Path)
	polecatMgr := polecat.NewManager(r, polecatGit)

	// Router for mail operations (used for checking inbox and sending assignments)
	router := mail.NewRouter(r.Path)

	// Auto-select polecat if not specified
	if polecatName == "" {
		polecatName, err = selectIdlePolecat(polecatMgr, r)
		if err != nil {
			// If --create is set, allocate a name from the pool
			if spawnCreate {
				polecatName, err = polecatMgr.AllocateName()
				if err != nil {
					return fmt.Errorf("allocating polecat name: %w", err)
				}
				fmt.Printf("Allocated polecat name: %s\n", polecatName)
			} else {
				return fmt.Errorf("auto-select polecat: %w", err)
			}
		} else {
			fmt.Printf("Auto-selected polecat: %s\n", polecatName)
		}
	}

	// Address for this polecat (used for mail operations)
	polecatAddress := fmt.Sprintf("%s/%s", rigName, polecatName)

	// Check if polecat exists
	existingPolecat, err := polecatMgr.Get(polecatName)
	polecatExists := err == nil

	if polecatExists {
		// Polecat exists - we'll recreate it fresh after safety checks

		// Check if polecat is currently working (cannot interrupt active work)
		if existingPolecat.State == polecat.StateWorking {
			return fmt.Errorf("polecat '%s' is already working on %s", polecatName, existingPolecat.Issue)
		}

		// Check for uncommitted work (safety check before recreating)
		pGit := git.NewGit(existingPolecat.ClonePath)
		workStatus, checkErr := pGit.CheckUncommittedWork()
		if checkErr == nil && !workStatus.Clean() {
			fmt.Printf("\n%s Polecat has uncommitted work:\n", style.Warning.Render("⚠"))
			if workStatus.HasUncommittedChanges {
				fmt.Printf("  • %d uncommitted change(s)\n", len(workStatus.ModifiedFiles)+len(workStatus.UntrackedFiles))
			}
			if workStatus.StashCount > 0 {
				fmt.Printf("  • %d stash(es)\n", workStatus.StashCount)
			}
			if workStatus.UnpushedCommits > 0 {
				fmt.Printf("  • %d unpushed commit(s)\n", workStatus.UnpushedCommits)
			}
			fmt.Println()
			if !spawnForce {
				return fmt.Errorf("polecat '%s' has uncommitted work (%s)\nCommit or stash changes before spawning, or use --force to proceed anyway",
					polecatName, workStatus.String())
			}
			fmt.Printf("%s Proceeding with --force (uncommitted work will be lost)\n",
				style.Dim.Render("Warning:"))
		}

		// Check for unread mail (indicates existing unstarted work)
		mailbox, mailErr := router.GetMailbox(polecatAddress)
		if mailErr == nil {
			_, unread, _ := mailbox.Count()
			if unread > 0 && !spawnForce {
				return fmt.Errorf("polecat '%s' has %d unread message(s) in inbox (possible existing work assignment)\nUse --force to override, or let the polecat process its inbox first",
					polecatName, unread)
			} else if unread > 0 {
				fmt.Printf("%s Polecat has %d unread message(s), proceeding with --force\n",
					style.Dim.Render("Warning:"), unread)
			}
		}

		// Recreate the polecat with a fresh worktree (latest code from main)
		fmt.Printf("Recreating polecat %s with fresh worktree...\n", polecatName)
		if _, err = polecatMgr.Recreate(polecatName, spawnForce); err != nil {
			return fmt.Errorf("recreating polecat: %w", err)
		}
		fmt.Printf("%s Fresh worktree created\n", style.Bold.Render("✓"))
	} else if err == polecat.ErrPolecatNotFound {
		// Polecat doesn't exist - create new one
		if !spawnCreate {
			return fmt.Errorf("polecat '%s' not found (use --create to create)", polecatName)
		}
		fmt.Printf("Creating polecat %s...\n", polecatName)
		if _, err = polecatMgr.Add(polecatName); err != nil {
			return fmt.Errorf("creating polecat: %w", err)
		}
	} else {
		return fmt.Errorf("getting polecat: %w", err)
	}

	// Beads operations use rig-level beads (at rig root, not mayor/rig)
	beadsPath := r.Path

	// Sync beads to ensure fresh state before spawn operations
	if err := syncBeads(beadsPath, true); err != nil {
		// Non-fatal - continue with possibly stale beads
		fmt.Printf("%s beads sync: %v\n", style.Dim.Render("Warning:"), err)
	}

	// Track molecule context for work assignment mail
	var moleculeCtx *MoleculeContext

	// Handle molecule instantiation if specified
	if spawnMolecule != "" {
		// Use bd mol run to spawn the molecule - this handles everything:
		// - Creates child issues from proto template
		// - Assigns root to polecat
		// - Sets root status to in_progress
		// - Pins root for session recovery
		fmt.Printf("Running molecule %s on %s...\n", spawnMolecule, spawnIssue)

		cmd := exec.Command("bd", "--no-daemon", "mol", "run", spawnMolecule,
			"--var", "issue="+spawnIssue, "--json")
		cmd.Dir = beadsPath

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			errMsg := strings.TrimSpace(stderr.String())
			if errMsg != "" {
				return fmt.Errorf("running molecule: %s", errMsg)
			}
			return fmt.Errorf("running molecule: %w", err)
		}

		// Parse mol run output
		var molResult struct {
			RootID    string            `json:"root_id"`
			IDMapping map[string]string `json:"id_mapping"`
			Created   int               `json:"created"`
			Assignee  string            `json:"assignee"`
			Pinned    bool              `json:"pinned"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &molResult); err != nil {
			return fmt.Errorf("parsing molecule result: %w", err)
		}

		fmt.Printf("%s Molecule spawned: %s (%d steps)\n",
			style.Bold.Render("✓"), molResult.RootID, molResult.Created-1) // -1 for root

		// Build molecule context for work assignment
		moleculeCtx = &MoleculeContext{
			MoleculeID:  spawnMolecule,
			RootIssueID: molResult.RootID,
			TotalSteps:  molResult.Created - 1, // -1 for root
			StepNumber:  1,                     // Starting on first step
		}

		// Update spawnIssue to be the molecule root (for assignment tracking)
		spawnIssue = molResult.RootID
	}

	// Get or create issue
	var issue *BeadsIssue
	var assignmentID string
	if spawnIssue != "" {
		// Use existing issue
		issue, err = fetchBeadsIssue(beadsPath, spawnIssue)
		if err != nil {
			return fmt.Errorf("fetching issue %s: %w", spawnIssue, err)
		}
		assignmentID = spawnIssue
	} else {
		// Create a beads issue for free-form task
		fmt.Printf("Creating beads issue for task...\n")
		issue, err = createBeadsTask(beadsPath, spawnMessage)
		if err != nil {
			return fmt.Errorf("creating task issue: %w", err)
		}
		assignmentID = issue.ID
		fmt.Printf("Created issue %s\n", assignmentID)
	}

	// Assign issue to polecat (sets issue.assignee in beads)
	if err := polecatMgr.AssignIssue(polecatName, assignmentID); err != nil {
		return fmt.Errorf("assigning issue: %w", err)
	}

	fmt.Printf("%s Assigned %s to %s/%s\n",
		style.Bold.Render("✓"),
		assignmentID, rigName, polecatName)

	// Sync beads to push assignment changes
	if err := syncBeads(beadsPath, false); err != nil {
		// Non-fatal warning
		fmt.Printf("%s beads push: %v\n", style.Dim.Render("Warning:"), err)
	}

	// Stop here if --no-start
	if spawnNoStart {
		fmt.Printf("\n  %s\n", style.Dim.Render("Use 'gt session start' to start the session"))
		return nil
	}

	// Send work assignment mail to polecat inbox (before starting session)
	workMsg := buildWorkAssignmentMail(issue, spawnMessage, polecatAddress, moleculeCtx)

	fmt.Printf("Sending work assignment to %s inbox...\n", polecatAddress)
	if err := router.Send(workMsg); err != nil {
		return fmt.Errorf("sending work assignment: %w", err)
	}
	fmt.Printf("%s Work assignment sent\n", style.Bold.Render("✓"))

	// Start session
	t := tmux.NewTmux()
	sessMgr := session.NewManager(t, r)

	// Check if already running
	running, _ := sessMgr.IsRunning(polecatName)
	if running {
		fmt.Printf("Session already running\n")
	} else {
		// Start new session
		fmt.Printf("Starting session for %s/%s...\n", rigName, polecatName)
		if err := sessMgr.Start(polecatName, session.StartOptions{}); err != nil {
			return fmt.Errorf("starting session: %w", err)
		}
	}

	fmt.Printf("%s Session started. Attach with: %s\n",
		style.Bold.Render("✓"),
		style.Dim.Render(fmt.Sprintf("gt session at %s/%s", rigName, polecatName)))

	// NOTE: We do NOT send a nudge here. Claude Code takes 10-20+ seconds to initialize,
	// and sending keys before the prompt is ready causes them to be mangled.
	// The Deacon will poll with WaitForClaudeReady and send a trigger when ready.
	// The polecat's SessionStart hook runs gt prime, and work assignment is in its inbox.

	// Notify Witness and Deacon about the spawn for monitoring
	// Use town-level beads for cross-agent mail (gt-c6b: mail coordination uses town-level)
	townRouter := mail.NewRouter(townRoot)
	sender := detectSender()
	sessionName := sessMgr.SessionName(polecatName)

	// Notify Witness with POLECAT_STARTED message
	witnessAddr := fmt.Sprintf("%s/witness", rigName)
	witnessNotification := &mail.Message{
		To:      witnessAddr,
		From:    sender,
		Subject: fmt.Sprintf("POLECAT_STARTED %s", polecatName),
		Body:    fmt.Sprintf("Issue: %s\nSession: %s", assignmentID, sessionName),
	}

	if err := townRouter.Send(witnessNotification); err != nil {
		fmt.Printf("  %s\n", style.Dim.Render(fmt.Sprintf("Warning: could not notify witness: %v", err)))
	} else {
		fmt.Printf("  %s\n", style.Dim.Render("Witness notified of polecat start"))
	}

	// Notify Deacon with POLECAT_STARTED message (includes full rig/polecat address)
	deaconAddr := "deacon/"
	deaconNotification := &mail.Message{
		To:      deaconAddr,
		From:    sender,
		Subject: fmt.Sprintf("POLECAT_STARTED %s/%s", rigName, polecatName),
		Body:    fmt.Sprintf("Issue: %s\nSession: %s", assignmentID, sessionName),
	}

	if err := townRouter.Send(deaconNotification); err != nil {
		fmt.Printf("  %s\n", style.Dim.Render(fmt.Sprintf("Warning: could not notify deacon: %v", err)))
	} else {
		fmt.Printf("  %s\n", style.Dim.Render("Deacon notified of polecat start"))
	}

	return nil
}

// parseSpawnAddress parses "rig/polecat" or "rig".
func parseSpawnAddress(addr string) (rigName, polecatName string, err error) {
	if strings.Contains(addr, "/") {
		parts := strings.SplitN(addr, "/", 2)
		if parts[0] == "" {
			return "", "", fmt.Errorf("invalid address: missing rig name")
		}
		return parts[0], parts[1], nil
	}
	return addr, "", nil
}


// selectIdlePolecat finds an idle polecat in the rig.
func selectIdlePolecat(mgr *polecat.Manager, r *rig.Rig) (string, error) {
	polecats, err := mgr.List()
	if err != nil {
		return "", err
	}

	// Prefer idle polecats
	for _, pc := range polecats {
		if pc.State == polecat.StateIdle {
			return pc.Name, nil
		}
	}

	// Accept active polecats without current work
	for _, pc := range polecats {
		if pc.State == polecat.StateActive && pc.Issue == "" {
			return pc.Name, nil
		}
	}

	// Check rig's polecat list for any we haven't loaded yet
	for _, name := range r.Polecats {
		found := false
		for _, pc := range polecats {
			if pc.Name == name {
				found = true
				break
			}
		}
		if !found {
			return name, nil
		}
	}

	return "", fmt.Errorf("no available polecats in rig '%s'", r.Name)
}

// fetchBeadsIssue gets issue details from beads CLI.
func fetchBeadsIssue(rigPath, issueID string) (*BeadsIssue, error) {
	cmd := exec.Command("bd", "show", issueID, "--json")
	cmd.Dir = rigPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("%s", errMsg)
		}
		return nil, err
	}

	// bd show --json returns an array, take the first element
	var issues []BeadsIssue
	if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
		return nil, fmt.Errorf("parsing issue: %w", err)
	}
	if len(issues) == 0 {
		return nil, fmt.Errorf("issue not found: %s", issueID)
	}

	return &issues[0], nil
}

// createBeadsTask creates a new beads task issue for a free-form task message.
func createBeadsTask(rigPath, message string) (*BeadsIssue, error) {
	// Truncate message for title if too long
	title := message
	if len(title) > 60 {
		title = title[:57] + "..."
	}

	// Use bd create to make a new task issue
	cmd := exec.Command("bd", "create",
		"--title="+title,
		"--type=task",
		"--priority=2",
		"--description="+message,
		"--json")
	cmd.Dir = rigPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("%s", errMsg)
		}
		return nil, err
	}

	// bd create --json returns the created issue
	var issue BeadsIssue
	if err := json.Unmarshal(stdout.Bytes(), &issue); err != nil {
		return nil, fmt.Errorf("parsing created issue: %w", err)
	}

	return &issue, nil
}

// syncBeads runs bd sync in the given directory.
// This ensures beads state is fresh before spawn operations.
func syncBeads(workDir string, fromMain bool) error {
	args := []string{"sync"}
	if fromMain {
		args = append(args, "--from-main")
	}
	cmd := exec.Command("bd", args...)
	cmd.Dir = workDir
	return cmd.Run()
}

// buildSpawnContext creates the initial context message for the polecat.
// Deprecated: Use buildWorkAssignmentMail instead for mail-based work assignment.
func buildSpawnContext(issue *BeadsIssue, message string) string {
	var sb strings.Builder

	sb.WriteString("[SPAWN] You have been assigned work.\n\n")

	if issue != nil {
		sb.WriteString(fmt.Sprintf("Issue: %s\n", issue.ID))
		sb.WriteString(fmt.Sprintf("Title: %s\n", issue.Title))
		sb.WriteString(fmt.Sprintf("Priority: P%d\n", issue.Priority))
		sb.WriteString(fmt.Sprintf("Type: %s\n", issue.Type))
		if issue.Description != "" {
			sb.WriteString(fmt.Sprintf("\nDescription:\n%s\n", issue.Description))
		}
	} else if message != "" {
		sb.WriteString(fmt.Sprintf("Task: %s\n", message))
	}

	sb.WriteString("\n## Workflow\n")
	sb.WriteString("1. Run `gt prime` to load polecat context\n")
	sb.WriteString("2. Work on your task, commit changes regularly\n")
	sb.WriteString("3. Run `bd close <issue-id>` when done\n")
	sb.WriteString("4. Run `bd sync` to push beads changes\n")
	sb.WriteString("5. Push code: `git push origin HEAD`\n")
	sb.WriteString("6. Run `gt done` to signal completion\n")

	return sb.String()
}

// MoleculeContext contains information about a molecule workflow assignment.
type MoleculeContext struct {
	MoleculeID  string // The molecule template ID (proto)
	RootIssueID string // The spawned molecule root issue
	TotalSteps  int    // Total number of steps in the molecule
	StepNumber  int    // Which step this is (1-indexed)
	IsWisp      bool   // True if this is a wisp (not durable mol)
}

// buildWorkAssignmentMail creates a work assignment mail message for a polecat.
// This replaces tmux-based context injection with persistent mailbox delivery.
// If moleculeCtx is non-nil, includes molecule workflow instructions.
func buildWorkAssignmentMail(issue *BeadsIssue, message, polecatAddress string, moleculeCtx *MoleculeContext) *mail.Message {
	var subject string
	var body strings.Builder

	if issue != nil {
		if moleculeCtx != nil {
			subject = fmt.Sprintf("🧬 Molecule: %s (step %d/%d)", issue.Title, moleculeCtx.StepNumber, moleculeCtx.TotalSteps)
		} else {
			subject = fmt.Sprintf("📋 Work Assignment: %s", issue.Title)
		}

		body.WriteString(fmt.Sprintf("Issue: %s\n", issue.ID))
		body.WriteString(fmt.Sprintf("Title: %s\n", issue.Title))
		body.WriteString(fmt.Sprintf("Priority: P%d\n", issue.Priority))
		body.WriteString(fmt.Sprintf("Type: %s\n", issue.Type))
		if issue.Description != "" {
			body.WriteString(fmt.Sprintf("\nDescription:\n%s\n", issue.Description))
		}
	} else if message != "" {
		// Truncate for subject if too long
		titleText := message
		if len(titleText) > 50 {
			titleText = titleText[:47] + "..."
		}
		subject = fmt.Sprintf("📋 Work Assignment: %s", titleText)
		body.WriteString(fmt.Sprintf("Task: %s\n", message))
	}

	// Add molecule context if present
	if moleculeCtx != nil {
		body.WriteString("\n## Molecule Workflow\n")
		body.WriteString(fmt.Sprintf("You are working on step %d of %d in molecule %s.\n", moleculeCtx.StepNumber, moleculeCtx.TotalSteps, moleculeCtx.MoleculeID))
		body.WriteString(fmt.Sprintf("Molecule root: %s\n\n", moleculeCtx.RootIssueID))
		body.WriteString("After completing this step:\n")
		body.WriteString("1. Run `bd close <step-id>`\n")
		body.WriteString("2. Run `bd ready --parent " + moleculeCtx.RootIssueID + "` to find next ready steps\n")
		body.WriteString("3. If more steps are ready, continue working on them\n")
		body.WriteString("4. When all steps are done, run `gt done` to signal completion\n\n")
	}

	body.WriteString("\n## Workflow\n")
	body.WriteString("1. Run `gt prime` to load polecat context\n")
	body.WriteString("2. Work on your task, commit changes regularly\n")
	body.WriteString("3. Run `bd close <issue-id>` when done\n")
	if moleculeCtx != nil {
		body.WriteString("4. Check `bd ready --parent " + moleculeCtx.RootIssueID + "` for more steps\n")
		body.WriteString("5. Repeat steps 2-4 for each ready step\n")
		body.WriteString("6. When all steps done: run `bd sync`, push code, run `gt done`\n")
	} else {
		body.WriteString("4. Run `bd sync` to push beads changes\n")
		body.WriteString("5. Push code: `git push origin HEAD`\n")
		body.WriteString("6. Run `gt done` to signal completion\n")
	}
	body.WriteString("\n## Handoff Protocol\n")
	body.WriteString("Before signaling done, ensure:\n")
	body.WriteString("- Git status is clean (no uncommitted changes)\n")
	body.WriteString("- Issue is closed with `bd close`\n")
	body.WriteString("- Beads are synced with `bd sync`\n")
	body.WriteString("- Code is pushed to origin\n")
	body.WriteString("\nThe `gt done` command verifies these and signals the Witness.\n")

	return &mail.Message{
		From:     "mayor/",
		To:       polecatAddress,
		Subject:  subject,
		Body:     body.String(),
		Priority: mail.PriorityHigh,
		Type:     mail.TypeTask,
	}
}

