package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/hgwk/ldgr/internal/guidance"
	"github.com/hgwk/ldgr/internal/ledger"
)

func init() {
	Commands["suggest"] = RunSuggestCLI
}

// RunSuggestCLI implements `ldgr suggest worklog|commit --ticket ID`.
func RunSuggestCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: ldgr suggest <worklog|commit> --ticket ID")
		return 2
	}
	sub, rest := args[0], args[1:]
	fs := newFlagSet("suggest " + sub)
	target := fs.String("target", "", "")
	ticket := fs.String("ticket", "", "")
	if err := fs.Parse(rest); err != nil {
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

	switch sub {
	case "worklog":
		return suggestWorklog(latest, worklog, stdout)
	case "commit":
		return suggestCommit(latest, stdout)
	default:
		fmt.Fprintf(stderr, "unknown suggest subcommand: %s\n", sub)
		return 2
	}
}

func suggestWorklog(latest ledger.Row, worklog []ledger.Row, stdout io.Writer) int {
	status, _ := latest["status"].(string)
	auditResult, _ := latest["audit_result"].(string)
	if status != "done" || auditResult != "pass" {
		g := guidance.Compute(latest, worklog)
		fmt.Fprint(stdout, guidance.RenderText(g))
		return 0
	}
	skeleton := map[string]any{
		"ticket":   latest["ticket"],
		"task":     latest["task"],
		"scope":    latest["scope"],
		"result":   "shipped: " + stringField(latest, "task"),
		"paths":    latest["paths"],
		"commands": ifSliceField(latest, "evidence"),
		"notes":    "",
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(skeleton); err != nil {
		return 1
	}
	return 0
}

func suggestCommit(latest ledger.Row, stdout io.Writer) int {
	commitType := commitTypeFromCategory(stringField(latest, "category"))
	scope := strings.ToLower(stringField(latest, "parent_ticket"))
	if scope == "" || scope == "root" {
		scope = ""
	}
	subject := truncate(stringField(latest, "task"), 72)

	var line string
	if scope != "" {
		line = fmt.Sprintf("%s(%s): %s", commitType, scope, subject)
	} else {
		line = fmt.Sprintf("%s: %s", commitType, subject)
	}
	fmt.Fprintln(stdout, line)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "## Summary")
	fmt.Fprintf(stdout, "- %s\n", stringField(latest, "task"))
	if notes := stringField(latest, "notes"); notes != "" {
		fmt.Fprintf(stdout, "- %s\n", notes)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "## Verification")
	evidence := stringSliceFromRow(latest, "evidence")
	if len(evidence) == 0 {
		fmt.Fprintln(stdout, "- TODO: paste the commands you ran (ldgr verify, go test, etc.)")
	} else {
		for _, e := range evidence {
			fmt.Fprintf(stdout, "- %s\n", e)
		}
	}
	return 0
}

func commitTypeFromCategory(cat string) string {
	switch cat {
	case "feature", "design", "demo":
		return "feat"
	case "bug":
		return "fix"
	case "docs", "research":
		return "docs"
	case "test":
		return "test"
	case "refactor", "cleanup":
		return "refactor"
	case "ops", "infra", "release":
		return "chore"
	}
	return "chore"
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return strings.TrimSpace(s[:max])
}

func ifSliceField(r ledger.Row, k string) []any {
	v, _ := r[k].([]any)
	if v == nil {
		return []any{}
	}
	return v
}

func stringSliceFromRow(r ledger.Row, k string) []string {
	raw, _ := r[k].([]any)
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func stringField(r ledger.Row, k string) string {
	v, _ := r[k].(string)
	return v
}
