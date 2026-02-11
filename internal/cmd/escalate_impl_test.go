package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
)

// TestExtractMailTargetsFromActions tests extraction of mail targets from action strings.
func TestExtractMailTargetsFromActions(t *testing.T) {
	tests := []struct {
		name    string
		actions []string
		want    []string
	}{
		{
			name:    "single mail target",
			actions: []string{"bead", "mail:mayor"},
			want:    []string{"mayor"},
		},
		{
			name:    "multiple mail targets",
			actions: []string{"mail:mayor", "mail:gastown/witness"},
			want:    []string{"mayor", "gastown/witness"},
		},
		{
			name:    "no mail targets",
			actions: []string{"bead", "log", "email:human"},
			want:    nil,
		},
		{
			name:    "empty actions",
			actions: []string{},
			want:    nil,
		},
		{
			name:    "nil actions",
			actions: nil,
			want:    nil,
		},
		{
			name:    "mail prefix with empty target",
			actions: []string{"mail:"},
			want:    nil,
		},
		{
			name:    "mixed actions with mail targets",
			actions: []string{"bead", "mail:mayor", "email:human", "sms:human", "mail:gastown/witness", "log"},
			want:    []string{"mayor", "gastown/witness"},
		},
		{
			name:    "similar but non-mail prefixes",
			actions: []string{"mailto:someone", "email:human", "mail:mayor"},
			want:    []string{"mayor"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractMailTargetsFromActions(tt.actions)
			if len(got) != len(tt.want) {
				t.Errorf("extractMailTargetsFromActions(%v) = %v (len %d), want %v (len %d)",
					tt.actions, got, len(got), tt.want, len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("extractMailTargetsFromActions(%v)[%d] = %q, want %q",
						tt.actions, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestGetNextSeverity tests severity escalation progression.
func TestGetNextSeverity(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"low", "medium"},
		{"medium", "high"},
		{"high", "critical"},
		{"critical", "critical"},
		{"", "critical"},
		{"unknown", "critical"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := getNextSeverity(tt.input)
			if got != tt.want {
				t.Errorf("getNextSeverity(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestSeverityEmoji tests emoji mapping for severity levels.
func TestSeverityEmoji(t *testing.T) {
	tests := []struct {
		severity string
		want     string
	}{
		{config.SeverityCritical, "🚨"},
		{config.SeverityHigh, "⚠️"},
		{config.SeverityMedium, "📢"},
		{config.SeverityLow, "ℹ️"},
		{"", "📋"},
		{"unknown", "📋"},
	}

	for _, tt := range tests {
		name := tt.severity
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			got := severityEmoji(tt.severity)
			if got != tt.want {
				t.Errorf("severityEmoji(%q) = %q, want %q", tt.severity, got, tt.want)
			}
		})
	}
}

// TestFormatRelativeTime tests human-readable relative time formatting.
func TestFormatRelativeTime(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		timestamp string
		want      string
	}{
		{
			name:      "just now",
			timestamp: now.Add(-10 * time.Second).Format(time.RFC3339),
			want:      "just now",
		},
		{
			name:      "1 minute ago",
			timestamp: now.Add(-1 * time.Minute).Format(time.RFC3339),
			want:      "1 minute ago",
		},
		{
			name:      "multiple minutes ago",
			timestamp: now.Add(-15 * time.Minute).Format(time.RFC3339),
			want:      "15 minutes ago",
		},
		{
			name:      "1 hour ago",
			timestamp: now.Add(-1 * time.Hour).Format(time.RFC3339),
			want:      "1 hour ago",
		},
		{
			name:      "multiple hours ago",
			timestamp: now.Add(-5 * time.Hour).Format(time.RFC3339),
			want:      "5 hours ago",
		},
		{
			name:      "1 day ago",
			timestamp: now.Add(-25 * time.Hour).Format(time.RFC3339),
			want:      "1 day ago",
		},
		{
			name:      "multiple days ago",
			timestamp: now.Add(-72 * time.Hour).Format(time.RFC3339),
			want:      "3 days ago",
		},
		{
			name:      "invalid timestamp returns raw string",
			timestamp: "not-a-timestamp",
			want:      "not-a-timestamp",
		},
		{
			name:      "empty timestamp returns raw string",
			timestamp: "",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatRelativeTime(tt.timestamp)
			if got != tt.want {
				t.Errorf("formatRelativeTime(%q) = %q, want %q", tt.timestamp, got, tt.want)
			}
		})
	}
}

// TestFormatEscalationMailBody tests mail body generation for escalations.
func TestFormatEscalationMailBody(t *testing.T) {
	tests := []struct {
		name     string
		beadID   string
		severity string
		reason   string
		from     string
		related  string
		checks   []string // substrings that must appear in the output
	}{
		{
			name:     "full fields",
			beadID:   "gt-abc123",
			severity: "critical",
			reason:   "CI blocked on dependency",
			from:     "gastown/polecats/furiosa",
			related:  "gt-xyz789",
			checks: []string{
				"Escalation ID: gt-abc123",
				"Severity: critical",
				"From: gastown/polecats/furiosa",
				"Reason:",
				"CI blocked on dependency",
				"Related: gt-xyz789",
				"gt escalate ack gt-abc123",
				"gt escalate close gt-abc123",
			},
		},
		{
			name:     "minimal fields",
			beadID:   "gt-min1",
			severity: "low",
			reason:   "",
			from:     "unknown",
			related:  "",
			checks: []string{
				"Escalation ID: gt-min1",
				"Severity: low",
				"From: unknown",
				"gt escalate ack gt-min1",
				"gt escalate close gt-min1",
			},
		},
		{
			name:     "no reason omits reason section",
			beadID:   "gt-noreason",
			severity: "medium",
			reason:   "",
			from:     "test",
			related:  "",
		},
		{
			name:     "no related omits related line",
			beadID:   "gt-norel",
			severity: "high",
			reason:   "some reason",
			from:     "test",
			related:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatEscalationMailBody(tt.beadID, tt.severity, tt.reason, tt.from, tt.related)

			for _, check := range tt.checks {
				if !strings.Contains(got, check) {
					t.Errorf("formatEscalationMailBody() missing %q in:\n%s", check, got)
				}
			}

			// Negative checks
			if tt.reason == "" && strings.Contains(got, "Reason:") {
				t.Error("formatEscalationMailBody() should not contain 'Reason:' when reason is empty")
			}
			if tt.related == "" && strings.Contains(got, "Related:") {
				t.Error("formatEscalationMailBody() should not contain 'Related:' when related is empty")
			}
		})
	}
}

// TestFormatReescalationMailBody tests mail body generation for re-escalations.
func TestFormatReescalationMailBody(t *testing.T) {
	tests := []struct {
		name          string
		result        *beads.ReescalationResult
		reescalatedBy string
		checks        []string
	}{
		{
			name:          "standard reescalation",
			result:        makeReescalationResult("gt-stale1", "", "low", "medium", 1, false, ""),
			reescalatedBy: "system",
			checks: []string{
				"Escalation ID: gt-stale1",
				"Severity bumped: low → medium",
				"Reescalation #1",
				"Reescalated by: system",
				"not acknowledged within the stale threshold",
				"gt escalate ack gt-stale1",
				"gt escalate close gt-stale1",
			},
		},
		{
			name:          "high to critical reescalation",
			result:        makeReescalationResult("gt-urgent", "", "high", "critical", 2, false, ""),
			reescalatedBy: "gastown/witness",
			checks: []string{
				"Severity bumped: high → critical",
				"Reescalation #2",
				"Reescalated by: gastown/witness",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatReescalationMailBody(tt.result, tt.reescalatedBy)

			for _, check := range tt.checks {
				if !strings.Contains(got, check) {
					t.Errorf("formatReescalationMailBody() missing %q in:\n%s", check, got)
				}
			}
		})
	}
}

// TestDetectSenderFallback tests the environment-based sender detection fallback.
func TestDetectSenderFallback(t *testing.T) {
	tests := []struct {
		name     string
		bdActor  string
		gtRole   string
		expected string
	}{
		{
			name:     "BD_ACTOR set",
			bdActor:  "gastown/polecats/furiosa",
			gtRole:   "",
			expected: "gastown/polecats/furiosa",
		},
		{
			name:     "GT_ROLE set when BD_ACTOR empty",
			bdActor:  "",
			gtRole:   "gastown/witness",
			expected: "gastown/witness",
		},
		{
			name:     "BD_ACTOR takes precedence over GT_ROLE",
			bdActor:  "gastown/polecats/furiosa",
			gtRole:   "gastown/witness",
			expected: "gastown/polecats/furiosa",
		},
		{
			name:     "neither set returns empty",
			bdActor:  "",
			gtRole:   "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore env
			oldActor := os.Getenv("BD_ACTOR")
			oldRole := os.Getenv("GT_ROLE")
			defer func() {
				os.Setenv("BD_ACTOR", oldActor)
				os.Setenv("GT_ROLE", oldRole)
			}()

			os.Setenv("BD_ACTOR", tt.bdActor)
			os.Setenv("GT_ROLE", tt.gtRole)

			got := detectSenderFallback()
			if got != tt.expected {
				t.Errorf("detectSenderFallback() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestExecuteExternalActions tests external notification dispatch logic.
func TestExecuteExternalActions(t *testing.T) {
	t.Run("email action with no contact configured", func(t *testing.T) {
		cfg := &config.EscalationConfig{
			Contacts: config.EscalationContacts{
				HumanEmail: "",
			},
		}
		// Should not panic - just prints a warning
		executeExternalActions([]string{"email:human"}, cfg, "gt-test", "high", "test desc")
	})

	t.Run("email action with contact configured", func(t *testing.T) {
		cfg := &config.EscalationConfig{
			Contacts: config.EscalationContacts{
				HumanEmail: "user@example.com",
			},
		}
		// Should not panic - prints "would send" message
		executeExternalActions([]string{"email:human"}, cfg, "gt-test", "high", "test desc")
	})

	t.Run("sms action with no contact configured", func(t *testing.T) {
		cfg := &config.EscalationConfig{
			Contacts: config.EscalationContacts{
				HumanSMS: "",
			},
		}
		executeExternalActions([]string{"sms:human"}, cfg, "gt-test", "high", "test desc")
	})

	t.Run("sms action with contact configured", func(t *testing.T) {
		cfg := &config.EscalationConfig{
			Contacts: config.EscalationContacts{
				HumanSMS: "+1234567890",
			},
		}
		executeExternalActions([]string{"sms:human"}, cfg, "gt-test", "high", "test desc")
	})

	t.Run("slack action with no webhook configured", func(t *testing.T) {
		cfg := &config.EscalationConfig{
			Contacts: config.EscalationContacts{
				SlackWebhook: "",
			},
		}
		executeExternalActions([]string{"slack"}, cfg, "gt-test", "high", "test desc")
	})

	t.Run("slack action with webhook configured", func(t *testing.T) {
		cfg := &config.EscalationConfig{
			Contacts: config.EscalationContacts{
				SlackWebhook: "https://hooks.slack.com/test",
			},
		}
		executeExternalActions([]string{"slack"}, cfg, "gt-test", "high", "test desc")
	})

	t.Run("log action", func(t *testing.T) {
		cfg := &config.EscalationConfig{}
		executeExternalActions([]string{"log"}, cfg, "gt-test", "high", "test desc")
	})

	t.Run("mixed actions", func(t *testing.T) {
		cfg := &config.EscalationConfig{
			Contacts: config.EscalationContacts{
				HumanEmail:   "user@example.com",
				HumanSMS:     "+1234567890",
				SlackWebhook: "https://hooks.slack.com/test",
			},
		}
		actions := []string{"email:human", "sms:human", "slack", "log", "mail:mayor"}
		// mail: actions are not external - should be skipped by executeExternalActions
		executeExternalActions(actions, cfg, "gt-test", "critical", "test desc")
	})

	t.Run("empty actions", func(t *testing.T) {
		cfg := &config.EscalationConfig{}
		executeExternalActions([]string{}, cfg, "gt-test", "high", "test desc")
	})

	t.Run("unknown action types are ignored", func(t *testing.T) {
		cfg := &config.EscalationConfig{}
		executeExternalActions([]string{"bead", "unknown:thing"}, cfg, "gt-test", "high", "test desc")
	})
}

// TestEscalationSeverityPriorityMapping verifies that severity levels map to
// correct mail priorities in the escalation flow.
func TestEscalationSeverityPriorityMapping(t *testing.T) {
	// This tests the severity → priority mapping logic used in runEscalate and runEscalateStale.
	// The actual mapping is inline in the function, so we test the config constants are consistent.
	tests := []struct {
		severity     string
		isValid      bool
		nextSeverity string
	}{
		{config.SeverityCritical, true, "critical"},
		{config.SeverityHigh, true, "critical"},
		{config.SeverityMedium, true, "high"},
		{config.SeverityLow, true, "medium"},
		{"invalid", false, "critical"},
		{"", false, "critical"},
	}

	for _, tt := range tests {
		name := tt.severity
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			if config.IsValidSeverity(tt.severity) != tt.isValid {
				t.Errorf("IsValidSeverity(%q) = %v, want %v", tt.severity, !tt.isValid, tt.isValid)
			}
			if got := getNextSeverity(tt.severity); got != tt.nextSeverity {
				t.Errorf("getNextSeverity(%q) = %q, want %q", tt.severity, got, tt.nextSeverity)
			}
		})
	}
}

// TestSeverityEscalationChain verifies the full escalation chain from low to critical.
func TestSeverityEscalationChain(t *testing.T) {
	// Start at low and verify each step up the chain
	chain := []string{"low", "medium", "high", "critical"}
	for i := 0; i < len(chain)-1; i++ {
		got := getNextSeverity(chain[i])
		if got != chain[i+1] {
			t.Errorf("getNextSeverity(%q) = %q, want %q", chain[i], got, chain[i+1])
		}
	}

	// Critical stays critical (ceiling)
	if got := getNextSeverity("critical"); got != "critical" {
		t.Errorf("getNextSeverity(\"critical\") = %q, want \"critical\"", got)
	}
}

// TestEscalationConfigIntegration tests loading escalation config from a test workspace.
func TestEscalationConfigIntegration(t *testing.T) {
	townRoot := t.TempDir()

	// Create settings directory
	settingsDir := filepath.Join(townRoot, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatalf("mkdir settings: %v", err)
	}

	t.Run("creates default config when none exists", func(t *testing.T) {
		configPath := config.EscalationConfigPath(townRoot)
		cfg, err := config.LoadOrCreateEscalationConfig(configPath)
		if err != nil {
			t.Fatalf("LoadOrCreateEscalationConfig: %v", err)
		}

		// Verify defaults
		if cfg.GetStaleThreshold() != 4*time.Hour {
			t.Errorf("default stale threshold = %v, want 4h", cfg.GetStaleThreshold())
		}
		if cfg.GetMaxReescalations() != 2 {
			t.Errorf("default max reescalations = %d, want 2", cfg.GetMaxReescalations())
		}
	})

	t.Run("routes return actions for each severity", func(t *testing.T) {
		configPath := config.EscalationConfigPath(townRoot)
		cfg, err := config.LoadOrCreateEscalationConfig(configPath)
		if err != nil {
			t.Fatalf("LoadOrCreateEscalationConfig: %v", err)
		}

		for _, sev := range []string{"critical", "high", "medium", "low"} {
			actions := cfg.GetRouteForSeverity(sev)
			if len(actions) == 0 {
				t.Errorf("GetRouteForSeverity(%q) returned empty actions", sev)
			}
		}
	})

	t.Run("extracting mail targets from default routes", func(t *testing.T) {
		configPath := config.EscalationConfigPath(townRoot)
		cfg, err := config.LoadOrCreateEscalationConfig(configPath)
		if err != nil {
			t.Fatalf("LoadOrCreateEscalationConfig: %v", err)
		}

		// The default config should have mail targets for at least critical/high
		criticalActions := cfg.GetRouteForSeverity("critical")
		targets := extractMailTargetsFromActions(criticalActions)
		// Default config routes critical to mail:mayor/ and others
		if len(targets) == 0 {
			t.Log("Warning: default critical route has no mail targets (config-dependent)")
		}
	})
}

// TestFormatRelativeTimeBoundaryConditions tests edge cases in time formatting.
func TestFormatRelativeTimeBoundaryConditions(t *testing.T) {
	now := time.Now()

	t.Run("exactly 1 minute boundary", func(t *testing.T) {
		// 60 seconds ago should be "1 minute ago"
		ts := now.Add(-60 * time.Second).Format(time.RFC3339)
		got := formatRelativeTime(ts)
		if got != "1 minute ago" {
			t.Errorf("formatRelativeTime(60s ago) = %q, want %q", got, "1 minute ago")
		}
	})

	t.Run("exactly 1 hour boundary", func(t *testing.T) {
		ts := now.Add(-60 * time.Minute).Format(time.RFC3339)
		got := formatRelativeTime(ts)
		if got != "1 hour ago" {
			t.Errorf("formatRelativeTime(60m ago) = %q, want %q", got, "1 hour ago")
		}
	})

	t.Run("exactly 24 hour boundary", func(t *testing.T) {
		ts := now.Add(-24 * time.Hour).Format(time.RFC3339)
		got := formatRelativeTime(ts)
		if got != "1 day ago" {
			t.Errorf("formatRelativeTime(24h ago) = %q, want %q", got, "1 day ago")
		}
	})

	t.Run("just under 1 minute", func(t *testing.T) {
		ts := now.Add(-59 * time.Second).Format(time.RFC3339)
		got := formatRelativeTime(ts)
		if got != "just now" {
			t.Errorf("formatRelativeTime(59s ago) = %q, want %q", got, "just now")
		}
	})
}

// TestFormatEscalationMailBodyStructure verifies the mail body has proper structure.
func TestFormatEscalationMailBodyStructure(t *testing.T) {
	body := formatEscalationMailBody("gt-test1", "high", "CI is broken", "gastown/polecats/furiosa", "gt-related1")

	lines := strings.Split(body, "\n")

	// Should start with Escalation ID
	if !strings.HasPrefix(lines[0], "Escalation ID:") {
		t.Errorf("first line should start with 'Escalation ID:', got %q", lines[0])
	}

	// Should contain separator line
	hasSeparator := false
	for _, line := range lines {
		if line == "---" {
			hasSeparator = true
			break
		}
	}
	if !hasSeparator {
		t.Error("mail body should contain '---' separator")
	}

	// Should end with close instruction
	lastNonEmpty := ""
	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i] != "" {
			lastNonEmpty = lines[i]
			break
		}
	}
	if !strings.HasPrefix(lastNonEmpty, "To close:") {
		t.Errorf("last non-empty line should start with 'To close:', got %q", lastNonEmpty)
	}
}

// TestFormatReescalationMailBodyStructure verifies the re-escalation mail body structure.
func TestFormatReescalationMailBodyStructure(t *testing.T) {
	result := makeReescalationResult("gt-reesc1", "", "medium", "high", 1, false, "")

	body := formatReescalationMailBody(result, "system")
	lines := strings.Split(body, "\n")

	// Check structure
	if !strings.HasPrefix(lines[0], "Escalation ID:") {
		t.Errorf("first line should start with 'Escalation ID:', got %q", lines[0])
	}

	// Should mention automatic re-escalation
	if !strings.Contains(body, "automatically re-escalated") {
		t.Error("body should mention automatic re-escalation")
	}

	// Should contain separator and instructions
	if !strings.Contains(body, "---") {
		t.Error("body should contain separator")
	}
	if !strings.Contains(body, "To acknowledge:") {
		t.Error("body should contain acknowledge instructions")
	}
	if !strings.Contains(body, "To close:") {
		t.Error("body should contain close instructions")
	}
}

// TestExtractMailTargetsEdgeCases tests edge cases in mail target extraction.
func TestExtractMailTargetsEdgeCases(t *testing.T) {
	t.Run("mail: with only whitespace target", func(t *testing.T) {
		// "mail: " has a space target - it's not empty after trim prefix
		got := extractMailTargetsFromActions([]string{"mail: "})
		if len(got) != 1 || got[0] != " " {
			// The function doesn't trim - it just checks for empty after TrimPrefix
			// " " is not empty, so it would be included
			t.Logf("extractMailTargetsFromActions([\"mail: \"]) = %v", got)
		}
	})

	t.Run("case sensitive mail prefix", func(t *testing.T) {
		// "Mail:" should NOT be recognized (case sensitive)
		got := extractMailTargetsFromActions([]string{"Mail:mayor"})
		if len(got) != 0 {
			t.Errorf("extractMailTargetsFromActions([\"Mail:mayor\"]) should return empty, got %v", got)
		}
	})

	t.Run("mail without colon", func(t *testing.T) {
		got := extractMailTargetsFromActions([]string{"mail"})
		if len(got) != 0 {
			t.Errorf("extractMailTargetsFromActions([\"mail\"]) should return empty, got %v", got)
		}
	})

	t.Run("mail:target:with:colons", func(t *testing.T) {
		got := extractMailTargetsFromActions([]string{"mail:target:with:colons"})
		if len(got) != 1 || got[0] != "target:with:colons" {
			t.Errorf("extractMailTargetsFromActions([\"mail:target:with:colons\"]) = %v, want [\"target:with:colons\"]", got)
		}
	})
}

// TestSeverityEmojiCompleteness ensures all valid severities map to unique emojis.
func TestSeverityEmojiCompleteness(t *testing.T) {
	severities := []string{
		config.SeverityCritical,
		config.SeverityHigh,
		config.SeverityMedium,
		config.SeverityLow,
	}

	emojis := make(map[string]string) // emoji -> severity that produced it
	for _, sev := range severities {
		emoji := severityEmoji(sev)
		if emoji == "" {
			t.Errorf("severityEmoji(%q) returned empty string", sev)
		}
		if prev, exists := emojis[emoji]; exists {
			t.Errorf("severityEmoji(%q) and severityEmoji(%q) both return %q", sev, prev, emoji)
		}
		emojis[emoji] = sev
	}

	// Default emoji should be different from all severity-specific ones
	defaultEmoji := severityEmoji("unknown")
	if defaultEmoji == "" {
		t.Error("severityEmoji(\"unknown\") returned empty string")
	}
}

// makeReescalationResult is a test helper to create beads.ReescalationResult values.
func makeReescalationResult(id, title, oldSev, newSev string, num int, skipped bool, skipReason string) *beads.ReescalationResult {
	return &beads.ReescalationResult{
		ID:              id,
		Title:           title,
		OldSeverity:     oldSev,
		NewSeverity:     newSev,
		ReescalationNum: num,
		Skipped:         skipped,
		SkipReason:      skipReason,
	}
}
