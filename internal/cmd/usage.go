package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	usageLast     string
	usageByAgent  bool
	usageSnapshot bool
	usageTrend    bool
	usageJSON     bool
)

var usageCmd = &cobra.Command{
	Use:     "usage",
	GroupID: GroupDiag,
	Short:   "Show token usage from Claude transcripts and /usage snapshots",
	Long: `Analyze Claude Code usage for Gas Town agents.

Modes:
  gt usage --last 1h --by-agent   # Transcript token totals by agent identity
  gt usage --snapshot              # Capture /usage percentages into local snapshot log
  gt usage --trend                 # Show %/hour trend from saved snapshots`,
	RunE: runUsage,
}

func init() {
	usageCmd.Flags().StringVar(&usageLast, "last", "", "Only include transcript records since duration/timestamp (e.g. 1h, 90m, 2026-03-06T14:00:00Z)")
	usageCmd.Flags().BoolVar(&usageByAgent, "by-agent", false, "Show transcript usage grouped by canonical agent identity")
	usageCmd.Flags().BoolVar(&usageSnapshot, "snapshot", false, "Capture /usage percentages in a temporary Claude session and append to snapshot log")
	usageCmd.Flags().BoolVar(&usageTrend, "trend", false, "Show percent-per-hour trend from snapshot log")
	usageCmd.Flags().BoolVar(&usageJSON, "json", false, "Output as JSON")
	rootCmd.AddCommand(usageCmd)
}

type usageTokenTotals struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
}

func (u *usageTokenTotals) add(v *TranscriptUsage) {
	if u == nil || v == nil {
		return
	}
	u.InputTokens += int64(v.InputTokens)
	u.OutputTokens += int64(v.OutputTokens)
	u.CacheReadInputTokens += int64(v.CacheReadInputTokens)
	u.CacheCreationInputTokens += int64(v.CacheCreationInputTokens)
}

func (u usageTokenTotals) totalTokens() int64 {
	return u.InputTokens + u.OutputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
}

type usageIdentity struct {
	Agent  string `json:"agent"`
	Role   string `json:"role"`
	Rig    string `json:"rig,omitempty"`
	Worker string `json:"worker,omitempty"`
}

type usageAgentUsage struct {
	Identity          usageIdentity    `json:"identity"`
	Totals            usageTokenTotals `json:"totals"`
	AssistantMessages int              `json:"assistant_messages"`
	FirstSeen         time.Time        `json:"first_seen,omitempty"`
	LastSeen          time.Time        `json:"last_seen,omitempty"`
}

type usageAggregate struct {
	Agents            map[string]*usageAgentUsage
	Total             usageTokenTotals
	AssistantMessages int
}

type usageIdentityLookup struct {
	identity usageIdentity
	ok       bool
}

type usageTranscriptRecord struct {
	Type      string                 `json:"type"`
	CWD       string                 `json:"cwd"`
	Timestamp string                 `json:"timestamp"`
	Message   *TranscriptMessageBody `json:"message,omitempty"`
}

type usageSnapshotEntry struct {
	Timestamp         time.Time `json:"timestamp"`
	CurrentSessionPct float64   `json:"current_session_pct"`
	CurrentWeekPct    float64   `json:"current_week_pct"`
	SonnetOnlyPct     float64   `json:"sonnet_only_pct"`
}

type usageTrendPoint struct {
	From            time.Time `json:"from"`
	To              time.Time `json:"to"`
	Hours           float64   `json:"hours"`
	SessionDeltaPct float64   `json:"session_delta_pct"`
	WeekDeltaPct    float64   `json:"week_delta_pct"`
	SonnetDeltaPct  float64   `json:"sonnet_delta_pct"`
	SessionPctPerH  float64   `json:"session_pct_per_hour"`
	WeekPctPerH     float64   `json:"week_pct_per_hour"`
	SonnetPctPerH   float64   `json:"sonnet_pct_per_hour"`
}

func runUsage(cmd *cobra.Command, args []string) error {
	selectedModes := 0
	if usageSnapshot {
		selectedModes++
	}
	if usageTrend {
		selectedModes++
	}
	if selectedModes > 1 {
		return fmt.Errorf("use only one of --snapshot or --trend at a time")
	}

	if usageSnapshot {
		return runUsageSnapshot()
	}
	if usageTrend {
		return runUsageTrend()
	}
	return runUsageFromTranscripts()
}

