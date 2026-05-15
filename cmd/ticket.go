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
	"github.com/hgwk/ldgr/internal/config"
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
	if isCanonicalTicketInput(input) {
		return normalizeCanonicalTicketAdd(dir, input, stderr)
	}
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
	if isCanonicalTicketInput(input) || isCanonicalTarget(dir) {
		return normalizeCanonicalTicketEvent(dir, input, stderr)
	}
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

func normalizeCanonicalTicketAdd(dir string, input map[string]any, stderr io.Writer) (map[string]any, error) {
	id, _ := input["id"].(string)
	if id == "" {
		return nil, errors.New("ticket: field 'id' is required for canonical v1")
	}
	rows, err := ledger.ReadRows(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r["id"] == id {
			return nil, fmt.Errorf("ticket %q already exists (use `ticket event` to update)", id)
		}
	}
	resolved, err := autoFieldsCanonical(dir, input, stderr)
	if err != nil {
		return nil, err
	}
	required := withoutN(ledger.CanonicalTicketRequired)
	if err := requireFields(resolved, required, "ticket"); err != nil {
		return nil, err
	}
	if err := requireNonEmpty(resolved, ledger.CanonicalTicketNonEmpty, "ticket"); err != nil {
		return nil, err
	}
	if err := requireCanonicalEvent(resolved); err != nil {
		return nil, err
	}
	if err := validateCanonicalTicketWrite(resolved, nil); err != nil {
		return nil, err
	}
	return resolved, nil
}

