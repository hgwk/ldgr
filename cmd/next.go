package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	"github.com/hgwk/ldgr/internal/config"
	"github.com/hgwk/ldgr/internal/gitutil"
	"github.com/hgwk/ldgr/internal/guidance"
	"github.com/hgwk/ldgr/internal/ledger"
)

func init() {
	Commands["next"] = RunNextCLI
}

var validRoles = map[string]bool{
	"implementer": true,
	"auditor":     true,
	"planner":     true,
	"maintainer":  true,
}

// RunNextCLI implements `ldgr next [--ticket ID] [--role ROLE] [--format text|json] [--git]`.
func RunNextCLI(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("next")
	target := fs.String("target", "", "")
	ticket := fs.String("ticket", "", "")
	format := fs.String("format", "text", "")
	role := fs.String("role", "", "implementer|auditor|planner|maintainer (project-wide mode)")
	gitFlag := fs.Bool("git", false, "compare git working tree against ticket paths")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	// Validate --role if provided.
	if *role != "" && !validRoles[*role] {
		fmt.Fprintf(stderr, "invalid --role %q: must be one of implementer|auditor|planner|maintainer\n", *role)
		return 2
	}

	dir := resolveTarget(*target)
	writingLanguage := loadWritingLanguage(dir)

	// Read ledger files (both modes need them).
	ticketRows, err := ledger.ReadRows(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	worklog, _ := ledger.ReadRows(filepath.Join(dir, "ledger", "worklog.jsonl"))

	if *ticket == "" {
		// Project-wide mode.
		pg := guidance.ComputeProject(ticketRows, worklog, *role)
		if isStateTarget(dir) {
			pg = guidance.ComputeStateProject(ticketRows, worklog, *role)
		}
		pg.WritingLanguage = writingLanguage
		if *gitFlag && gitutil.IsWorkTree(dir) {
			changed := gitutil.ChangedFiles(dir)
			findings := guidance.CompareGitToTickets(changed, latestActive(ticketRows), "")
			// Emit findings to stderr.
			if len(findings.Untracked) > 0 {
				fmt.Fprintf(stderr, "git: %d uncommitted file(s) not covered by any active ticket path:\n", len(findings.Untracked))
				for _, f := range findings.Untracked {
					fmt.Fprintf(stderr, "  %s\n", f)
				}
			}
			for _, id := range findings.IdleTickets {
				fmt.Fprintf(stderr, "git: ticket %s in_progress but no changed file matches its paths.\n", id)
			}
		}
		if *format == "json" {
			data, _ := guidance.RenderProjectJSON(pg)
			if *gitFlag && gitutil.IsWorkTree(dir) {
				changed := gitutil.ChangedFiles(dir)
				findings := guidance.CompareGitToTickets(changed, latestActive(ticketRows), "")
				var m map[string]any
				json.Unmarshal(data, &m)
				m["git"] = findings
				data, _ = json.MarshalIndent(m, "", "  ")
			}
			fmt.Fprintln(stdout, string(data))
		} else {
			fmt.Fprint(stdout, guidance.RenderProjectText(pg))
		}
		return 0
	}

	// Ticket-scoped mode.
	var latest ledger.Row
	var ok bool
	if isStateTarget(dir) {
		latest, ok = findLatestStateTicket(ticketRows, *ticket)
	} else {
		latest, ok = findLatestTicket(ticketRows, *ticket)
	}
	if !ok {
		fmt.Fprintf(stderr, "ticket %q not found\n", *ticket)
		return 1
	}
	var g guidance.Guidance
	if isStateTarget(dir) {
		g = guidance.ComputeState(latest, worklog)
	} else {
		g = guidance.Compute(latest, worklog)
	}
	g.WritingLanguage = writingLanguage
	if *gitFlag && gitutil.IsWorkTree(dir) {
		changed := gitutil.ChangedFiles(dir)
		findings := guidance.CompareGitToTickets(changed, latestActive(ticketRows), *ticket)
		// Emit findings to stderr.
		if len(findings.Untracked) > 0 {
			fmt.Fprintf(stderr, "git: %d uncommitted file(s) not covered by any active ticket path:\n", len(findings.Untracked))
			for _, f := range findings.Untracked {
				fmt.Fprintf(stderr, "  %s\n", f)
			}
		}
		for _, id := range findings.IdleTickets {
			fmt.Fprintf(stderr, "git: ticket %s in_progress but no changed file matches its paths.\n", id)
		}
	}
	switch *format {
	case "json":
		data, err := guidance.RenderJSON(g)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if *gitFlag && gitutil.IsWorkTree(dir) {
			changed := gitutil.ChangedFiles(dir)
			findings := guidance.CompareGitToTickets(changed, latestActive(ticketRows), *ticket)
			var m map[string]any
			json.Unmarshal(data, &m)
			m["git"] = findings
			data, _ = json.MarshalIndent(m, "", "  ")
		}
		fmt.Fprintln(stdout, string(data))
	default:
		fmt.Fprint(stdout, guidance.RenderText(g))
	}
	return 0
}

func loadWritingLanguage(dir string) string {
	cfg, err := config.Load(filepath.Join(dir, "ledger", "config.json"))
	if err != nil {
		return ""
	}
	return cfg.WritingLanguage
}

// latestActive returns the latest row for each ticket with active status.
func latestActive(ticketRows []ledger.Row) []ledger.Row {
	latest := guidance.LatestTickets(ticketRows)
	activeOnly := make([]ledger.Row, 0, len(latest))
	for _, r := range latest {
		s, _ := r["status"].(string)
		switch s {
		case "open", "in_progress", "blocked", "audit_ready", "changes_requested":
			activeOnly = append(activeOnly, r)
		}
	}
	return activeOnly
}

// findLatestTicket returns the latest non-invalidate row matching the ticket id.
func findLatestTicket(rows []ledger.Row, ticketID string) (ledger.Row, bool) {
	var latest ledger.Row
	var maxN float64 = -1
	for _, r := range rows {
		if id, _ := r["ticket"].(string); id != ticketID {
			continue
		}
		if _, isCompanion := r["invalidates_n"]; isCompanion {
			continue
		}
		n, _ := r["n"].(float64)
		if n > maxN {
			maxN = n
			latest = r
		}
	}
	return latest, latest != nil
}