func runUsageFromTranscripts() error {
	now := time.Now()
	since, err := parseUsageSinceFilter(usageLast, now)
	if err != nil {
		return err
	}

	projectsRoot, err := claudeProjectsRoot()
	if err != nil {
		return err
	}
	agg, err := aggregateUsageFromProjects(projectsRoot, since)
	if err != nil {
		return err
	}

	if agg.AssistantMessages == 0 {
		fmt.Println(style.Dim.Render("No assistant usage records found in the selected window."))
		return nil
	}

	if usageJSON {
		return outputUsageJSON(agg, since)
	}

	if usageByAgent {
		return outputUsageByAgentHuman(agg, since)
	}
	return outputUsageSummaryHuman(agg, since)
}

func aggregateUsageFromProjects(projectsRoot string, since time.Time) (*usageAggregate, error) {
	if projectsRoot == "" {
		return nil, fmt.Errorf("projects root is empty")
	}
	if _, err := os.Stat(projectsRoot); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("Claude projects directory not found: %s", projectsRoot)
		}
		return nil, fmt.Errorf("checking Claude projects directory: %w", err)
	}

	agg := &usageAggregate{
		Agents: make(map[string]*usageAgentUsage),
	}
	identityCache := make(map[string]usageIdentityLookup)

	err := filepath.WalkDir(projectsRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".jsonl" {
			return nil
		}
		if !since.IsZero() {
			if info, err := d.Info(); err == nil && info.ModTime().Before(since) {
				return nil
			}
		}
		return aggregateUsageFromTranscript(path, since, identityCache, agg)
	})
	if err != nil {
		return nil, err
	}

	return agg, nil
}

func aggregateUsageFromTranscript(
	transcriptPath string,
	since time.Time,
	identityCache map[string]usageIdentityLookup,
	agg *usageAggregate,
) error {
	f, err := os.Open(transcriptPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 256*1024)
	scanner.Buffer(buf, 8*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var rec usageTranscriptRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		if rec.Type != "assistant" || rec.Message == nil || rec.Message.Usage == nil {
			continue
		}

		ts, ok := parseUsageTimestamp(rec.Timestamp)
		if !ok {
			if !since.IsZero() {
				continue
			}
			ts = time.Time{}
		}
		if !since.IsZero() && !ts.IsZero() && ts.Before(since) {
			continue
		}

		lookup, found := identityCache[rec.CWD]
		if !found {
			identity, ok := usageIdentityForCWD(rec.CWD)
			lookup = usageIdentityLookup{identity: identity, ok: ok}
			identityCache[rec.CWD] = lookup
		}
		if !lookup.ok {
			continue
		}

		entry := agg.Agents[lookup.identity.Agent]
		if entry == nil {
			entry = &usageAgentUsage{Identity: lookup.identity}
			agg.Agents[lookup.identity.Agent] = entry
		}
		entry.Totals.add(rec.Message.Usage)
		entry.AssistantMessages++
		if !ts.IsZero() {
			if entry.FirstSeen.IsZero() || ts.Before(entry.FirstSeen) {
				entry.FirstSeen = ts
			}
			if entry.LastSeen.IsZero() || ts.After(entry.LastSeen) {
				entry.LastSeen = ts
			}
		}

		agg.Total.add(rec.Message.Usage)
		agg.AssistantMessages++
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading transcript %s: %w", transcriptPath, err)
	}
	return nil
}

func parseUsageTimestamp(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	if ts, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return ts, true
	}
	if ts, err := time.Parse(time.RFC3339, raw); err == nil {
		return ts, true
	}
	return time.Time{}, false
}

func usageIdentityForCWD(cwd string) (usageIdentity, bool) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return usageIdentity{}, false
	}

	townRoot, err := workspace.Find(cwd)
	if err != nil || townRoot == "" {
		return usageIdentity{}, false
	}
	// Use cwd-only detection for transcript attribution.
	// Environment-based role detection can point at the caller's session instead
	// of the transcript record's actual working directory.
	roleInfo := detectRole(cwd, townRoot)

	role, rig, worker, ok := usageRoleParts(roleInfo)
	if !ok {
		return usageIdentity{}, false
	}
	agent := usageCanonicalAgentIdentity(role, rig, worker)
	return usageIdentity{
		Agent:  agent,
		Role:   role,
		Rig:    rig,
		Worker: worker,
	}, true
}

