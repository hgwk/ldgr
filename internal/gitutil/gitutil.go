// Package gitutil wraps best-effort calls to the git CLI. All functions
// return empty string + nil error when git is unavailable or the directory
// is not a working tree — callers treat empty as "unknown".
package gitutil

import (
	"os/exec"
	"strings"
)

// CurrentBranch returns the abbreviated symbolic ref of HEAD or "" if
// detached / not a repo / git missing.
func CurrentBranch(dir string) string {
	out, err := run(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(out)
	if branch == "HEAD" {
		return ""
	}
	return branch
}

// IsWorkTree returns true if dir is inside a git working tree.
func IsWorkTree(dir string) bool {
	out, err := run(dir, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == "true"
}

// Dirty returns true if `git status --porcelain` produces any output (i.e.
// there are uncommitted changes). Returns false on errors or when git is missing.
func Dirty(dir string) bool {
	out, err := run(dir, "status", "--porcelain")
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) != ""
}

// ChangedFiles returns the relative paths of files with uncommitted changes
// (modified, added, deleted, renamed, untracked). Empty slice on error.
func ChangedFiles(dir string) []string {
	out, err := run(dir, "status", "--porcelain")
	if err != nil {
		return nil
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		// porcelain v1 format: "XY path" with X/Y as 1-char status flags.
		// Trim the leading 3 chars (status + space).
		if len(line) > 3 {
			p := strings.TrimSpace(line[3:])
			// Handle rename: "old -> new" — keep the destination.
			if idx := strings.Index(p, " -> "); idx >= 0 {
				p = p[idx+4:]
			}
			paths = append(paths, p)
		}
	}
	return paths
}

func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}
