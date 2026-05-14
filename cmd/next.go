package cmd

import (
	"fmt"
	"io"
	"path/filepath"

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

// RunNextCLI implements `ldgr next [--ticket ID] [--role ROLE] [--format text|json]`.
func RunNextCLI(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("next")
	target := fs.String("target", "", "")
	ticket := fs.String("ticket", "", "")
	format := fs.String("format", "text", "")
	role := fs.String("role", "", "implementer|auditor|planner|maintainer (project-wide mode)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	// Validate --role if provided.
	if *role != "" && !validRoles[*role] {
		fmt.Fprintf(stderr, "invalid --role %q: must be one of implementer|auditor|planner|maintainer\n", *role)
		return 2
	}

	dir := resolveTarget(*target)

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
		if *format == "json" {
			data, _ := guidance.RenderProjectJSON(pg)
			fmt.Fprintln(stdout, string(data))
		} else {
			fmt.Fprint(stdout, guidance.RenderProjectText(pg))
		}
		return 0
	}

	// Ticket-scoped mode.
	latest, ok := findLatestTicket(ticketRows, *ticket)
	if !ok {
		fmt.Fprintf(stderr, "ticket %q not found\n", *ticket)
		return 1
	}
	g := guidance.Compute(latest, worklog)
	switch *format {
	case "json":
		data, err := guidance.RenderJSON(g)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintln(stdout, string(data))
	default:
		fmt.Fprint(stdout, guidance.RenderText(g))
	}
	return 0
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