func usageRoleParts(info RoleInfo) (role, rig, worker string, ok bool) {
	rig = strings.TrimSpace(info.Rig)
	worker = strings.TrimSpace(info.Polecat)

	switch info.Role {
	case RoleMayor:
		return constants.RoleMayor, "", "", true
	case RoleDeacon:
		return constants.RoleDeacon, "", "", true
	case RoleWitness:
		return constants.RoleWitness, rig, "", true
	case RoleRefinery:
		return constants.RoleRefinery, rig, "", true
	case RoleCrew:
		return constants.RoleCrew, rig, worker, true
	case RolePolecat:
		return constants.RolePolecat, rig, worker, true
	case RoleDog:
		return string(RoleDog), "", worker, true
	case RoleBoot:
		return string(RoleBoot), "", "", true
	default:
		return "", "", "", false
	}
}

func parseUsageSinceFilter(raw string, now time.Time) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}

	if dur, err := time.ParseDuration(raw); err == nil {
		if dur < 0 {
			return time.Time{}, fmt.Errorf("invalid --last %q: duration must be non-negative", raw)
		}
		return now.Add(-dur), nil
	}

	if ts, err := time.Parse(time.RFC3339, raw); err == nil {
		return ts, nil
	}

	layouts := []string{
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if ts, err := time.ParseInLocation(layout, raw, now.Location()); err == nil {
			return ts, nil
		}
	}

	return time.Time{}, fmt.Errorf("invalid --last %q (use duration like 1h/90m or timestamp like 2026-03-06T14:00:00Z)", raw)
}

func usageCanonicalAgentIdentity(role, rig, worker string) string {
	role = strings.TrimSpace(role)
	rig = strings.TrimSpace(rig)
	worker = strings.TrimSpace(worker)

	switch role {
	case constants.RoleMayor, constants.RoleDeacon:
		return role
	case constants.RoleWitness, constants.RoleRefinery:
		if rig != "" {
			return rig + "/" + role
		}
		return role
	case constants.RoleCrew, constants.RolePolecat:
		if rig != "" && worker != "" {
			return rig + "/" + role + "/" + worker
		}
		if worker != "" {
			return role + "/" + worker
		}
		if rig != "" {
			return rig + "/" + role
		}
		return role
	default:
		parts := make([]string, 0, 3)
		if rig != "" {
			parts = append(parts, rig)
		}
		if role != "" {
			parts = append(parts, role)
		}
		if worker != "" {
			parts = append(parts, worker)
		}
		if len(parts) == 0 {
			return "unknown"
		}
		return strings.Join(parts, "/")
	}
}

