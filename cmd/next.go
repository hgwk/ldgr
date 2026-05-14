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

// RunNextCLI implements `ldgr next --ticket ID [--format text|json]`.
func RunNextCLI(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("next")
	target := fs.String("target", "", "")
	ticket := fs.String("ticket", "", "")
	format := fs.String("format", "text", "")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *ticket == "" {
		fmt.Fprintln(stderr, "--ticket is required")
		return 2
	}
	dir := resolveTarget(*target)

	rows, err := ledger.ReadRows(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	latest, ok := findLatestTicket(rows, *ticket)
	if !ok {
		fmt.Fprintf(stderr, "ticket %q not found\n", *ticket)
		return 1
	}
	worklog, _ := ledger.ReadRows(filepath.Join(dir, "ledger", "worklog.jsonl"))
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
