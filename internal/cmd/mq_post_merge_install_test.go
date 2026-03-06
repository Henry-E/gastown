package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}

func commitFile(t *testing.T, dir, name, content, msg string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	runGit(t, dir, "add", name)
	runGit(t, dir, "commit", "-m", msg)
}

func setupGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	commitFile(t, repo, "README.md", "init\n", "init")
	return repo
}

func TestMergedGoFilesChanged_WhenGoTouched(t *testing.T) {
	repo := setupGitRepo(t)
	commitFile(t, repo, "internal/cmd/main.go", "package main\n", "go change")

	changed, files, err := mergedGoFilesChanged(repo)
	if err != nil {
		t.Fatalf("mergedGoFilesChanged() error = %v", err)
	}
	if !changed {
		t.Fatalf("mergedGoFilesChanged() changed = false, want true")
	}
	if len(files) != 1 || files[0] != "internal/cmd/main.go" {
		t.Fatalf("mergedGoFilesChanged() files = %v, want [internal/cmd/main.go]", files)
	}
}

func TestMergedGoFilesChanged_WhenNoGoTouched(t *testing.T) {
	repo := setupGitRepo(t)
	commitFile(t, repo, "docs/notes.txt", "hello\n", "text change")

	changed, files, err := mergedGoFilesChanged(repo)
	if err != nil {
		t.Fatalf("mergedGoFilesChanged() error = %v", err)
	}
	if changed {
		t.Fatalf("mergedGoFilesChanged() changed = true, want false")
	}
	if len(files) != 0 {
		t.Fatalf("mergedGoFilesChanged() files = %v, want []", files)
	}
}