func outputUsageJSON(agg *usageAggregate, since time.Time) error {
	type agentRow struct {
		Agent             string           `json:"agent"`
		Role              string           `json:"role"`
		Rig               string           `json:"rig,omitempty"`
		Worker            string           `json:"worker,omitempty"`
		Totals            usageTokenTotals `json:"totals"`
		TotalTokens       int64            `json:"total_tokens"`
		AssistantMessages int              `json:"assistant_messages"`
		FirstSeen         string           `json:"first_seen,omitempty"`
		LastSeen          string           `json:"last_seen,omitempty"`
	}
	rows := make([]agentRow, 0, len(agg.Agents))
	for _, usage := range agg.Agents {
		row := agentRow{
			Agent:             usage.Identity.Agent,
			Role:              usage.Identity.Role,
			Rig:               usage.Identity.Rig,
			Worker:            usage.Identity.Worker,
			Totals:            usage.Totals,
			TotalTokens:       usage.Totals.totalTokens(),
			AssistantMessages: usage.AssistantMessages,
		}
		if !usage.FirstSeen.IsZero() {
			row.FirstSeen = usage.FirstSeen.UTC().Format(time.RFC3339)
		}
		if !usage.LastSeen.IsZero() {
			row.LastSeen = usage.LastSeen.UTC().Format(time.RFC3339)
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].TotalTokens == rows[j].TotalTokens {
			return rows[i].Agent < rows[j].Agent
		}
		return rows[i].TotalTokens > rows[j].TotalTokens
	})

	output := map[string]interface{}{
		"assistant_messages": agg.AssistantMessages,
		"total": map[string]interface{}{
			"input_tokens":                agg.Total.InputTokens,
			"output_tokens":               agg.Total.OutputTokens,
			"cache_read_input_tokens":     agg.Total.CacheReadInputTokens,
			"cache_creation_input_tokens": agg.Total.CacheCreationInputTokens,
			"total_tokens":                agg.Total.totalTokens(),
		},
		"agents": rows,
	}
	if !since.IsZero() {
		output["since"] = since.UTC().Format(time.RFC3339)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

func outputUsageSummaryHuman(agg *usageAggregate, since time.Time) error {
	fmt.Printf("\n%s Transcript Usage Summary\n", style.Bold.Render("📈"))
	if !since.IsZero() {
		fmt.Printf("%s %s\n", style.Dim.Render("Since:"), since.Local().Format(time.RFC3339))
	}
	fmt.Printf("%s %d\n", style.Bold.Render("Assistant messages:"), agg.AssistantMessages)
	fmt.Printf("%s %s\n", style.Bold.Render("Input tokens:"), formatUsageCount(agg.Total.InputTokens))
	fmt.Printf("%s %s\n", style.Bold.Render("Output tokens:"), formatUsageCount(agg.Total.OutputTokens))
	fmt.Printf("%s %s\n", style.Bold.Render("Cache read tokens:"), formatUsageCount(agg.Total.CacheReadInputTokens))
	fmt.Printf("%s %s\n", style.Bold.Render("Cache create tokens:"), formatUsageCount(agg.Total.CacheCreationInputTokens))
	fmt.Printf("%s %s\n", style.Bold.Render("Total tokens:"), formatUsageCount(agg.Total.totalTokens()))
	return nil
}

func outputUsageByAgentHuman(agg *usageAggregate, since time.Time) error {
	type row struct {
		Agent string
		usage *usageAgentUsage
	}
	rows := make([]row, 0, len(agg.Agents))
	for agent, usage := range agg.Agents {
		rows = append(rows, row{Agent: agent, usage: usage})
	}
	sort.Slice(rows, func(i, j int) bool {
		left := rows[i].usage.Totals.totalTokens()
		right := rows[j].usage.Totals.totalTokens()
		if left == right {
			return rows[i].Agent < rows[j].Agent
		}
		return left > right
	})

	fmt.Printf("\n%s Transcript Usage by Agent\n", style.Bold.Render("📈"))
	if !since.IsZero() {
		fmt.Printf("%s %s\n", style.Dim.Render("Since:"), since.Local().Format(time.RFC3339))
	}
	fmt.Printf("%s %d assistant messages\n\n", style.Dim.Render("Records:"), agg.AssistantMessages)

	fmt.Printf("%-32s %10s %10s %10s %10s %10s\n",
		"Agent", "Input", "Output", "CacheRead", "CacheNew", "Total")
	fmt.Println(strings.Repeat("─", 96))

	for _, row := range rows {
		u := row.usage.Totals
		fmt.Printf("%-32s %10s %10s %10s %10s %10s\n",
			row.Agent,
			formatUsageCount(u.InputTokens),
			formatUsageCount(u.OutputTokens),
			formatUsageCount(u.CacheReadInputTokens),
			formatUsageCount(u.CacheCreationInputTokens),
			formatUsageCount(u.totalTokens()),
		)
	}
	fmt.Println(strings.Repeat("─", 96))
	fmt.Printf("%-32s %10s %10s %10s %10s %10s\n",
		style.Bold.Render("TOTAL"),
		style.Bold.Render(formatUsageCount(agg.Total.InputTokens)),
		style.Bold.Render(formatUsageCount(agg.Total.OutputTokens)),
		style.Bold.Render(formatUsageCount(agg.Total.CacheReadInputTokens)),
		style.Bold.Render(formatUsageCount(agg.Total.CacheCreationInputTokens)),
		style.Bold.Render(formatUsageCount(agg.Total.totalTokens())),
	)
	return nil
}

func claudeProjectsRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding home directory: %w", err)
	}
	return filepath.Join(home, ".claude", "projects"), nil
}

