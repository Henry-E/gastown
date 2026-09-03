package tmux

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeExecutableTestScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-dialog.sh")
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatalf("write fake dialog: %v", err)
	}
	return path
}

func waitForPaneText(t *testing.T, tm *Tmux, sessionName, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		content, err := tm.CapturePaneVisible(sessionName)
		if err == nil && strings.Contains(content, want) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("pane %q never displayed %q", sessionName, want)
}

// TestAcceptWorkspaceTrustDialog_NoDialog verifies that when no trust dialog
// is present (agent prompt visible), the function returns quickly without error.
func TestAcceptWorkspaceTrustDialog_NoDialog(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-trust-nodlg-" + t.Name()

	_ = tm.KillSession(sessionName)
	if err := tm.NewSession(sessionName, ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer func() { _ = tm.KillSession(sessionName) }()

	// Session starts with a shell prompt containing ">", "$", or "%"
	// The polling loop should exit early when it sees the prompt.
	start := time.Now()
	err := tm.AcceptWorkspaceTrustDialog(sessionName)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("AcceptWorkspaceTrustDialog: %v", err)
	}

	// Should complete well before the 8s timeout since prompt is visible
	if elapsed > 6*time.Second {
		t.Errorf("took %v, expected early exit (< 6s)", elapsed)
	}
}

// TestAcceptWorkspaceTrustDialog_VerifiesClaudeSelection exercises a stateful
// default-No dialog. The fake deliberately ignores the first Down key so the
// handler must observe that the selection did not move and retry it.
func TestAcceptWorkspaceTrustDialog_VerifiesClaudeSelection(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-trust-dlg-" + t.Name()

	_ = tm.KillSession(sessionName)
	script := writeExecutableTestScript(t, `#!/usr/bin/env bash
set -eu
draw_no() {
  printf '\033[3J\033[2J\033[HQuick safety check: Is this a project you created or one you trust?\n\n❯ No, exit\n  Yes, I trust this folder\n\nEnter to confirm\n'
}
draw_yes() {
  printf '\033[3J\033[2J\033[HQuick safety check: Is this a project you created or one you trust?\n\n  No, exit\n❯ Yes, I trust this folder\n\nEnter to confirm\n'
}
stty raw -echo
sleep 0.3
draw_no
dd bs=1 count=3 of=/dev/null 2>/dev/null
draw_no
dd bs=1 count=3 of=/dev/null 2>/dev/null
draw_yes
dd bs=1 count=1 of=/dev/null 2>/dev/null
printf '\033[3J\033[2J\033[HFAKE_AGENT_READY\n❯ '
sleep 30
`)
	if err := tm.NewSessionWithCommand(sessionName, "", script); err != nil {
		t.Fatalf("NewSessionWithCommand: %v", err)
	}
	defer func() { _ = tm.KillSession(sessionName) }()

	if err := tm.AcceptWorkspaceTrustDialog(sessionName); err != nil {
		t.Fatalf("AcceptWorkspaceTrustDialog: %v", err)
	}
	waitForPaneText(t, tm, sessionName, "FAKE_AGENT_READY")
}

// TestAcceptWorkspaceTrustDialog_DetectsCodexDialog verifies that Codex's
// workspace trust prompt is treated as a trust dialog instead of an agent prompt.
func TestAcceptWorkspaceTrustDialog_DetectsCodexDialog(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-trust-codex-" + t.Name()

	_ = tm.KillSession(sessionName)
	script := writeExecutableTestScript(t, `#!/usr/bin/env bash
set -eu
stty raw -echo
sleep 0.3
printf '\033[3J\033[2J\033[H> You are in /tmp/demo\nDo you trust the contents of this directory?\n'
dd bs=1 count=1 of=/dev/null 2>/dev/null
printf '\033[3J\033[2J\033[HFAKE_CODEX_READY\n> '
sleep 30
`)
	if err := tm.NewSessionWithCommand(sessionName, "", script); err != nil {
		t.Fatalf("NewSessionWithCommand: %v", err)
	}
	defer func() { _ = tm.KillSession(sessionName) }()

	if err := tm.AcceptWorkspaceTrustDialog(sessionName); err != nil {
		t.Fatalf("AcceptWorkspaceTrustDialog: %v", err)
	}
	waitForPaneText(t, tm, sessionName, "FAKE_CODEX_READY")
}

// TestAcceptBypassPermissionsWarning_NoDialog verifies that when no bypass
// permissions dialog is present, the function returns quickly without error.
func TestAcceptBypassPermissionsWarning_NoDialog(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-bypass-nodlg-" + t.Name()

	_ = tm.KillSession(sessionName)
	if err := tm.NewSession(sessionName, ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer func() { _ = tm.KillSession(sessionName) }()

	start := time.Now()
	err := tm.AcceptBypassPermissionsWarning(sessionName)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("AcceptBypassPermissionsWarning: %v", err)
	}

	if elapsed > 6*time.Second {
		t.Errorf("took %v, expected early exit (< 6s)", elapsed)
	}
}

