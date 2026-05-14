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

// RunSuggestCLI implements `ldgr suggest <worklog|commit|audit|correction|plan|pr> --ticket ID [--options]`.
func RunSuggestCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: ldgr suggest <worklog|commit|audit|correction|plan|pr> --ticket ID [--options]")
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "worklog":
		return suggestWorklogCmd(rest, stdout, stderr)
	case "commit":
		return suggestCommitCmd(rest, stdout, stderr)
	case "audit":
		return suggestAuditCmd(rest, stdout, stderr)
	case "correction":
		return suggestCorrectionCmd(rest, stdout, stderr)
	case "plan":
		return suggestPlanCmd(rest, stdout, stderr)
	case "pr":
		return suggestPRCmd(rest, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown suggest subcommand: %s\n", sub)
		return 2
	}
}

// loadTicketContext loads the ticket context for suggest subcommands.
// For "plan", latest may be nil (creating a new ticket).
// For others, latest must exist.
func loadTicketContext(target, ticket string, allowNew bool, stderr io.Writer) (ledger.Row, []ledger.Row, []ledger.Row, string, int) {
	if ticket == "" {
		fmt.Fprintln(stderr, "--ticket is required")
		return nil, nil, nil, "", 2
	}
	dir := resolveTarget(target)
	rows, err := ledger.ReadRows(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return nil, nil, nil, "", 1
	}
	latest, ok := findLatestTicket(rows, ticket)
	if !ok && !allowNew {
		fmt.Fprintf(stderr, "ticket %q not found\n", ticket)
		return nil, nil, nil, "", 1
	}
	worklog, _ := ledger.ReadRows(filepath.Join(dir, "ledger", "worklog.jsonl"))
	return latest, rows, worklog, dir, 0
}

func suggestWorklogCmd(rest []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("suggest worklog")
	target := fs.String("target", "", "")
	ticket := fs.String("ticket", "", "")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	latest, _, worklog, _, code := loadTicketContext(*target, *ticket, false, stderr)
	if code != 0 {
		return code
	}
	return suggestWorklog(latest, worklog, stdout)
}

func suggestCommitCmd(rest []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("suggest commit")
	target := fs.String("target", "", "")
	ticket := fs.String("ticket", "", "")
	allowUnaudited := fs.Bool("allow-unaudited", false, "")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	latest, _, worklog, _, code := loadTicketContext(*target, *ticket, false, stderr)
	if code != 0 {
		return code
	}
	return suggestCommit(latest, worklog, *allowUnaudited, stdout)
}

func suggestAuditCmd(rest []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("suggest audit")
	target := fs.String("target", "", "")
	ticket := fs.String("ticket", "", "")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	latest, _, worklog, _, code := loadTicketContext(*target, *ticket, false, stderr)
	if code != 0 {
		return code
	}
	return suggestAudit(latest, worklog, stdout)
}

func suggestCorrectionCmd(rest []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("suggest correction")
	_ = fs.String("target", "", "")
	ticket := fs.String("ticket", "", "")
	invalidatesN := fs.Int("invalidates-n", 0, "")
	notes := fs.String("notes", "", "")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if *ticket == "" {
		fmt.Fprintln(stderr, "--ticket is required")
		return 2
	}
	if *invalidatesN <= 0 {
		fmt.Fprintln(stderr, "--invalidates-n is required (positive integer)")
		return 2
	}
	return suggestCorrection(*ticket, *invalidatesN, *notes, stdout)
}

func suggestPlanCmd(rest []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("suggest plan")
	target := fs.String("target", "", "")
	ticket := fs.String("ticket", "", "")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	latest, _, _, _, code := loadTicketContext(*target, *ticket, true, stderr)
	if code != 0 {
		return code
	}
	return suggestPlan(latest, *ticket, stdout)
}

func suggestPRCmd(rest []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("suggest pr")
	target := fs.String("target", "", "")
	ticket := fs.String("ticket", "", "")
	allowUnaudited := fs.Bool("allow-unaudited", false, "")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	latest, _, worklog, _, code := loadTicketContext(*target, *ticket, false, stderr)
	if code != 0 {
		return code
	}
	return suggestPR(latest, worklog, *allowUnaudited, stdout)
}

func ticketIsAuditPassDone(latest ledger.Row) bool {
	if s, _ := latest["status"].(string); s != "done" {
		return false
	}
	if r, _ := latest["role"].(string); r != "audit" {
		return false
	}
	if ar, _ := latest["audit_result"].(string); ar != "pass" {
		return false
	}
	return true
}