func formatUsageCount(v int64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	s := strconv.FormatInt(v, 10)
	if len(s) <= 3 {
		if neg {
			return "-" + s
		}
		return s
	}

	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	rem := len(s) % 3
	if rem == 0 {
		rem = 3
	}
	b.WriteString(s[:rem])
	for i := rem; i < len(s); i += 3 {
		b.WriteByte(',')
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

func runUsageSnapshot() error {
	entry, err := captureUsageSnapshot()
	if err != nil {
		return err
	}
	if err := appendUsageSnapshot(entry); err != nil {
		return err
	}

	if usageJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(entry)
	}

	fmt.Printf("%s Captured /usage snapshot\n", style.Success.Render("✓"))
	fmt.Printf("  Current session: %.2f%%\n", entry.CurrentSessionPct)
	fmt.Printf("  Current week:    %.2f%%\n", entry.CurrentWeekPct)
	fmt.Printf("  Sonnet-only:     %.2f%%\n", entry.SonnetOnlyPct)
	fmt.Printf("  Saved: %s\n", usageSnapshotsPath())
	return nil
}

func runUsageTrend() error {
	entries, err := readUsageSnapshots(usageSnapshotsPath())
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Println(style.Dim.Render("No usage snapshots found. Run: gt usage --snapshot"))
		return nil
	}
	if len(entries) < 2 {
		fmt.Println(style.Dim.Render("Need at least 2 snapshots for trend."))
		return nil
	}

	points := buildUsageTrend(entries)
	if usageJSON {
		payload := map[string]interface{}{
			"snapshots": len(entries),
			"points":    points,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(payload)
	}

	fmt.Printf("\n%s Usage Trend (%% per hour)\n\n", style.Bold.Render("📉"))
	fmt.Printf("%-20s %-20s %7s %10s %10s %10s\n", "From", "To", "Hours", "Session/h", "Week/h", "Sonnet/h")
	fmt.Println(strings.Repeat("─", 86))
	for _, p := range points {
		fmt.Printf("%-20s %-20s %7.2f %10.3f %10.3f %10.3f\n",
			p.From.Local().Format("2006-01-02 15:04"),
			p.To.Local().Format("2006-01-02 15:04"),
			p.Hours,
			p.SessionPctPerH,
			p.WeekPctPerH,
			p.SonnetPctPerH,
		)
	}

	first := entries[0]
	last := entries[len(entries)-1]
	totalHours := last.Timestamp.Sub(first.Timestamp).Hours()
	if totalHours > 0 {
		fmt.Println(strings.Repeat("─", 86))
		fmt.Printf("%-20s %-20s %7.2f %10.3f %10.3f %10.3f\n",
			style.Bold.Render("OVERALL"),
			"",
			totalHours,
			(last.CurrentSessionPct-first.CurrentSessionPct)/totalHours,
			(last.CurrentWeekPct-first.CurrentWeekPct)/totalHours,
			(last.SonnetOnlyPct-first.SonnetOnlyPct)/totalHours,
		)
	}
	return nil
}

func resolveUsageAgentCommand() (string, error) {
	if cmd, err := resolveSeanceCommand(); err == nil && strings.TrimSpace(cmd) != "" {
		return cmd, nil
	}
	if p, err := exec.LookPath("claude"); err == nil {
		return p, nil
	}
	if p, err := exec.LookPath("codex"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("could not resolve Claude/Codex command for /usage snapshot")
}

func captureUsageSnapshot() (usageSnapshotEntry, error) {
	agentCmd, err := resolveUsageAgentCommand()
	if err != nil {
		return usageSnapshotEntry{}, err
	}

	socketName := fmt.Sprintf("gt-usage-%d", time.Now().UnixNano())
	sessionName := fmt.Sprintf("gt-usage-snapshot-%d", time.Now().UnixNano())
	t := tmux.NewTmuxWithSocket(socketName)

	workDir, _ := os.Getwd()
	if strings.TrimSpace(workDir) == "" {
		workDir = os.Getenv("HOME")
	}

	if err := t.NewSessionWithCommand(sessionName, workDir, agentCmd); err != nil {
		return usageSnapshotEntry{}, fmt.Errorf("starting temporary session: %w", err)
	}
	defer func() {
		_ = t.KillSessionWithProcesses(sessionName)
		_ = exec.Command("tmux", "-u", "-L", socketName, "kill-server").Run()
	}()

	_ = t.WaitForCommand(sessionName, []string{"bash", "zsh", "sh", "fish"}, 20*time.Second)
	time.Sleep(2 * time.Second)

	if err := t.SendKeys(sessionName, "/usage"); err != nil {
		return usageSnapshotEntry{}, fmt.Errorf("sending /usage command: %w", err)
	}

	deadline := time.Now().Add(45 * time.Second)
	var lastPane string
	var lastParseErr error

	for time.Now().Before(deadline) {
		pane, err := t.CapturePaneAll(sessionName)
		if err == nil {
			lastPane = pane
			currentSessionPct, currentWeekPct, sonnetOnlyPct, parseErr := parseUsageSnapshotPane(pane)
			if parseErr == nil {
				return usageSnapshotEntry{
					Timestamp:         time.Now().UTC(),
					CurrentSessionPct: currentSessionPct,
					CurrentWeekPct:    currentWeekPct,
					SonnetOnlyPct:     sonnetOnlyPct,
				}, nil
			}
			lastParseErr = parseErr
		}
		time.Sleep(1 * time.Second)
	}

	snippet := tailLines(stripANSI(lastPane), 20)
	if lastParseErr != nil {
		return usageSnapshotEntry{}, fmt.Errorf("timed out waiting for parsable /usage output: %w\nLast pane output:\n%s", lastParseErr, snippet)
	}
	return usageSnapshotEntry{}, fmt.Errorf("timed out waiting for /usage output\nLast pane output:\n%s", snippet)
}

var (
	ansiEscapeRe   = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)
	percentValueRe = regexp.MustCompile(`(\d{1,3}(?:\.\d+)?)\s*%`)
)

func parseUsageSnapshotPane(pane string) (currentSessionPct, currentWeekPct, sonnetOnlyPct float64, err error) {
	clean := stripANSI(pane)
	lines := strings.Split(clean, "\n")

	var (
		foundSession bool
		foundWeek    bool
		foundSonnet  bool
	)
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		pct, ok := firstPercentInLine(line)
		if !ok {
			continue
		}

		switch {
		case !foundSession && strings.Contains(lower, "current session"):
			currentSessionPct = pct
			foundSession = true
		case !foundWeek && strings.Contains(lower, "current week"):
			currentWeekPct = pct
			foundWeek = true
		case !foundSonnet && strings.Contains(lower, "sonnet"):
			sonnetOnlyPct = pct
			foundSonnet = true
		}
	}

	if !foundSession || !foundWeek || !foundSonnet {
		return 0, 0, 0, fmt.Errorf("missing expected /usage metrics (session=%t week=%t sonnet=%t)", foundSession, foundWeek, foundSonnet)
	}
	return currentSessionPct, currentWeekPct, sonnetOnlyPct, nil
}