func normalizeCanonicalTicketEvent(dir string, input map[string]any, stderr io.Writer) (map[string]any, error) {
	id, _ := input["id"].(string)
	if id == "" {
		return nil, errors.New("ticket event: field 'id' is required for canonical v1")
	}
	rows, err := ledger.ReadRows(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if err != nil {
		return nil, err
	}
	var base map[string]any
	var prevRow ledger.Row
	for _, r := range rows {
		if r["id"] == id {
			base = make(map[string]any)
			for k, v := range r {
				base[k] = v
			}
			prevRow = r
		}
	}
	if base == nil {
		return nil, fmt.Errorf("ticket %q does not exist (use `ticket add` first)", id)
	}
	for k, v := range input {
		base[k] = v
	}
	delete(base, "n")
	base["ts"] = ""
	resolved, err := autoFieldsCanonical(dir, base, stderr)
	if err != nil {
		return nil, err
	}
	if err := requireFields(resolved, withoutN(ledger.CanonicalTicketRequired), "ticket"); err != nil {
		return nil, err
	}
	if err := requireNonEmpty(resolved, ledger.CanonicalTicketNonEmpty, "ticket"); err != nil {
		return nil, err
	}
	if err := requireCanonicalEvent(resolved); err != nil {
		return nil, err
	}
	if err := validateCanonicalTicketWrite(resolved, prevRow); err != nil {
		return nil, err
	}
	return resolved, nil
}

func autoFieldsCanonical(dir string, in map[string]any, stderr io.Writer) (map[string]any, error) {
	if _, ok := in["ts"]; !ok || in["ts"] == "" {
		in["ts"] = time.Now().UTC().Format("2006-01-02T15:04:05Z")
	}
	envMap := envAsMap()
	owner, _ := in["owner"].(string)
	resolved, warn, err := agent.Resolve(owner, envMap)
	if err != nil {
		return nil, err
	}
	if owner == "" {
		in["owner"] = resolved
	}
	event, _ := in["event"].(map[string]any)
	if event != nil {
		if actor, _ := event["actor"].(string); actor == "" {
			event["actor"] = resolved
		}
	}
	if warn != "" {
		fmt.Fprintln(stderr, "warning:", warn)
	}
	return in, nil
}

func requireCanonicalEvent(row map[string]any) error {
	event, ok := row["event"].(map[string]any)
	if !ok {
		return errors.New("ticket: field \"event\" must be an object")
	}
	for _, f := range ledger.CanonicalEventRequired {
		if _, ok := event[f]; !ok {
			return fmt.Errorf("ticket: missing required field \"event.%s\"", f)
		}
	}
	for _, f := range ledger.CanonicalEventNonEmpty {
		v, ok := event[f].(string)
		if !ok || v == "" {
			return fmt.Errorf("ticket: field \"event.%s\" must be non-empty", f)
		}
	}
	return nil
}

func validateCanonicalTicketWrite(row map[string]any, prev ledger.Row) error {
	for _, check := range []struct {
		key  string
		enum map[string]struct{}
	}{
		{"type", ledger.CanonicalTypeEnum},
		{"state", ledger.CanonicalStateEnum},
		{"area", ledger.CanonicalAreaEnum},
		{"priority", ledger.PriorityEnum},
	} {
		value, _ := row[check.key].(string)
		if _, ok := check.enum[value]; !ok {
			return fmt.Errorf("ticket: invalid %s %q", check.key, value)
		}
	}
	event, _ := row["event"].(map[string]any)
	role, _ := event["role"].(string)
	if _, ok := ledger.CanonicalEventRoleEnum[role]; !ok {
		return fmt.Errorf("ticket: invalid event.role %q", role)
	}
	if result, _ := event["result"].(string); result != "" {
		if _, ok := ledger.CanonicalEventResultEnum[result]; !ok {
			return fmt.Errorf("ticket: invalid event.result %q", result)
		}
	}
	prevState := ""
	if prev != nil {
		prevState, _ = prev["state"].(string)
	}
	state, _ := row["state"].(string)
	if state != prevState {
		if !allowedCanonicalWriteTransition(prevState, state) {
			return fmt.Errorf("ticket: lifecycle does not allow %s -> %s", displayState(prevState), state)
		}
	}
	switch state {
	case "done":
		if role != "auditor" || event["result"] != "pass" || !hasPositiveCanonicalNumber(event["reviewed_n"]) || !hasNonEmptyCanonicalList(row, "evidence") {
			return errors.New("ticket: state=done requires event.role=auditor, event.result=pass, event.reviewed_n, and non-empty evidence")
		}
	case "rework":
		notes, _ := event["notes"].(string)
		if role != "auditor" || event["result"] != "changes_requested" || !hasPositiveCanonicalNumber(event["reviewed_n"]) || notes == "" {
			return errors.New("ticket: state=rework requires event.role=auditor, event.result=changes_requested, event.reviewed_n, and event.notes")
		}
	}
	return nil
}

func allowedCanonicalWriteTransition(prev, next string) bool {
	allowed := map[string]map[string]bool{
		"":        {"backlog": true, "ready": true},
		"backlog": {"ready": true, "dropped": true},
		"ready":   {"doing": true, "blocked": true, "dropped": true},
		"doing":   {"review": true, "blocked": true, "dropped": true},
		"blocked": {"ready": true, "doing": true, "dropped": true},
		"review":  {"done": true, "rework": true, "dropped": true},
		"rework":  {"doing": true, "ready": true, "dropped": true},
	}
	return allowed[prev][next]
}

func displayState(s string) string {
	if s == "" {
		return "<new>"
	}
	return s
}

func hasPositiveCanonicalNumber(v any) bool {
	switch n := v.(type) {
	case float64:
		return n > 0 && n == float64(int(n))
	case int:
		return n > 0
	}
	return false
}

func hasNonEmptyCanonicalList(row map[string]any, key string) bool {
	arr, _ := row[key].([]any)
	for _, item := range arr {
		if s, ok := item.(string); ok && s != "" {
			return true
		}
	}
	return false
}

func isCanonicalTarget(dir string) bool {
	cfgPath := filepath.Join(dir, "ledger", "config.json")
	version, err := config.SchemaVersion(cfgPath)
	if err == nil && version != 1 {
		return false
	}
	rows, readErr := ledger.ReadRows(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if readErr != nil {
		return false
	}
	for _, row := range rows {
		if _, ok := row["id"]; ok {
			return true
		}
		if _, ok := row["state"]; ok {
			return true
		}
		if _, ok := row["event"]; ok {
			return true
		}
		if _, ok := row["ticket"]; ok {
			return false
		}
		if _, ok := row["status"]; ok {
			return false
		}
	}
	return false
}

func isCanonicalTicketInput(input map[string]any) bool {
	if _, ok := input["id"]; ok {
		return true
	}
	if _, ok := input["state"]; ok {
		return true
	}
	if _, ok := input["event"]; ok {
		return true
	}
	return false
}

func withoutN(fields []string) []string {
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f != "n" {
			out = append(out, f)
		}
	}
	return out
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
	g.WritingLanguage = loadWritingLanguage(dir)
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
	input := map[string]any{}
	if isCanonicalTarget(dir) {
		input = map[string]any{
			"id":       *ticket,
			"state":    "review",
			"evidence": stringsToAny(evidence),
			"event": map[string]any{
				"role":    "implementer",
				"summary": "ready for review",
				"notes":   "",
			},
		}
	} else {
		input = map[string]any{
			"ticket":   *ticket,
			"status":   "audit_ready",
			"evidence": stringsToAny(evidence),
		}
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