func suggestWorklog(latest ledger.Row, worklog []ledger.Row, stdout io.Writer) int {
	if !ticketIsAuditPassDone(latest) {
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

func suggestCommit(latest ledger.Row, worklog []ledger.Row, allowUnaudited bool, stdout io.Writer) int {
	if !allowUnaudited && !ticketIsAuditPassDone(latest) {
		g := guidance.Compute(latest, worklog)
		fmt.Fprint(stdout, guidance.RenderText(g))
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Run with --allow-unaudited to emit the commit scaffold anyway.")
		return 0
	}

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

func suggestAudit(latest ledger.Row, worklog []ledger.Row, stdout io.Writer) int {
	// Check if latest status is audit_ready
	if status, _ := latest["status"].(string); status != "audit_ready" {
		g := guidance.Compute(latest, worklog)
		fmt.Fprint(stdout, guidance.RenderText(g))
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Ticket must be in 'audit_ready' status to emit audit skeletons.")
		return 0
	}

	// Extract reviewed_n from the latest audit_ready row's n field
	reviewedN := int(latest["n"].(float64))

	// Build two skeletons: pass and changes_requested
	pass := map[string]any{
		"ticket":       latest["ticket"],
		"role":         "audit",
		"status":       "done",
		"audit_result": "pass",
		"evidence":     []any{},
		"reviewed_n":   reviewedN,
	}
	changes := map[string]any{
		"ticket":       latest["ticket"],
		"role":         "audit",
		"status":       "changes_requested",
		"audit_result": "changes_requested",
		"audit_notes":  "",
		"evidence":     []any{},
		"reviewed_n":   reviewedN,
	}

	skeletons := []map[string]any{pass, changes}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(skeletons); err != nil {
		return 1
	}
	return 0
}

func suggestCorrection(ticket string, invalidatesN int, notes string, stdout io.Writer) int {
	skeleton := map[string]any{
		"ticket":        ticket,
		"role":          "ops",
		"status":        "cancelled",
		"invalidates_n": invalidatesN,
		"notes":         notes,
		"task":          fmt.Sprintf("invalidate n=%d", invalidatesN),
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(skeleton); err != nil {
		return 1
	}
	return 0
}

func suggestPlan(latest ledger.Row, ticket string, stdout io.Writer) int {
	// If ticket doesn't exist (latest is nil), create a new skeleton with defaults
	var parentTicket, scope string
	var paths []any

	if latest != nil {
		// Carry forward fields from existing ticket
		parentTicket = stringField(latest, "parent_ticket")
		scope = stringField(latest, "scope")
		if pathsVal, ok := latest["paths"].([]any); ok {
			paths = pathsVal
		} else {
			paths = []any{}
		}
	} else {
		// Defaults for new ticket
		parentTicket = "ROOT"
		scope = "repo"
		paths = []any{}
	}

	skeleton := map[string]any{
		"ticket":        ticket,
		"parent_ticket": parentTicket,
		"role":          "plan",
		"kind":          "plan",
		"priority":      "P2",
		"status":        "open",
		"task":          "<one-line>",
		"scope":         scope,
		"paths":         paths,
		"blocked_by":    []any{},
		"acceptance":    []any{},
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(skeleton); err != nil {
		return 1
	}
	return 0
}

func suggestPR(latest ledger.Row, worklog []ledger.Row, allowUnaudited bool, stdout io.Writer) int {
	if !allowUnaudited && !ticketIsAuditPassDone(latest) {
		g := guidance.Compute(latest, worklog)
		fmt.Fprint(stdout, guidance.RenderText(g))
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Run with --allow-unaudited to emit the PR scaffold anyway.")
		return 0
	}

	ticketID := stringField(latest, "ticket")
	task := stringField(latest, "task")
	truncatedTask := truncate(task, 60)

	fmt.Fprintf(stdout, "# PR: %s %s\n", ticketID, truncatedTask)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "## Summary")
	fmt.Fprintf(stdout, "- %s\n", task)
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
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "## Related ticket")
	auditResult := stringField(latest, "audit_result")
	if auditResult == "" {
		auditResult = "pending"
	}
	fmt.Fprintf(stdout, "- %s (audit_result=%s)\n", ticketID, auditResult)
	return 0
}