func stripANSI(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	return ansiEscapeRe.ReplaceAllString(s, "")
}

func firstPercentInLine(line string) (float64, bool) {
	m := percentValueRe.FindStringSubmatch(line)
	if len(m) < 2 {
		return 0, false
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func tailLines(s string, n int) string {
	if n <= 0 {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

func usageSnapshotsPath() string {
	return filepath.Join(gtDataDir(), "usage-snapshots.jsonl")
}

func appendUsageSnapshot(entry usageSnapshotEntry) error {
	path := usageSnapshotsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating snapshot directory: %w", err)
	}

	payload, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encoding snapshot entry: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening snapshot log: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(payload, '\n')); err != nil {
		return fmt.Errorf("writing snapshot entry: %w", err)
	}
	return nil
}

func readUsageSnapshots(path string) ([]usageSnapshotEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("opening snapshot log: %w", err)
	}
	defer f.Close()

	var entries []usageSnapshotEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry usageSnapshotEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Timestamp.IsZero() {
			continue
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading snapshot log: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})
	return entries, nil
}

func buildUsageTrend(entries []usageSnapshotEntry) []usageTrendPoint {
	if len(entries) < 2 {
		return nil
	}
	points := make([]usageTrendPoint, 0, len(entries)-1)
	for i := 1; i < len(entries); i++ {
		prev := entries[i-1]
		cur := entries[i]
		hours := cur.Timestamp.Sub(prev.Timestamp).Hours()
		if hours <= 0 {
			continue
		}

		sessionDelta := cur.CurrentSessionPct - prev.CurrentSessionPct
		weekDelta := cur.CurrentWeekPct - prev.CurrentWeekPct
		sonnetDelta := cur.SonnetOnlyPct - prev.SonnetOnlyPct

		points = append(points, usageTrendPoint{
			From:            prev.Timestamp,
			To:              cur.Timestamp,
			Hours:           hours,
			SessionDeltaPct: sessionDelta,
			WeekDeltaPct:    weekDelta,
			SonnetDeltaPct:  sonnetDelta,
			SessionPctPerH:  sessionDelta / hours,
			WeekPctPerH:     weekDelta / hours,
			SonnetPctPerH:   sonnetDelta / hours,
		})
	}
	return points
}
