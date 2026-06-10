package cmd

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
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
	for _, f := range ledger.EventRequired {
		if _, ok := event[f]; !ok {
			return fmt.Errorf("ticket: missing required field \"event.%s\"", f)
		}
	}
	for _, f := range ledger.EventNonEmpty {
		v, ok := event[f].(string)
		if !ok || v == "" {
			return fmt.Errorf("ticket: field \"event.%s\" must be non-empty", f)
		}
	}
	return nil
}

func validateStateTicketWrite(row map[string]any, prev ledger.Row) error {
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
			return fmt.Errorf("ticket: invalid %s %q", check.key, value)
		}
	}
	event, _ := row["event"].(map[string]any)
	role, _ := event["role"].(string)
	if _, ok := ledger.EventRoleEnum[role]; !ok {
		return fmt.Errorf("ticket: invalid event.role %q", role)
	}
	if result, _ := event["result"].(string); result != "" {
		if _, ok := ledger.EventResultEnum[result]; !ok {
			return fmt.Errorf("ticket: invalid event.result %q", result)
		}
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
			return errors.New("ticket: state=rework requires event.role=auditor, event.result=changes_requested, event.reviewed_n, and event.notes")
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
