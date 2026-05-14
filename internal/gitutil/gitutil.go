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

func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}
