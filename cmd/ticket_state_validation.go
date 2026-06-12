package cmd

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hgwk/ldgr/internal/agent"
	"github.com/hgwk/ldgr/internal/config"
	"github.com/hgwk/ldgr/internal/ledger"
)

func autoFieldsState(dir string, in map[string]any, stderr io.Writer) (map[string]any, error) {
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

func requireStateEvent(row map[string]any) error {
	event, ok := row["event"].(map[string]any)
	if !ok {
		return errors.New("ticket: field \"event\" must be an object")
	}
	var errs validationErrors
	for _, f := range ledger.EventRequired {
		if _, ok := event[f]; !ok {
			errs.Add(fmt.Sprintf("ticket: missing required field \"event.%s\"", f))
		}
	}
	for _, f := range ledger.EventNonEmpty {
		raw, exists := event[f]
		if !exists {
			continue
		}
		v, ok := raw.(string)
		if !ok || v == "" {
			errs.Add(fmt.Sprintf("ticket: field \"event.%s\" must be non-empty", f))
		}
	}
	return errs.Err()
}

func validateStateTicketWrite(row map[string]any, prev ledger.Row) error {
	var errs validationErrors
	for _, check := range []struct {
		key  string
		enum map[string]struct{}
	}{
		{"type", ledger.TicketTypeEnum},
		{"state", ledger.StateEnum},
		{"area", ledger.AreaEnum},
		{"priority", ledger.PriorityEnum},
	} {
		value, _ := row[check.key].(string)
		if _, ok := check.enum[value]; !ok {
			errs.Add(fmt.Sprintf("ticket: invalid %s %q (allowed: %s)", check.key, value, allowedEnumValues(check.enum)))
		}
	}
	event, _ := row["event"].(map[string]any)
	role, _ := event["role"].(string)
	if _, ok := ledger.EventRoleEnum[role]; !ok {
		errs.Add(fmt.Sprintf("ticket: invalid event.role %q (allowed: %s)", role, allowedEnumValues(ledger.EventRoleEnum)))
	}
	if result, _ := event["result"].(string); result != "" {
		if _, ok := ledger.EventResultEnum[result]; !ok {
			errs.Add(fmt.Sprintf("ticket: invalid event.result %q (allowed: %s)", result, allowedEnumValues(ledger.EventResultEnum)))
		}
	}
	if err := errs.Err(); err != nil {
		return err
	}
	prevState := ""
	if prev != nil {
		prevState, _ = prev["state"].(string)
	}
	state, _ := row["state"].(string)
	if state != prevState {
		if !ledger.AllowsStateTransition(prevState, state) {
			return fmt.Errorf("ticket: lifecycle does not allow %s -> %s", displayState(prevState), state)
		}
	}
	switch state {
	case "done":
		if role != "auditor" || event["result"] != "pass" || !hasPositiveStateNumber(event["reviewed_n"]) || !hasNonEmptyStateList(row, "evidence") {
			return errors.New("ticket: state=done requires event.role=auditor, event.result=pass, event.reviewed_n, and non-empty evidence")
		}
	case "rework":
		notes, _ := event["notes"].(string)
		if role != "auditor" || event["result"] != "changes_requested" || !hasPositiveStateNumber(event["reviewed_n"]) || notes == "" {
			return errors.New("ticket: state=rework is an auditor changes-requested decision, not implementer rework start; to start fixes after rework, append state=doing with event.role=implementer; to request changes, use event.role=auditor, event.result=changes_requested, event.reviewed_n, and event.notes")
		}
	}
	return nil
}

func displayState(s string) string {
	if s == "" {
		return "<new>"
	}
	return s
}

func allowedEnumValues(values map[string]struct{}) string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

func hasPositiveStateNumber(v any) bool {
	switch n := v.(type) {
	case float64:
		return n > 0 && n == float64(int(n))
	case int:
		return n > 0
	}
	return false
}

func hasNonEmptyStateList(row map[string]any, key string) bool {
	arr, _ := row[key].([]any)
	for _, item := range arr {
		if s, ok := item.(string); ok && s != "" {
			return true
		}
	}
	return false
}

func isStateTarget(dir string) bool {
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
	return true
}

func isStateTicketInput(input map[string]any) bool {
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
