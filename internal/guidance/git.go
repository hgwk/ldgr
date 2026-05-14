package guidance

import "github.com/hgwk/ldgr/internal/ledger"

// GitFindings carries diagnostics derived from git working tree vs ticket paths.
type GitFindings struct {
	Untracked   []string `json:"untracked"`    // changed files not covered by any active ticket's paths
	IdleTickets []string `json:"idle_tickets"` // active tickets whose paths all match no changed file
}

// CompareGitToTickets computes GitFindings. `changedFiles` is the git porcelain
// list; `tickets` is the slice of latest rows for active tickets only (caller
// supplies the filtering for project mode); `focus` may be a single ticket id
// (for --ticket mode), or "" for project mode.
func CompareGitToTickets(changedFiles []string, tickets []ledger.Row, focus string) GitFindings {
	covered := map[string]bool{}
	for _, t := range tickets {
		paths, _ := t["paths"].([]any)
		for _, p := range paths {
			if s, ok := p.(string); ok && s != "" {
				covered[s] = true
			}
		}
	}
	var untracked []string
	for _, c := range changedFiles {
		if !covered[c] {
			untracked = append(untracked, c)
		}
	}
	// Idle tickets: focus mode only.
	var idle []string
	changedSet := map[string]bool{}
	for _, c := range changedFiles {
		changedSet[c] = true
	}
	for _, t := range tickets {
		id, _ := t["ticket"].(string)
		if focus != "" && id != focus {
			continue
		}
		s, _ := t["status"].(string)
		if s != "in_progress" {
			continue
		}
		paths, _ := t["paths"].([]any)
		if len(paths) == 0 {
			continue
		}
		anyMatch := false
		for _, p := range paths {
			if str, ok := p.(string); ok && changedSet[str] {
				anyMatch = true
				break
			}
		}
		if !anyMatch {
			idle = append(idle, id)
		}
	}
	return GitFindings{Untracked: untracked, IdleTickets: idle}
}
