package viewer

import (
	"time"

	"github.com/hgwk/ldgr/internal/ledger"
)

func parseTS(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func stringField(r ledger.Row, k string) string {
	v, _ := r[k].(string)
	return v
}

func ticketID(r ledger.Row) string {
	if v := stringField(r, "ticket"); v != "" {
		return v
	}
	return stringField(r, "id")
}

func ticketParent(r ledger.Row) string {
	if v := stringField(r, "parent_ticket"); v != "" {
		return v
	}
	return stringField(r, "parent")
}

func ticketState(r ledger.Row) string {
	if v := stringField(r, "status"); v != "" {
		return v
	}
	return stringField(r, "state")
}

func ticketTitle(r ledger.Row) string {
	if v := stringField(r, "task"); v != "" {
		return v
	}
	return stringField(r, "title")
}

func ticketType(r ledger.Row) string {
	if v := stringField(r, "kind"); v != "" {
		return v
	}
	return stringField(r, "type")
}

func ticketOwner(r ledger.Row) string {
	if v := stringField(r, "claimed_by"); v != "" {
		return v
	}
	if v := stringField(r, "owner"); v != "" {
		return v
	}
	return stringField(r, "agent")
}

func eventString(r ledger.Row, k string) string {
	event, _ := r["event"].(map[string]any)
	if event == nil {
		return ""
	}
	v, _ := event[k].(string)
	return v
}

func isTerminalState(state string) bool {
	return state == "done" || state == "cancelled" || state == "dropped"
}

func isActiveState(state string) bool {
	if state == "" || isTerminalState(state) {
		return false
	}
	return true
}

// --- Dashboard (control tower) -------------------------------------------------
