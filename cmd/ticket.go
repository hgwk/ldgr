package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/hgwk/ldgr/internal/agent"
	"github.com/hgwk/ldgr/internal/gitutil"
	"github.com/hgwk/ldgr/internal/guidance"
	"github.com/hgwk/ldgr/internal/ledger"
	"github.com/hgwk/ldgr/internal/lifecycle"
)

func init() {
	Commands["ticket"] = func(args []string, stdout, stderr io.Writer) int {
		return RunTicketCLI(args, os.Stdin, stdout, stderr)
	}
}

// RunTicketCLI is the entry for `ldgr ticket ...`.
func RunTicketCLI(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: ldgr ticket <add|event|ready> [flags]")
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "add":
		return runTicketAdd(rest, stdin, stdout, stderr)
	case "event":
		return runTicketEvent(rest, stdin, stdout, stderr)
	case "ready":
		return runTicketReady(rest, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown ticket subcommand: %s\n", sub)
		return 2
	}
}

func runTicketAdd(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := newFlagSet("ticket add")
	target := fs.String("target", "", "")
	jsonSpec := fs.String("json", "", "")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dir := resolveTarget(*target)

	input, err := ReadJSONInput(*jsonSpec, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	row, err := normalizeTicketAdd(dir, input, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	path := filepath.Join(dir, "ledger", "tickets.jsonl")
	lock := filepath.Join(dir, "ledger", ".lock")
	out, err := ledger.Append(path, lock, ledger.Row(row))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	emitTicketGuidance(dir, out, stderr)
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return encErr(enc.Encode(out), stderr)
}

func normalizeTicketAdd(dir string, input map[string]any, stderr io.Writer) (map[string]any, error) {
	ticket, _ := input["ticket"].(string)
	if ticket == "" {
		return nil, errors.New("ticket: field 'ticket' is required")
	}
	rows, err := ledger.ReadRows(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r["ticket"] == ticket {
			return nil, fmt.Errorf("ticket %q already exists (use `ticket event` to update)", ticket)
		}
	}

	// Set defaults for kind and priority
	if _, has := input["kind"]; !has {
		input["kind"] = "task"
	}
	if _, has := input["priority"]; !has {
		input["priority"] = "P2"
	}

	resolved, err := autoFields(dir, input, stderr)
	if err != nil {
		return nil, err
	}
	// Check all required fields except 'n' which is assigned by Append
	required := make([]string, 0, len(ledger.TicketRequired))
	for _, f := range ledger.TicketRequired {
		if f != "n" {
			required = append(required, f)
		}
	}
	if err := requireFields(resolved, required, "ticket"); err != nil {
		return nil, err
	}
	if err := requireNonEmpty(resolved, ledger.TicketNonEmpty, "ticket"); err != nil {
		return nil, err
	}
	if v := lifecycle.Validate(ledger.Row(resolved), nil); v != nil {
		return nil, fmt.Errorf("%s\n%s", v.Message, v.Hint)
	}
	return resolved, nil
}

func runTicketEvent(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := newFlagSet("ticket event")
	target := fs.String("target", "", "")
	jsonSpec := fs.String("json", "", "")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dir := resolveTarget(*target)

	input, err := ReadJSONInput(*jsonSpec, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	row, err := normalizeTicketEvent(dir, input, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	out, err := ledger.Append(filepath.Join(dir, "ledger", "tickets.jsonl"), filepath.Join(dir, "ledger", ".lock"), ledger.Row(row))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	emitTicketGuidance(dir, out, stderr)
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return encErr(enc.Encode(out), stderr)
}

func normalizeTicketEvent(dir string, input map[string]any, stderr io.Writer) (map[string]any, error) {
	ticket, _ := input["ticket"].(string)
	if ticket == "" {
		return nil, errors.New("ticket event: field 'ticket' is required")
	}
	rows, err := ledger.ReadRows(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if err != nil {
		return nil, err
	}
	var base map[string]any
	var prevRow ledger.Row
	for _, r := range rows {
		if r["ticket"] == ticket {
			// Make a copy of the row to avoid modifying the original
			base = make(map[string]any)
			for k, v := range r {
				base[k] = v
			}
			prevRow = r
		}
	}
	if base == nil {
		return nil, fmt.Errorf("ticket %q does not exist (use `ticket add` first)", ticket)
	}
	for k, v := range input {
		base[k] = v
	}
	delete(base, "n")
	base["ts"] = ""

	resolved, err := autoFields(dir, base, stderr)
	if err != nil {
		return nil, err
	}
	// Check all required fields except 'n' which is assigned by Append
	required := make([]string, 0, len(ledger.TicketRequired))
	for _, f := range ledger.TicketRequired {
		if f != "n" {
			required = append(required, f)
		}
	}
	if err := requireFields(resolved, required, "ticket"); err != nil {
		return nil, err
	}
	if err := requireNonEmpty(resolved, ledger.TicketNonEmpty, "ticket"); err != nil {
		return nil, err
	}
	if v := lifecycle.Validate(ledger.Row(resolved), prevRow); v != nil {
		return nil, fmt.Errorf("%s\n%s", v.Message, v.Hint)
	}
	return resolved, nil
}

// autoFields fills agent, ts, branch when the caller did not supply them.
// n is assigned later by ledger.Append.
func autoFields(dir string, in map[string]any, stderr io.Writer) (map[string]any, error) {
	if _, ok := in["ts"]; !ok || in["ts"] == "" {
		in["ts"] = time.Now().UTC().Format("2006-01-02T15:04:05Z")
	}
	if _, ok := in["branch"]; !ok {
		in["branch"] = gitutil.CurrentBranch(dir)
	}
	envMap := envAsMap()
	fromJSON, _ := in["agent"].(string)
	resolved, warn, err := agent.Resolve(fromJSON, envMap)
	if err != nil {
		return nil, err
	}
	in["agent"] = resolved
	if warn != "" {
		fmt.Fprintln(stderr, "warning:", warn)
	}
	return in, nil
}

func envAsMap() map[string]string {
	out := map[string]string{}
	for _, kv := range os.Environ() {
		for i := 0; i < len(kv); i++ {
			if kv[i] == '=' {
				out[kv[:i]] = kv[i+1:]
				break
			}
		}
	}
	return out
}

func requireFields(row map[string]any, required []string, kind string) error {
	for _, f := range required {
		v, ok := row[f]
		if !ok || v == nil {
			return fmt.Errorf("%s: missing required field %q", kind, f)
		}
	}
	return nil
}

func isEmpty(v any) bool {
	if v == nil {
		return true
	}
	if s, ok := v.(string); ok && s == "" {
		return true
	}
	return false
}

func requireNonEmpty(row map[string]any, fields []string, kind string) error {
	for _, f := range fields {
		v, ok := row[f]
		if !ok {
			return fmt.Errorf("%s: missing required field %q", kind, f)
		}
		s, isStr := v.(string)
		if !isStr || s == "" {
			return fmt.Errorf("%s: field %q must be non-empty", kind, f)
		}
	}
	return nil
}

func resolveTarget(target string) string {
	if target == "" {
		wd, _ := os.Getwd()
		return wd
	}
	abs, _ := filepath.Abs(target)
	return abs
}

func encErr(err error, stderr io.Writer) int {
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

// emitTicketGuidance reads the latest state for the row that was just appended
// and writes guidance text to the provided stderr writer. Best-effort: any
// error during read/compute is ignored because the ledger has already been
// successfully written.
func emitTicketGuidance(dir string, row map[string]any, stderr io.Writer) {
	id, _ := row["ticket"].(string)
	if id == "" {
		return
	}
	tickets, err := ledger.ReadRows(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if err != nil {
		return
	}
	latest, ok := findLatestTicket(tickets, id)
	if !ok {
		latest = ledger.Row(row)
	}
	worklog, _ := ledger.ReadRows(filepath.Join(dir, "ledger", "worklog.jsonl"))
	g := guidance.Compute(latest, worklog)
	fmt.Fprint(stderr, guidance.RenderText(g))
}

// multiString is a flag.Value for repeatable string flags.
type multiString []string

func (m *multiString) String() string { return "" }
func (m *multiString) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func stringsToAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func runTicketReady(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("ticket ready")
	target := fs.String("target", "", "")
	ticket := fs.String("ticket", "", "")
	var evidence multiString
	fs.Var(&evidence, "evidence", "evidence (repeatable)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *ticket == "" {
		fmt.Fprintln(stderr, "--ticket is required")
		return 2
	}
	if len(evidence) == 0 {
		fmt.Fprintln(stderr, "at least one --evidence is required")
		return 2
	}
	dir := resolveTarget(*target)
	// Build event JSON as map, hand to normalizeTicketEvent.
	input := map[string]any{
		"ticket":   *ticket,
		"status":   "audit_ready",
		"evidence": stringsToAny(evidence),
	}
	row, err := normalizeTicketEvent(dir, input, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	out, err := ledger.Append(filepath.Join(dir, "ledger", "tickets.jsonl"), filepath.Join(dir, "ledger", ".lock"), ledger.Row(row))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	emitTicketGuidance(dir, out, stderr)
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return encErr(enc.Encode(out), stderr)
}
