package label

import (
	"os"
	"os/exec"
	"strings"
)

// DetectLabel returns a label for the current working context.
// If explicit is non-empty, it is returned as-is.
// Otherwise, attempts to detect the current git branch name.
// Falls back to hostname or "unknown" if not in a git repo.
func DetectLabel(explicit string) string {
	if explicit != "" {
		return explicit
	}

	// Try git branch name
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err == nil {
		branch := strings.TrimSpace(string(out))
		if branch != "" && branch != "HEAD" {
			return branch
		}
		// Detached HEAD — try short SHA
		if branch == "HEAD" {
			cmd2 := exec.Command("git", "rev-parse", "--short", "HEAD")
			out2, err2 := cmd2.Output()
			if err2 == nil {
				return strings.TrimSpace(string(out2))
			}
		}
	}

	// Fallback to hostname
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		return hostname
	}

	return "unknown"
}