// TestAcceptBypassPermissionsWarning_DetectsDialog verifies that when bypass
// permissions dialog text appears in the pane, it is detected and accepted.
func TestAcceptBypassPermissionsWarning_DetectsDialog(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-bypass-dlg-" + t.Name()

	_ = tm.KillSession(sessionName)
	if err := tm.NewSession(sessionName, ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer func() { _ = tm.KillSession(sessionName) }()

	// Simulate the bypass permissions dialog
	if err := tm.SendKeys(sessionName, "echo 'Bypass Permissions mode is enabled'"); err != nil {
		t.Fatalf("SendKeys: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	err := tm.AcceptBypassPermissionsWarning(sessionName)
	if err != nil {
		t.Fatalf("AcceptBypassPermissionsWarning: %v", err)
	}
}

// TestAcceptStartupDialogs_NoDialogs verifies the combined function returns
// quickly when no dialogs are present.
func TestAcceptStartupDialogs_NoDialogs(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-startup-nodlg-" + t.Name()

	_ = tm.KillSession(sessionName)
	if err := tm.NewSession(sessionName, ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer func() { _ = tm.KillSession(sessionName) }()

	start := time.Now()
	err := tm.AcceptStartupDialogs(sessionName)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("AcceptStartupDialogs: %v", err)
	}

	// Both dialog checks should early-exit when prompt is visible
	if elapsed > 12*time.Second {
		t.Errorf("took %v, expected faster completion", elapsed)
	}
}

// TestAcceptWorkspaceTrustDialog_InvalidSession verifies error handling
// when the session doesn't exist.
func TestAcceptWorkspaceTrustDialog_InvalidSession(t *testing.T) {
	tm := newTestTmux(t)

	// Should not panic or hang — should return nil after timeout
	err := tm.AcceptWorkspaceTrustDialog("gt-nonexistent-session-xyz")
	// CapturePane errors are retried until timeout, then returns nil
	if err != nil {
		t.Fatalf("expected nil error for nonexistent session, got: %v", err)
	}
}

// TestContainsPromptIndicator verifies the prompt detection helper
// recognizes various shell and agent prompt patterns.
func TestContainsPromptIndicator(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"claude prompt", "Hello! How can I help?\n>", true},
		{"bash prompt", "user@host:~$", true},
		{"zsh prompt", "╰─❯", true},
		{"root prompt", "root@host:~#", true},
		{"csh prompt", "host%", true},
		{"dialog text only", "Quick safety check\nDo you trust this folder?", false},
		{"empty", "", false},
		{"whitespace only", "   \n  \n  ", false},
		{"bypass dialog", "Bypass Permissions mode\n1. No\n2. Yes, I accept", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsPromptIndicator(tt.content)
			if got != tt.want {
				t.Errorf("containsPromptIndicator(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}

func TestContainsWorkspaceTrustDialog(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"claude trust prompt", "Quick safety check\nDo you trust this folder?", true},
		{"codex trust prompt", "> You are in /tmp/demo\nDo you trust the contents of this directory?", true},
		{"bypass dialog", "Bypass Permissions mode\n1. No\n2. Yes, I accept", false},
		{"shell prompt", "user@host:~$", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsWorkspaceTrustDialog(tt.content)
			if got != tt.want {
				t.Errorf("containsWorkspaceTrustDialog(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}

func TestClaudeWorkspaceTrustSelection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		want    workspaceTrustSelection
	}{
		{
			name:    "claude current default no",
			content: "Quick safety check\nIs this a project you created or one you trust?\n❯ No, exit\n  Yes, I trust this folder",
			want:    workspaceTrustSelectionNo,
		},
		{
			name:    "claude yes selected",
			content: "Quick safety check\nIs this a project you created or one you trust?\n  No, exit\n❯ Yes, I trust this folder",
			want:    workspaceTrustSelectionYes,
		},
		{
			name:    "claude wording without selection",
			content: "Do you trust this folder?",
			want:    workspaceTrustSelectionUnknown,
		},
		{
			name:    "codex trust screen",
			content: "Do you trust the contents of this directory?",
			want:    workspaceTrustSelectionUnknown,
		},
		{
			name:    "ordinary prompt",
			content: "❯ ",
			want:    workspaceTrustSelectionUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := claudeWorkspaceTrustSelection(tt.content); got != tt.want {
				t.Fatalf("claudeWorkspaceTrustSelection(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}

// TestDismissStartupDialogsBlind_SendsKeys verifies that the blind dismiss
// sends keys without error on a valid session (no screen-scraping).
func TestDismissStartupDialogsBlind_SendsKeys(t *testing.T) {
	tm := newTestTmux(t)
	sessionName := "gt-test-blind-dismiss-" + t.Name()

	_ = tm.KillSession(sessionName)
	if err := tm.NewSession(sessionName, ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer func() { _ = tm.KillSession(sessionName) }()

	// Should complete quickly — no polling, no CapturePane
	start := time.Now()
	err := tm.DismissStartupDialogsBlind(sessionName)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("DismissStartupDialogsBlind: %v", err)
	}

	// Should take ~700ms (500ms + 200ms sleeps) — not the 8s+ dialog poll timeout
	if elapsed > 3*time.Second {
		t.Errorf("took %v, expected ~700ms (no polling)", elapsed)
	}
}

// TestDismissStartupDialogsBlind_InvalidSession verifies error handling
// when the session doesn't exist.
func TestDismissStartupDialogsBlind_InvalidSession(t *testing.T) {
	tm := newTestTmux(t)

	err := tm.DismissStartupDialogsBlind("gt-nonexistent-session-blind-xyz")
	// Should return an error since the session doesn't exist
	if err == nil {
		t.Error("expected error for nonexistent session, got nil")
	}
}
