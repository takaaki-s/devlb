package label

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDetectLabel_ExplicitOverride(t *testing.T) {
	got := DetectLabel("my-custom-label")
	if got != "my-custom-label" {
		t.Errorf("DetectLabel(explicit) = %q, want %q", got, "my-custom-label")
	}
}

func TestDetectLabel_GitRepo(t *testing.T) {
	dir := t.TempDir()

	// Initialize a git repo with a branch
	cmds := [][]string{
		{"git", "init"},
		{"git", "checkout", "-b", "feature-test"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "init"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}

	// Change to git repo dir and restore after
	orig, _ := os.Getwd()
	defer func() { _ = os.Chdir(orig) }()
	_ = os.Chdir(dir)

	got := DetectLabel("")
	if got != "feature-test" {
		t.Errorf("DetectLabel(\"\") in git repo = %q, want %q", got, "feature-test")
	}
}

func TestDetectLabel_NotGitRepo(t *testing.T) {
	dir := t.TempDir()

	orig, _ := os.Getwd()
	defer func() { _ = os.Chdir(orig) }()
	_ = os.Chdir(dir)

	got := DetectLabel("")
	if got == "" {
		t.Error("DetectLabel(\"\") in non-git dir returned empty string")
	}
	// Should return some fallback (hostname or "unknown")
	_ = got
}

func TestDetectLabel_DetachedHead(t *testing.T) {
	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "init"},
		{"git", "checkout", "--detach"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}

	// Get short SHA for comparison
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	cmd.Dir = dir
	shortSHA, _ := cmd.Output()

	orig, _ := os.Getwd()
	defer func() { _ = os.Chdir(orig) }()
	_ = os.Chdir(dir)

	got := DetectLabel("")
	expected := filepath.Base(string(shortSHA[:len(shortSHA)-1])) // trim newline
	if got != expected && got != "HEAD" {
		// Accept either short SHA or "HEAD" as valid detached head labels
		t.Errorf("DetectLabel(\"\") detached HEAD = %q, want %q or %q", got, expected, "HEAD")
	}
}
